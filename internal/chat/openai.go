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

const openaiAPIURL = "https://api.openai.com/v1/chat/completions"

// OpenAIProvider implements the Provider interface for GPT and o-series models.
type OpenAIProvider struct {
	apiKey string
	client *http.Client
}

func NewOpenAIProvider(apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		apiKey: apiKey,
		client: &http.Client{},
	}
}

func (p *OpenAIProvider) Name() string { return "openai" }

func (p *OpenAIProvider) Models() []Model {
	return []Model{
		{
			ID: "openai/gpt-4o", Name: "GPT-4o", Provider: "openai",
			CostPerMIn: 2.50, CostPerMOut: 10.0, MaxContext: 128000,
			Multimodal: true, ToolUse: true,
		},
		{
			ID: "openai/gpt-4o-mini", Name: "GPT-4o Mini", Provider: "openai",
			CostPerMIn: 0.15, CostPerMOut: 0.60, MaxContext: 128000,
			Multimodal: true, ToolUse: true,
		},
		{
			ID: "openai/o3", Name: "o3", Provider: "openai",
			CostPerMIn: 10.0, CostPerMOut: 40.0, MaxContext: 200000,
			Multimodal: false, ToolUse: true,
		},
		{
			ID: "openai/o3-mini", Name: "o3 Mini", Provider: "openai",
			CostPerMIn: 1.10, CostPerMOut: 4.40, MaxContext: 200000,
			Multimodal: false, ToolUse: true,
		},
	}
}

func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error) {
	if p.apiKey == "" {
		return nil, &ProviderError{Provider: "openai", Message: "API key not configured"}
	}

	modelID := req.Model
	if strings.HasPrefix(modelID, "openai/") {
		modelID = modelID[len("openai/"):]
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	body := map[string]any{
		"model":  modelID,
		"stream": true,
		"stream_options": map[string]any{
			"include_usage": true,
		},
	}

	// Build messages
	msgs := buildOpenAIMessages(req.SystemPrompt, req.Messages)
	body["messages"] = msgs

	// o-series models use max_completion_tokens instead of max_tokens
	if strings.HasPrefix(modelID, "o") {
		body["max_completion_tokens"] = maxTokens
	} else {
		body["max_tokens"] = maxTokens
	}

	// Add tools if provided
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			var params any
			if err := json.Unmarshal([]byte(t.InputSchema), &params); err != nil {
				log.Printf("WARNING: unmarshal openai tool schema for %s: %v", t.Name, err)
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

	httpReq, err := http.NewRequestWithContext(ctx, "POST", openaiAPIURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{Provider: "openai", Message: "request failed", Err: err}
	}

	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &ProviderError{Provider: "openai", Message: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(data))}
	}

	ch := make(chan StreamChunk, 64)
	go p.streamResponse(resp, ch)
	return ch, nil
}

func (p *OpenAIProvider) streamResponse(resp *http.Response, ch chan<- StreamChunk) {
	defer close(ch)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	var totalIn, totalOut int
	// Track tool call state across deltas
	toolCalls := map[int]*ToolCall{}

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := line[6:]
		if data == "[DONE]" {
			break
		}

		var event struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}

		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		// Track usage from stream_options include_usage
		if event.Usage != nil {
			totalIn = event.Usage.PromptTokens
			totalOut = event.Usage.CompletionTokens
		}

		for _, choice := range event.Choices {
			// Text content
			if choice.Delta.Content != "" {
				ch <- StreamChunk{Type: "text", Content: choice.Delta.Content}
			}

			// Tool calls (streamed incrementally)
			for _, tc := range choice.Delta.ToolCalls {
				existing, ok := toolCalls[tc.Index]
				if !ok {
					existing = &ToolCall{ID: tc.ID, Name: tc.Function.Name}
					toolCalls[tc.Index] = existing
				}
				if tc.Function.Name != "" {
					existing.Name = tc.Function.Name
				}
				existing.Input += tc.Function.Arguments
			}
		}
	}

	// Emit all completed tool calls
	for _, tc := range toolCalls {
		ch <- StreamChunk{Type: "tool_call", ToolCall: tc}
	}

	ch <- StreamChunk{
		Type: "done",
		Usage: &Usage{
			InputTokens:  totalIn,
			OutputTokens: totalOut,
		},
	}
}

func buildOpenAIMessages(systemPrompt string, msgs []Message) []map[string]any {
	var result []map[string]any

	if systemPrompt != "" {
		result = append(result, map[string]any{
			"role":    "system",
			"content": systemPrompt,
		})
	}

	for _, msg := range msgs {
		// Tool results
		if msg.Role == "tool" && msg.ToolCallID != "" {
			result = append(result, map[string]any{
				"role":         "tool",
				"content":      msg.Content,
				"tool_call_id": msg.ToolCallID,
			})
			continue
		}

		// Assistant with tool calls
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			toolCalls := make([]map[string]any, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				toolCalls = append(toolCalls, map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": tc.Input,
					},
				})
			}
			m := map[string]any{
				"role":       "assistant",
				"tool_calls": toolCalls,
			}
			if msg.Content != "" {
				m["content"] = msg.Content
			}
			result = append(result, m)
			continue
		}

		result = append(result, map[string]any{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}

	return result
}
