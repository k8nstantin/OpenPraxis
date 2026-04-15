package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// OllamaProvider implements the Provider interface for local Ollama models.
type OllamaProvider struct {
	host   string
	client *http.Client
}

func NewOllamaProvider(host string) *OllamaProvider {
	if host == "" {
		host = "http://localhost:11434"
	}
	return &OllamaProvider{
		host:   strings.TrimRight(host, "/"),
		client: &http.Client{},
	}
}

func (p *OllamaProvider) Name() string { return "ollama" }

func (p *OllamaProvider) Models() []Model {
	// Query Ollama for available models, return known ones
	models := []Model{
		{
			ID: "ollama/llama3.1:8b", Name: "Llama 3.1 8B", Provider: "ollama",
			CostPerMIn: 0, CostPerMOut: 0, MaxContext: 128000,
			Multimodal: false, ToolUse: true,
		},
		{
			ID: "ollama/llama3.1:70b", Name: "Llama 3.1 70B", Provider: "ollama",
			CostPerMIn: 0, CostPerMOut: 0, MaxContext: 128000,
			Multimodal: false, ToolUse: true,
		},
		{
			ID: "ollama/qwen2.5:14b", Name: "Qwen 2.5 14B", Provider: "ollama",
			CostPerMIn: 0, CostPerMOut: 0, MaxContext: 32768,
			Multimodal: false, ToolUse: true,
		},
		{
			ID: "ollama/mistral:7b", Name: "Mistral 7B", Provider: "ollama",
			CostPerMIn: 0, CostPerMOut: 0, MaxContext: 32768,
			Multimodal: false, ToolUse: false,
		},
		{
			ID: "ollama/llava:13b", Name: "LLaVA 13B", Provider: "ollama",
			CostPerMIn: 0, CostPerMOut: 0, MaxContext: 4096,
			Multimodal: true, ToolUse: false,
		},
	}

	// Check which models are actually available
	available := p.listLocalModels()
	if len(available) > 0 {
		var filtered []Model
		for _, m := range models {
			modelTag := strings.TrimPrefix(m.ID, "ollama/")
			for _, a := range available {
				if a == modelTag || strings.HasPrefix(a, strings.Split(modelTag, ":")[0]) {
					filtered = append(filtered, m)
					break
				}
			}
		}
		// Also add any locally available models not in our list
		for _, a := range available {
			found := false
			for _, m := range models {
				if strings.TrimPrefix(m.ID, "ollama/") == a {
					found = true
					break
				}
			}
			if !found {
				filtered = append(filtered, Model{
					ID: "ollama/" + a, Name: a, Provider: "ollama",
					CostPerMIn: 0, CostPerMOut: 0, MaxContext: 32768,
				})
			}
		}
		if len(filtered) > 0 {
			return filtered
		}
	}

	return models
}

// embeddingModels lists known embedding-only model families that should not appear in the chat picker.
var embeddingModels = []string{
	"nomic-embed", "mxbai-embed", "all-minilm", "snowflake-arctic-embed",
	"bge-", "gte-", "e5-", "paraphrase-",
}

func isEmbeddingModel(name string) bool {
	lower := strings.ToLower(name)
	for _, prefix := range embeddingModels {
		if strings.Contains(lower, prefix) {
			return true
		}
	}
	return false
}

func (p *OllamaProvider) listLocalModels() []string {
	resp, err := p.client.Get(p.host + "/api/tags")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	var names []string
	for _, m := range result.Models {
		if !isEmbeddingModel(m.Name) {
			names = append(names, m.Name)
		}
	}
	return names
}

func (p *OllamaProvider) Chat(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error) {
	modelID := req.Model
	if strings.HasPrefix(modelID, "ollama/") {
		modelID = modelID[len("ollama/"):]
	}

	// Build Ollama chat request
	body := map[string]any{
		"model":  modelID,
		"stream": true,
	}

	msgs := buildOllamaMessages(req.SystemPrompt, req.Messages)
	body["messages"] = msgs

	// Add tools if provided
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			var params any
			if err := json.Unmarshal([]byte(t.InputSchema), &params); err != nil {
				log.Printf("WARNING: unmarshal ollama tool schema for %s: %v", t.Name, err)
			}
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  params,
				},
			})
		}
		body["tools"] = tools
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.host+"/api/chat", bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{Provider: "ollama", Message: "request failed (is Ollama running?)", Err: err}
	}

	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &ProviderError{Provider: "ollama", Message: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(data))}
	}

	ch := make(chan StreamChunk, 64)
	go p.streamResponse(resp, ch)
	return ch, nil
}

func (p *OllamaProvider) streamResponse(resp *http.Response, ch chan<- StreamChunk) {
	defer close(ch)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	var totalIn, totalOut int

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var event struct {
			Message struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					Function struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			Done            bool `json:"done"`
			PromptEvalCount int  `json:"prompt_eval_count"`
			EvalCount       int  `json:"eval_count"`
		}

		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		// Handle tool calls
		if len(event.Message.ToolCalls) > 0 {
			for _, tc := range event.Message.ToolCalls {
				inputJSON := string(tc.Function.Arguments)
				ch <- StreamChunk{
					Type: "tool_call",
					ToolCall: &ToolCall{
						ID:    fmt.Sprintf("tc_%s_%d", tc.Function.Name, totalOut),
						Name:  tc.Function.Name,
						Input: inputJSON,
					},
				}
			}
		}

		// Handle text content
		if event.Message.Content != "" {
			ch <- StreamChunk{Type: "text", Content: event.Message.Content}
		}

		if event.Done {
			totalIn = event.PromptEvalCount
			totalOut = event.EvalCount
			break
		}
	}

	ch <- StreamChunk{
		Type: "done",
		Usage: &Usage{
			InputTokens:  totalIn,
			OutputTokens: totalOut,
		},
	}
}

func buildOllamaMessages(systemPrompt string, msgs []Message) []map[string]any {
	var result []map[string]any

	if systemPrompt != "" {
		result = append(result, map[string]any{
			"role":    "system",
			"content": systemPrompt,
		})
	}

	for _, msg := range msgs {
		m := map[string]any{
			"role":    msg.Role,
			"content": msg.Content,
		}

		// Tool results map to tool role
		if msg.Role == "tool" {
			m["role"] = "tool"
		}

		result = append(result, m)
	}
	return result
}
