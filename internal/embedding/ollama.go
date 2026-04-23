package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Engine handles embedding generation via Ollama's local API.
// Immutable-text embedding results are cached in-memory keyed by model+text
// so repeat calls (visceral rules + manifest titles hit on every tool-call
// hook) don't round-trip to Ollama every time. Cached entries live until
// the process exits; rules/manifest text are effectively immutable per
// row-id so cache hit rate is near 100% after the first warmup.
type Engine struct {
	baseURL   string
	model     string
	dimension int
	client    *http.Client

	cacheMu sync.RWMutex
	cache   map[string][]float32
}

// NewEngine creates an embedding engine that talks to Ollama.
func NewEngine(ollamaURL, model string, dimension int) *Engine {
	return &Engine{
		baseURL:   ollamaURL,
		model:     model,
		dimension: dimension,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: make(map[string][]float32),
	}
}

// Embed generates an embedding vector for the given text, serving repeat
// calls for the same text from an in-memory cache. Kills the "embed every
// visceral rule / every manifest title per PostToolUse hook" pattern that
// pegged the serve at 500%+ CPU on tool-call bursts.
func (e *Engine) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return make([]float32, e.dimension), nil
	}

	// Cache key includes model so a model swap invalidates naturally.
	key := e.model + "\x00" + text
	e.cacheMu.RLock()
	if v, ok := e.cache[key]; ok {
		e.cacheMu.RUnlock()
		return v, nil
	}
	e.cacheMu.RUnlock()

	body := embedRequest{
		Model: e.model,
		Input: text,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/api/embed", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result embedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(result.Embeddings) == 0 {
		return nil, fmt.Errorf("ollama returned no embeddings")
	}

	vec := result.Embeddings[0]
	if len(vec) != e.dimension {
		return nil, fmt.Errorf("expected %d dimensions, got %d", e.dimension, len(vec))
	}

	e.cacheMu.Lock()
	e.cache[key] = vec
	e.cacheMu.Unlock()
	return vec, nil
}

// EmbedDocument prefixes text with "search_document: " for nomic-embed-text.
func (e *Engine) EmbedDocument(ctx context.Context, text string) ([]float32, error) {
	return e.Embed(ctx, "search_document: "+text)
}

// EmbedQuery prefixes text with "search_query: " for nomic-embed-text.
func (e *Engine) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return e.Embed(ctx, "search_query: "+text)
}

// Dimension returns the configured embedding dimension.
func (e *Engine) Dimension() int {
	return e.dimension
}

// Healthy checks if Ollama is reachable and the model is available.
func (e *Engine) Healthy(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", e.baseURL+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama unreachable at %s: %w", e.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned %d", resp.StatusCode)
	}
	return nil
}

type embedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}
