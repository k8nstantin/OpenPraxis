package setup

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

// AgentConfig describes a coding agent that can be connected to OpenLoom.
type AgentConfig struct {
	Name       string `json:"name"`        // Display name
	ID         string `json:"id"`          // Internal identifier
	ConfigPath string `json:"config_path"` // Where to write MCP config
	ConfigType string `json:"config_type"` // "mcp_json", "settings_json", "custom"
	Installed  bool   `json:"installed"`   // Detected on this machine
	Connected  bool   `json:"connected"`   // MCP config present
}

// binaryPath returns the path to the current openloom binary.
func binaryPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "openloom"
	}
	// Resolve symlinks
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe
	}
	return resolved
}

func mcpEntry() map[string]any {
	return map[string]any{
		"command": binaryPath(),
		"args":    []string{"mcp"},
	}
}

// DetectAgents finds installed coding agents and checks if they're connected.
func DetectAgents() []AgentConfig {
	home, _ := os.UserHomeDir()

	agents := []AgentConfig{
		{
			Name:       "Claude Code",
			ID:         "claude-code",
			ConfigPath: filepath.Join(home, ".claude", "settings.json"),
			ConfigType: "settings_json",
		},
		{
			Name:       "Cursor",
			ID:         "cursor",
			ConfigPath: filepath.Join(home, ".cursor", "mcp.json"),
			ConfigType: "mcp_json",
		},
		{
			Name:       "Windsurf",
			ID:         "windsurf",
			ConfigPath: filepath.Join(home, ".codeium", "windsurf", "mcp_config.json"),
			ConfigType: "mcp_json",
		},
		{
			Name:       "VS Code (Copilot)",
			ID:         "vscode",
			ConfigPath: vscodeMCPPath(home),
			ConfigType: "mcp_json",
		},
	}

	for i := range agents {
		agents[i].Installed = agentInstalled(agents[i])
		agents[i].Connected = agentConnected(agents[i])
	}

	return agents
}

// ConfigureAgents detects agents and offers to connect them. Interactive prompts.
func ConfigureAgents() error {
	agents := DetectAgents()

	found := false
	for _, a := range agents {
		if a.Installed {
			found = true
		}
	}

	if !found {
		fmt.Println("  No coding agents detected. You can add them later via the dashboard.")
		return nil
	}

	fmt.Println("\n  Detected coding agents:")
	for _, a := range agents {
		if !a.Installed {
			continue
		}
		status := "not connected"
		if a.Connected {
			status = "connected"
		}
		fmt.Printf("    %s — %s\n", a.Name, status)
	}

	fmt.Println("")
	for _, a := range agents {
		if !a.Installed {
			continue
		}
		if a.Connected {
			continue
		}
		if askPermission(fmt.Sprintf("  Connect %s to OpenLoom? [Y/n] ", a.Name)) {
			if err := ConnectAgent(a); err != nil {
				fmt.Printf("    Failed to configure %s: %v\n", a.Name, err)
			} else {
				fmt.Printf("    %s: connected\n", a.Name)
			}
		}
	}

	return nil
}

// ConnectAgent writes MCP config for a single agent.
func ConnectAgent(agent AgentConfig) error {
	switch agent.ConfigType {
	case "mcp_json":
		return writeMCPJSON(agent.ConfigPath)
	case "settings_json":
		return writeSettingsJSON(agent.ConfigPath)
	default:
		return fmt.Errorf("unknown config type: %s", agent.ConfigType)
	}
}

// DisconnectAgent removes MCP config for a single agent.
func DisconnectAgent(agent AgentConfig) error {
	switch agent.ConfigType {
	case "mcp_json":
		return removeMCPJSON(agent.ConfigPath)
	case "settings_json":
		return removeSettingsJSON(agent.ConfigPath)
	default:
		return fmt.Errorf("unknown config type: %s", agent.ConfigType)
	}
}

