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

const googleAPIURL = "https://generativelanguage.googleapis.com/v1beta/models"

// GoogleProvider implements the Provider interface for Gemini models.
type GoogleProvider struct {
	apiKey string
	client *http.Client
}

func NewGoogleProvider(apiKey string) *GoogleProvider {
	return &GoogleProvider{
		apiKey: apiKey,
		client: &http.Client{},
	}
}

func (p *GoogleProvider) Name() string { return "google" }

func (p *GoogleProvider) Models() []Model {
	return []Model{
		{
			ID: "google/gemini-2.5-pro", Name: "Gemini 2.5 Pro", Provider: "google",
			CostPerMIn: 1.25, CostPerMOut: 10.0, MaxContext: 1048576,
			Multimodal: true, ToolUse: true,
		},
		{
			ID: "google/gemini-2.5-flash", Name: "Gemini 2.5 Flash", Provider: "google",
			CostPerMIn: 0.15, CostPerMOut: 0.60, MaxContext: 1048576,
			Multimodal: true, ToolUse: true,
		},
	}
}

func (p *GoogleProvider) Chat(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error) {
	if p.apiKey == "" {
		return nil, &ProviderError{Provider: "google", Message: "API key not configured"}
	}

	modelID := req.Model
	if strings.HasPrefix(modelID, "google/") {
		modelID = modelID[len("google/"):]
	}

	// Build Gemini request
	contents := buildGeminiContents(req.SystemPrompt, req.Messages)

	body := map[string]any{
		"contents": contents,
	}

	// Generation config
	genConfig := map[string]any{}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	genConfig["maxOutputTokens"] = maxTokens
	body["generationConfig"] = genConfig

	// Add tools if provided
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			var params any
			if err := json.Unmarshal([]byte(t.InputSchema), &params); err != nil {
				log.Printf("WARNING: unmarshal google tool schema for %s: %v", t.Name, err)
			}
			tools = append(tools, map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  params,
			})
		}
		body["tools"] = []map[string]any{
			{"functionDeclarations": tools},
		}
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s:streamGenerateContent?alt=sse&key=%s", googleAPIURL, modelID, p.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{Provider: "google", Message: "request failed", Err: err}
	}

	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &ProviderError{Provider: "google", Message: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(data))}
	}

	ch := make(chan StreamChunk, 64)
	go p.streamResponse(resp, ch)
	return ch, nil
}

func (p *GoogleProvider) streamResponse(resp *http.Response, ch chan<- StreamChunk) {
	defer close(ch)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	var totalIn, totalOut int

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := line[6:]

		var event struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text         string `json:"text"`
						FunctionCall *struct {
							Name string          `json:"name"`
							Args json.RawMessage `json:"args"`
						} `json:"functionCall"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
			UsageMetadata struct {
				PromptTokenCount     int `json:"promptTokenCount"`
				CandidatesTokenCount int `json:"candidatesTokenCount"`
			} `json:"usageMetadata"`
		}

		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		// Track usage
		if event.UsageMetadata.PromptTokenCount > 0 {
			totalIn = event.UsageMetadata.PromptTokenCount
		}
		if event.UsageMetadata.CandidatesTokenCount > 0 {
			totalOut = event.UsageMetadata.CandidatesTokenCount
		}

		for _, candidate := range event.Candidates {
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					ch <- StreamChunk{Type: "text", Content: part.Text}
				}
				if part.FunctionCall != nil {
					argsJSON := string(part.FunctionCall.Args)
					ch <- StreamChunk{
						Type: "tool_call",
						ToolCall: &ToolCall{
							ID:    fmt.Sprintf("gc_%s_%d", part.FunctionCall.Name, totalOut),
							Name:  part.FunctionCall.Name,
							Input: argsJSON,
						},
					}
				}
			}
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

func buildGeminiContents(systemPrompt string, msgs []Message) []map[string]any {
	var contents []map[string]any

	// System instruction as first user message if present
	if systemPrompt != "" {
		contents = append(contents, map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"text": systemPrompt}},
		})
		contents = append(contents, map[string]any{
			"role":  "model",
			"parts": []map[string]any{{"text": "Understood. I'll follow these instructions."}},
		})
	}

	for _, msg := range msgs {
		role := "user"
		if msg.Role == "assistant" {
			role = "model"
		}

		// Tool results
		if msg.Role == "tool" {
			contents = append(contents, map[string]any{
				"role": "user",
				"parts": []map[string]any{
					{
						"functionResponse": map[string]any{
							"name": msg.ToolCallID,
							"response": map[string]any{
								"content": msg.Content,
							},
						},
					},
				},
			})
			continue
		}

		// Assistant messages with tool calls
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			parts := []map[string]any{}
			if msg.Content != "" {
				parts = append(parts, map[string]any{"text": msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				var args any
				if err := json.Unmarshal([]byte(tc.Input), &args); err != nil {
					log.Printf("WARNING: unmarshal google tool call args for %s: %v", tc.Name, err)
				}
				parts = append(parts, map[string]any{
					"functionCall": map[string]any{
						"name": tc.Name,
						"args": args,
					},
				})
			}
			contents = append(contents, map[string]any{"role": "model", "parts": parts})
			continue
		}

		parts := []map[string]any{{"text": msg.Content}}
		contents = append(contents, map[string]any{"role": role, "parts": parts})
	}

	return contents
}
