package chat

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/k8nstantin/OpenPraxis/internal/config"
)

// Router resolves providers from model IDs and manages available models.
type Router struct {
	providers map[string]Provider
	models    []Model
	mu        sync.RWMutex
}

// NewRouter creates a router with providers configured from the app config.
func NewRouter(cfg *config.Config) *Router {
	r := &Router{
		providers: make(map[string]Provider),
	}
	r.ConfigureProviders(cfg)
	return r
}

// ConfigureProviders re-registers all providers based on current config.
// Called on startup and when API keys are updated via settings.
func (r *Router) ConfigureProviders(cfg *config.Config) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear existing providers
	r.providers = make(map[string]Provider)

	// Anthropic
	apiKey := cfg.Chat.Providers.Anthropic.APIKey
	if apiKey == "" {
		apiKey = cfg.Chat.Providers.Anthropic.EnvKey()
	}
	if apiKey != "" {
		r.providers["anthropic"] = NewAnthropicProvider(apiKey)
	}

	// Google (Gemini)
	googleKey := cfg.Chat.Providers.Google.APIKey
	if googleKey == "" {
		googleKey = cfg.Chat.Providers.Google.EnvKey()
	}
	if googleKey != "" {
		r.providers["google"] = NewGoogleProvider(googleKey)
	}

	// OpenAI
	openaiKey := cfg.Chat.Providers.OpenAI.APIKey
	if openaiKey == "" {
		openaiKey = cfg.Chat.Providers.OpenAI.EnvKey()
	}
	if openaiKey != "" {
		r.providers["openai"] = NewOpenAIProvider(openaiKey)
	}

	// Ollama (always available — free local models)
	ollamaHost := cfg.Chat.Providers.Ollama.Host
	if ollamaHost == "" {
		ollamaHost = cfg.Embedding.OllamaURL
	}
	r.providers["ollama"] = NewOllamaProvider(ollamaHost)

	r.refreshModels()
}

// RegisterProvider adds a provider to the router.
func (r *Router) RegisterProvider(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

// Chat routes a request to the appropriate provider based on the model ID.
func (r *Router) Chat(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error) {
	provider, err := r.resolveProvider(req.Model)
	if err != nil {
		return nil, err
	}
	return provider.Chat(ctx, req)
}

// Models returns all available models across all providers.
func (r *Router) Models() []Model {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.models
}

// FindModel looks up a model by ID.
func (r *Router) FindModel(id string) (Model, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, m := range r.models {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

// AvailableProviders returns the names of configured providers.
func (r *Router) AvailableProviders() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var names []string
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// RefreshModels rebuilds the model list from all providers.
func (r *Router) RefreshModels() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshModels()
}

func (r *Router) refreshModels() {
	r.models = nil
	for _, p := range r.providers {
		r.models = append(r.models, p.Models()...)
	}
}

func (r *Router) resolveProvider(modelID string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Extract provider name from model ID (e.g. "anthropic/claude-sonnet-4-6" -> "anthropic")
	parts := strings.SplitN(modelID, "/", 2)
	if len(parts) == 2 {
		if p, ok := r.providers[parts[0]]; ok {
			return p, nil
		}
	}

	// Try to find the model in any provider
	for _, p := range r.providers {
		for _, m := range p.Models() {
			if m.ID == modelID {
				return p, nil
			}
		}
	}

	return nil, fmt.Errorf("no provider found for model %q (available: %v)", modelID, r.AvailableProviders())
}