// writeMCPJSON writes or merges into a .mcp.json / mcp.json file.
func writeMCPJSON(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	config := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &config); err != nil {
			slog.Warn("unmarshal mcp config failed", "path", path, "error", err)
		}
	}

	servers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		servers = make(map[string]any)
	}
	servers["openloom"] = mcpEntry()
	config["mcpServers"] = servers

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// writeSettingsJSON merges MCP config and hooks into Claude Code's settings.json.
func writeSettingsJSON(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	config := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &config); err != nil {
			slog.Warn("unmarshal settings config failed", "path", path, "error", err)
		}
	}

	// Claude Code uses mcpServers at top level in settings.json
	servers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		servers = make(map[string]any)
	}
	servers["openloom"] = mcpEntry()
	config["mcpServers"] = servers

	// Add hooks for automatic conversation capture
	hookEntry := map[string]any{
		"type":    "http",
		"url":     "http://127.0.0.1:8765/api/hook",
		"timeout": 10,
	}
	hookWrapper := []any{map[string]any{"hooks": []any{hookEntry}}}

	hooks, ok := config["hooks"].(map[string]any)
	if !ok {
		hooks = make(map[string]any)
	}
	hooks["UserPromptSubmit"] = hookWrapper
	hooks["PostToolUse"] = []any{map[string]any{
		"matcher": "*",
		"hooks":   []any{hookEntry},
	}}
	hooks["Stop"] = hookWrapper
	hooks["SessionEnd"] = hookWrapper
	config["hooks"] = hooks

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// removeMCPJSON removes openloom from an mcp.json file.
func removeMCPJSON(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil // file doesn't exist, nothing to remove
	}

	config := make(map[string]any)
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	if servers, ok := config["mcpServers"].(map[string]any); ok {
		delete(servers, "openloom")
		config["mcpServers"] = servers
	}

	out, _ := json.MarshalIndent(config, "", "  ")
	return os.WriteFile(path, append(out, '\n'), 0644)
}

// removeSettingsJSON removes openloom from Claude Code settings.json.
func removeSettingsJSON(path string) error {
	return removeMCPJSON(path) // Same structure
}

func agentInstalled(a AgentConfig) bool {
	// Check if the agent's config directory exists
	dir := filepath.Dir(a.ConfigPath)
	if _, err := os.Stat(dir); err == nil {
		return true
	}

	// Also check for the binary
	switch a.ID {
	case "claude-code":
		_, err := findExecutable("claude")
		return err == nil
	case "cursor":
		_, err := findExecutable("cursor")
		return err == nil
	case "windsurf":
		_, err := findExecutable("windsurf")
		return err == nil
	}
	return false
}

func agentConnected(a AgentConfig) bool {
	data, err := os.ReadFile(a.ConfigPath)
	if err != nil {
		return false
	}

	config := make(map[string]any)
	if err := json.Unmarshal(data, &config); err != nil {
		return false
	}

	servers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		return false
	}

	_, exists := servers["openloom"]
	return exists
}

func findExecutable(name string) (string, error) {
	// Check PATH
	path, err := findInPath(name)
	if err == nil {
		return path, nil
	}

	// Check common locations
	home, _ := os.UserHomeDir()
	locations := []string{
		"/usr/local/bin/" + name,
		"/opt/homebrew/bin/" + name,
		filepath.Join(home, ".local", "bin", name),
	}
	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc, nil
		}
	}
	return "", fmt.Errorf("not found")
}

func findInPath(name string) (string, error) {
	dirs := filepath.SplitList(os.Getenv("PATH"))
	for _, dir := range dirs {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("not in PATH")
}

func vscodeMCPPath(home string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Code", "User", "mcp.json")
	case "linux":
		return filepath.Join(home, ".config", "Code", "User", "mcp.json")
	default:
		return filepath.Join(home, "AppData", "Roaming", "Code", "User", "mcp.json")
	}
}
