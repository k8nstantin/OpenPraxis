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

const anthropicAPIURL = "https://api.anthropic.com/v1/messages"

// AnthropicProvider implements the Provider interface for Claude models.
type AnthropicProvider struct {
	apiKey string
	client *http.Client
}

func NewAnthropicProvider(apiKey string) *AnthropicProvider {
	return &AnthropicProvider{
		apiKey: apiKey,
		client: &http.Client{},
	}
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) Models() []Model {
	return []Model{
		{
			ID: "anthropic/claude-opus-4-6", Name: "Claude Opus 4.6", Provider: "anthropic",
			CostPerMIn: 15.0, CostPerMOut: 75.0, MaxContext: 200000,
			Multimodal: true, ToolUse: true,
		},
		{
			ID: "anthropic/claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Provider: "anthropic",
			CostPerMIn: 3.0, CostPerMOut: 15.0, MaxContext: 200000,
			Multimodal: true, ToolUse: true,
		},
		{
			ID: "anthropic/claude-haiku-4-5", Name: "Claude Haiku 4.5", Provider: "anthropic",
			CostPerMIn: 0.80, CostPerMOut: 4.0, MaxContext: 200000,
			Multimodal: true, ToolUse: true,
		},
	}
}

func (p *AnthropicProvider) Chat(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error) {
	if p.apiKey == "" {
		return nil, &ProviderError{Provider: "anthropic", Message: "API key not configured"}
	}

	// Extract model ID (strip provider prefix)
	modelID := req.Model
	if strings.HasPrefix(modelID, "anthropic/") {
		modelID = modelID[len("anthropic/"):]
	}
	// Convert slash-style to API model ID
	modelID = strings.ReplaceAll(modelID, "-4-6", "-4-6-20250414")
	modelID = strings.ReplaceAll(modelID, "-4-5", "-4-5-20251001")

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	body := map[string]any{
		"model":      modelID,
		"max_tokens": maxTokens,
		"stream":     true,
	}

	// Build messages
	msgs := buildAnthropicMessages(req.Messages)
	body["messages"] = msgs

	if req.SystemPrompt != "" {
		body["system"] = req.SystemPrompt
	}

	// Add tools if provided
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			var schema any
			if err := json.Unmarshal([]byte(t.InputSchema), &schema); err != nil {
				log.Printf("WARNING: unmarshal anthropic tool schema for %s: %v", t.Name, err)
			}
			tools = append(tools, map[string]any{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": schema,
			})
		}
		body["tools"] = tools
	}

	// Extended thinking
	if req.ThinkingLevel != "" && req.ThinkingLevel != "off" {
		budgetTokens := 1024
		switch req.ThinkingLevel {
		case "low":
			budgetTokens = 2048
		case "medium":
			budgetTokens = 8192
		case "high":
			budgetTokens = 32768
		}
		body["thinking"] = map[string]any{
			"type":          "enabled",
			"budget_tokens": budgetTokens,
		}
		// Thinking requires higher max_tokens
		if maxTokens < budgetTokens+1024 {
			body["max_tokens"] = budgetTokens + 4096
		}
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", anthropicAPIURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{Provider: "anthropic", Message: "request failed", Err: err}
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &ProviderError{Provider: "anthropic", Message: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))}
	}

	ch := make(chan StreamChunk, 64)
	go p.streamResponse(resp, ch)
	return ch, nil
}

func (p *AnthropicProvider) streamResponse(resp *http.Response, ch chan<- StreamChunk) {
	defer close(ch)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	var currentToolCall *ToolCall
	var toolInputBuf strings.Builder
	var totalIn, totalOut int

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := line[6:]
		if data == "[DONE]" {
			break
		}

		var event map[string]any
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		eventType, _ := event["type"].(string)

		switch eventType {
		case "content_block_start":
			cb, _ := event["content_block"].(map[string]any)
			if cb == nil {
				continue
			}
			blockType, _ := cb["type"].(string)
			if blockType == "tool_use" {
				name, _ := cb["name"].(string)
				id, _ := cb["id"].(string)
				currentToolCall = &ToolCall{ID: id, Name: name}
				toolInputBuf.Reset()
			}

		case "content_block_delta":
			delta, _ := event["delta"].(map[string]any)
			if delta == nil {
				continue
			}
			deltaType, _ := delta["type"].(string)

			switch deltaType {
			case "text_delta":
				text, _ := delta["text"].(string)
				if text != "" {
					ch <- StreamChunk{Type: "text", Content: text}
				}
			case "thinking_delta":
				text, _ := delta["thinking"].(string)
				if text != "" {
					ch <- StreamChunk{Type: "thinking", Content: text}
				}
			case "input_json_delta":
				partial, _ := delta["partial_json"].(string)
				toolInputBuf.WriteString(partial)
			}

		case "content_block_stop":
			if currentToolCall != nil {
				currentToolCall.Input = toolInputBuf.String()
				ch <- StreamChunk{Type: "tool_call", ToolCall: currentToolCall}
				currentToolCall = nil
				toolInputBuf.Reset()
			}

		case "message_delta":
			usage, _ := event["usage"].(map[string]any)
			if usage != nil {
				if v, ok := usage["output_tokens"].(float64); ok {
					totalOut = int(v)
				}
			}

		case "message_start":
			msg, _ := event["message"].(map[string]any)
			if msg != nil {
				if usage, ok := msg["usage"].(map[string]any); ok {
					if v, ok := usage["input_tokens"].(float64); ok {
						totalIn = int(v)
					}
				}
			}
		}
	}

	// Send final done chunk with usage
	ch <- StreamChunk{
		Type: "done",
		Usage: &Usage{
			InputTokens:  totalIn,
			OutputTokens: totalOut,
		},
	}
}

func buildAnthropicMessages(msgs []Message) []map[string]any {
	var result []map[string]any
	for _, msg := range msgs {
		m := map[string]any{
			"role": msg.Role,
		}

		// Handle tool results
		if msg.Role == "tool" && msg.ToolCallID != "" {
			m["role"] = "user"
			m["content"] = []map[string]any{
				{
					"type":        "tool_result",
					"tool_use_id": msg.ToolCallID,
					"content":     msg.Content,
				},
			}
			result = append(result, m)
			continue
		}

		// Handle messages with tool calls (assistant messages)
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			content := []map[string]any{}
			if msg.Content != "" {
				content = append(content, map[string]any{
					"type": "text",
					"text": msg.Content,
				})
			}
			for _, tc := range msg.ToolCalls {
				var input any
				if err := json.Unmarshal([]byte(tc.Input), &input); err != nil {
					log.Printf("WARNING: unmarshal anthropic tool call input for %s: %v", tc.Name, err)
				}
				content = append(content, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": input,
				})
			}
			m["content"] = content
			result = append(result, m)
			continue
		}

		// Handle multimodal messages
		if len(msg.Attachments) > 0 {
			content := []map[string]any{}
			for _, att := range msg.Attachments {
				if att.Type == "image" {
					content = append(content, map[string]any{
						"type": "image",
						"source": map[string]any{
							"type":       "base64",
							"media_type": att.MimeType,
							"data":       att.Base64,
						},
					})
				}
			}
			if msg.Content != "" {
				content = append(content, map[string]any{
					"type": "text",
					"text": msg.Content,
				})
			}
			m["content"] = content
			result = append(result, m)
			continue
		}

		m["content"] = msg.Content
		result = append(result, m)
	}
	return result
}
