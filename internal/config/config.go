package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Node      NodeConfig      `yaml:"node"`
	Server    ServerConfig    `yaml:"server"`
	Sync      SyncConfig      `yaml:"sync"`
	Embedding EmbeddingConfig `yaml:"embedding"`
	Storage   StorageConfig   `yaml:"storage"`
	Defaults  DefaultsConfig  `yaml:"defaults"`
	Peers     PeersConfig     `yaml:"peers"`
	Chat      ChatConfig      `yaml:"chat"`

	// internal — not serialized
	filePath string `yaml:"-"`
}

type ChatConfig struct {
	DefaultModel string             `yaml:"default_model"`
	Providers    ChatProvidersConfig `yaml:"providers"`
}

type ChatProvidersConfig struct {
	Anthropic ChatProviderKeyConfig `yaml:"anthropic"`
	Google    ChatProviderKeyConfig `yaml:"google"`
	OpenAI    ChatProviderKeyConfig `yaml:"openai"`
	Ollama    ChatOllamaConfig      `yaml:"ollama"`
}

type ChatProviderKeyConfig struct {
	APIKey string `yaml:"api_key"`
	Env    string `yaml:"env"` // env var name to read key from
}

// EnvKey returns the API key from environment if configured.
func (c ChatProviderKeyConfig) EnvKey() string {
	if c.APIKey != "" {
		return c.APIKey
	}
	envName := c.Env
	if envName == "" {
		return ""
	}
	return os.Getenv(envName)
}

type ChatOllamaConfig struct {
	Host string `yaml:"host"`
}

type NodeConfig struct {
	Name        string `yaml:"name"`
	DisplayName string `yaml:"display_name"`
	Email       string `yaml:"email"`
	Avatar      string `yaml:"avatar"` // URL or emoji
	UUID        string `yaml:"uuid"`   // UUIDv7 peer identity, auto-generated on first run

	// Runtime fields (not persisted in YAML)
	Hostname string `yaml:"-"`
	MAC      string `yaml:"-"`
}

// PeerID returns the node's unique peer identifier (UUIDv7).
func (n *NodeConfig) PeerID() string {
	return n.UUID
}

type ServerConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	OpenBrowser bool   `yaml:"open_browser"`
}

type SyncConfig struct {
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	FanOut  int    `yaml:"fan_out"`
	AntiEntropySeconds int `yaml:"anti_entropy_seconds"`
}

type EmbeddingConfig struct {
	OllamaURL string `yaml:"ollama_url"`
	Model     string `yaml:"model"`
	Dimension int    `yaml:"dimension"`
}

type StorageConfig struct {
	DataDir string `yaml:"data_dir"`
}

type DefaultsConfig struct {
	Scope   string `yaml:"scope"`
	Project string `yaml:"project"`
}

type PeersConfig struct {
	Static []string `yaml:"static"`
}

func DefaultConfig() *Config {
	hostname, _ := os.Hostname()
	homeDir, _ := os.UserHomeDir()

	return &Config{
		Node: NodeConfig{
			Name: hostname,
		},
		Server: ServerConfig{
			Host:        "127.0.0.1",
			Port:        8765,
			OpenBrowser: true,
		},
		Sync: SyncConfig{
			Host:               "0.0.0.0",
			Port:               8766,
			FanOut:             3,
			AntiEntropySeconds: 60,
		},
		Embedding: EmbeddingConfig{
			OllamaURL: "http://localhost:11434",
			Model:     "nomic-embed-text",
			Dimension: 768,
		},
		Storage: StorageConfig{
			DataDir: filepath.Join(homeDir, ".openloom", "data"),
		},
		Defaults: DefaultsConfig{
			Scope:   "project",
			Project: filepath.Base(mustCwd()),
		},
		Chat: ChatConfig{
			DefaultModel: "anthropic/claude-sonnet-4-6",
			Providers: ChatProvidersConfig{
				Anthropic: ChatProviderKeyConfig{Env: "ANTHROPIC_API_KEY"},
				Google:    ChatProviderKeyConfig{Env: "GOOGLE_API_KEY"},
				OpenAI:    ChatProviderKeyConfig{Env: "OPENAI_API_KEY"},
				Ollama:    ChatOllamaConfig{Host: "http://localhost:11434"},
			},
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	// Determine config file path
	if path == "" {
		homeDir, _ := os.UserHomeDir()
		path = filepath.Join(homeDir, ".openloom", "config.yaml")
	}

	// Read config file if it exists
	data, err := os.ReadFile(path)
	if err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	cfg.filePath = path

	// Environment variable overrides
	applyEnvOverrides(cfg)

	// Populate runtime fields
	cfg.Node.Hostname, _ = os.Hostname()
	cfg.Node.MAC = getPrimaryMAC()

	// Auto-generate UUIDv7 on first run and persist
	if cfg.Node.UUID == "" {
		id, err := uuid.NewV7()
		if err == nil {
			cfg.Node.UUID = id.String()
			if err := cfg.Save(); err != nil {
				slog.Warn("save config after UUID generation failed", "error", err)
			}
		}
	}

	// Ensure data directory exists
	if err := os.MkdirAll(cfg.Storage.DataDir, 0755); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Save writes the config back to its YAML file.
func (c *Config) Save() error {
	if c.filePath == "" {
		homeDir, _ := os.UserHomeDir()
		c.filePath = filepath.Join(homeDir, ".openloom", "config.yaml")
	}
	if err := os.MkdirAll(filepath.Dir(c.filePath), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(c.filePath, data, 0644)
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("OPENLOOM_NODE_NAME"); v != "" {
		cfg.Node.Name = v
	}
	if v := os.Getenv("OPENLOOM_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("OPENLOOM_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
	if v := os.Getenv("OPENLOOM_SYNC_HOST"); v != "" {
		cfg.Sync.Host = v
	}
	if v := os.Getenv("OPENLOOM_SYNC_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Sync.Port = p
		}
	}
	if v := os.Getenv("OPENLOOM_OLLAMA_URL"); v != "" {
		cfg.Embedding.OllamaURL = v
	}
	if v := os.Getenv("OPENLOOM_EMBEDDING_MODEL"); v != "" {
		cfg.Embedding.Model = v
	}
	if v := os.Getenv("OPENLOOM_DATA_DIR"); v != "" {
		cfg.Storage.DataDir = v
	}
	if v := os.Getenv("OPENLOOM_DEFAULT_PROJECT"); v != "" {
		cfg.Defaults.Project = v
	}
	if v := os.Getenv("OPENLOOM_OPEN_BROWSER"); v == "false" {
		cfg.Server.OpenBrowser = false
	}
}

func mustCwd() string {
	dir, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return dir
}
