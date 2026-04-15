package chat

import (
	"context"
	"fmt"
)

// Provider is the interface all model providers must implement.
type Provider interface {
	// Chat sends a message and returns a channel of streaming chunks.
	Chat(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error)
	// Models returns the list of models this provider supports.
	Models() []Model
	// Name returns the provider identifier (e.g. "anthropic", "ollama").
	Name() string
}

// Model describes a single model offered by a provider.
type Model struct {
	ID           string  `json:"id"`            // e.g. "anthropic/claude-sonnet-4-6"
	Name         string  `json:"name"`          // e.g. "Claude Sonnet 4.6"
	Provider     string  `json:"provider"`      // e.g. "anthropic"
	CostPerMIn   float64 `json:"cost_per_m_in"` // USD per 1M input tokens
	CostPerMOut  float64 `json:"cost_per_m_out"`// USD per 1M output tokens
	MaxContext   int     `json:"max_context"`
	Multimodal   bool    `json:"multimodal"`
	ToolUse      bool    `json:"tool_use"`
}

// ChatRequest is sent to a provider to generate a response.
type ChatRequest struct {
	Messages      []Message     `json:"messages"`
	SystemPrompt  string        `json:"system_prompt"`
	Tools         []ToolDef     `json:"tools,omitempty"`
	Model         string        `json:"model"`
	ThinkingLevel string        `json:"thinking_level"` // "off", "low", "medium", "high"
	MaxTokens     int           `json:"max_tokens"`
}

// Message is a single turn in a conversation.
type Message struct {
	Role       string       `json:"role"`    // "user", "assistant", "tool"
	Content    string       `json:"content"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall   `json:"tool_calls,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// Attachment is an image or file attached to a message.
type Attachment struct {
	Type     string `json:"type"`      // "image"
	MimeType string `json:"mime_type"` // "image/png", "image/jpeg"
	Base64   string `json:"base64"`
}

// ToolCall represents a function call made by the model.
type ToolCall struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input string `json:"input"` // JSON string
}

// ToolDef defines a tool the model can call.
type ToolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema string `json:"input_schema"` // JSON Schema
}

// StreamChunk is one piece of a streaming response.
type StreamChunk struct {
	Type       string    `json:"type"`                  // "text", "tool_call", "tool_result", "thinking", "done", "error"
	Content    string    `json:"content,omitempty"`
	ToolCall   *ToolCall `json:"tool_call,omitempty"`
	ToolResult *ToolCallResult `json:"tool_result,omitempty"`
	Usage      *Usage    `json:"usage,omitempty"`       // only on "done"
	Cost       float64   `json:"cost,omitempty"`        // only on "done"
	Error      string    `json:"error,omitempty"`       // only on "error"
}

// ToolCallResult is returned after executing a tool.
type ToolCallResult struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Result string `json:"result"`
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ComputeCost calculates USD cost from token counts and model pricing.
func ComputeCost(model Model, usage Usage) float64 {
	inCost := float64(usage.InputTokens) / 1_000_000 * model.CostPerMIn
	outCost := float64(usage.OutputTokens) / 1_000_000 * model.CostPerMOut
	return inCost + outCost
}

// ProviderError wraps provider-specific errors with context.
type ProviderError struct {
	Provider string
	Message  string
	Err      error
}

func (e *ProviderError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Provider, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Provider, e.Message)
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}
