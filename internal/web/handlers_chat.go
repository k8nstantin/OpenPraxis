package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"openloom/internal/chat"
	"openloom/internal/node"

	"github.com/gorilla/mux"
)

func apiChatModels(router *chat.Router) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"models":    router.Models(),
			"providers": router.AvailableProviders(),
		})
	}
}

func apiChatSessionList(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessions, err := n.ChatSessions.List(50)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if sessions == nil {
			sessions = []*chat.Session{}
		}
		writeJSON(w, sessions)
	}
}

func apiChatSessionCreate(n *node.Node, router *chat.Router) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		model := n.Config.Chat.DefaultModel
		if model == "" {
			// Pick the first available model
			models := router.Models()
			if len(models) > 0 {
				model = models[0].ID
			} else {
				model = "ollama/llama3.1:8b"
			}
		}
		sess, err := n.ChatSessions.Create(model, n.Config.Node.PeerID())
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, sess)
	}
}

func apiChatSessionGet(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		sess, err := n.ChatSessions.Get(id)
		if err != nil {
			http.Error(w, "session not found", 404)
			return
		}
		writeJSON(w, sess)
	}
}

func apiChatSessionDelete(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if err := n.ChatSessions.Delete(id); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "deleted"})
	}
}

func apiChatSessionReset(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if err := n.ChatSessions.Reset(id); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "reset"})
	}
}

func apiChatSessionUpdateTitle(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		var body struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", 400)
			return
		}
		if err := n.ChatSessions.UpdateTitle(id, body.Title); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "updated"})
	}
}

func apiChatSessionUpdateModel(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", 400)
			return
		}
		if err := n.ChatSessions.UpdateModel(id, body.Model); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "updated"})
	}
}

func apiChatSessionUpdateThinking(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		var body struct {
			Level string `json:"level"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", 400)
			return
		}
		if err := n.ChatSessions.UpdateThinking(id, body.Level); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "updated"})
	}
}

func apiChatSessionRestore(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if err := n.ChatSessions.Restore(id); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "restored"})
	}
}

func apiChatSend(n *node.Node, router *chat.Router, ctxBuilder *chat.ContextBuilder, tools *chat.ChatTools) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SessionID   string            `json:"session_id"`
			Message     string            `json:"message"`
			Attachments []chat.Attachment `json:"attachments"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", 400)
			return
		}
		if req.SessionID == "" || req.Message == "" {
			http.Error(w, "session_id and message required", 400)
			return
		}

		sess, err := n.ChatSessions.Get(req.SessionID)
		if err != nil {
			http.Error(w, "session not found", 404)
			return
		}

		// Handle slash commands
		if strings.HasPrefix(req.Message, "/") {
			result := handleChatCommand(n, sess, req.Message, router)
			// Save command as user message + response as assistant message
			n.ChatSessions.AppendMessage(sess.ID, chat.Message{Role: "user", Content: req.Message})
			n.ChatSessions.AppendMessage(sess.ID, chat.Message{Role: "assistant", Content: result})

			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			flusher, _ := w.(http.Flusher)
			writeSSE(w, flusher, chat.StreamChunk{Type: "text", Content: result})
			writeSSE(w, flusher, chat.StreamChunk{Type: "done", Usage: &chat.Usage{}, Cost: 0})
			return
		}

		// Build context system prompt
		ctx := r.Context()
		systemPrompt := ctxBuilder.Build(ctx, req.Message)

		// Add user message to session
		userMsg := chat.Message{Role: "user", Content: req.Message, Attachments: req.Attachments}
		n.ChatSessions.AppendMessage(sess.ID, userMsg)

		// Reload session to get all messages including the new one
		sess, _ = n.ChatSessions.Get(req.SessionID)

		// Build chat request
		chatReq := chat.ChatRequest{
			Messages:      sess.Messages,
			SystemPrompt:  systemPrompt,
			Tools:         tools.Definitions(),
			Model:         sess.Model,
			ThinkingLevel: sess.ThinkingLevel,
		}

		// Set up SSE streaming
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", 500)
			return
		}

		// Stream the response with tool call loop
		streamChatResponse(ctx, w, flusher, n, sess, chatReq, router, tools)
	}
}

func streamChatResponse(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, n *node.Node, sess *chat.Session, chatReq chat.ChatRequest, router *chat.Router, tools *chat.ChatTools) {
	maxToolRounds := 5
	var fullText strings.Builder
	var totalUsage chat.Usage

	for round := 0; round <= maxToolRounds; round++ {
		stream, err := router.Chat(ctx, chatReq)
		if err != nil {
			writeSSE(w, flusher, chat.StreamChunk{Type: "error", Error: err.Error()})
			return
		}

		var pendingToolCalls []chat.ToolCall
		var roundText strings.Builder

		for chunk := range stream {
			switch chunk.Type {
			case "text":
				roundText.WriteString(chunk.Content)
				fullText.WriteString(chunk.Content)
				writeSSE(w, flusher, chunk)
			case "thinking":
				writeSSE(w, flusher, chunk)
			case "tool_call":
				if chunk.ToolCall != nil {
					pendingToolCalls = append(pendingToolCalls, *chunk.ToolCall)
					writeSSE(w, flusher, chunk)
				}
			case "done":
				if chunk.Usage != nil {
					totalUsage.InputTokens += chunk.Usage.InputTokens
					totalUsage.OutputTokens += chunk.Usage.OutputTokens
				}
			case "error":
				writeSSE(w, flusher, chunk)
				return
			}
		}

		// If there were tool calls, execute them and continue
		if len(pendingToolCalls) > 0 {
			// Save assistant message with tool calls
			assistantMsg := chat.Message{
				Role:      "assistant",
				Content:   roundText.String(),
				ToolCalls: pendingToolCalls,
			}
			chatReq.Messages = append(chatReq.Messages, assistantMsg)

			// Execute each tool call
			for _, tc := range pendingToolCalls {
				result, err := tools.Execute(ctx, tc.Name, tc.Input)
				if err != nil {
					result = fmt.Sprintf("Error: %v", err)
				}
				writeSSE(w, flusher, chat.StreamChunk{
					Type:       "tool_result",
					ToolResult: &chat.ToolCallResult{ID: tc.ID, Name: tc.Name, Result: result},
				})

				// Add tool result to messages
				toolMsg := chat.Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: tc.ID,
				}
				chatReq.Messages = append(chatReq.Messages, toolMsg)
			}
			continue // Next round with tool results
		}

		// No tool calls — we're done
		break
	}

	// Save assistant response
	n.ChatSessions.AppendMessage(sess.ID, chat.Message{
		Role:    "assistant",
		Content: fullText.String(),
	})

	// Compute cost
	model, found := router.FindModel(sess.Model)
	var cost float64
	if found {
		cost = chat.ComputeCost(model, totalUsage)
	}
	n.ChatSessions.AddTokens(sess.ID, totalUsage.InputTokens, totalUsage.OutputTokens, cost)

	// Auto-generate title from first message
	if sess.Title == "New Chat" && len(sess.Messages) <= 1 {
		title := sess.Messages[0].Content
		if len(title) > 50 {
			title = title[:50] + "..."
		}
		n.ChatSessions.UpdateTitle(sess.ID, title)
	}

	writeSSE(w, flusher, chat.StreamChunk{
		Type:  "done",
		Usage: &totalUsage,
		Cost:  cost,
	})
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, chunk chat.StreamChunk) {
	data, _ := json.Marshal(chunk)
	fmt.Fprintf(w, "data: %s\n\n", data)
	if flusher != nil {
		flusher.Flush()
	}
}

func handleChatCommand(n *node.Node, sess *chat.Session, cmd string, router *chat.Router) string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "Unknown command"
	}
	switch parts[0] {
	case "/status":
		memCount, _ := n.Index.Count()
		convCount, _ := n.Conversations.Count()
		manifests, _ := n.Manifests.List("open", 100)
		tasks, _ := n.Tasks.List("running", 100)
		return fmt.Sprintf("Memories: %d | Conversations: %d | Active Manifests: %d | Running Tasks: %d | Model: %s",
			memCount, convCount, len(manifests), len(tasks), sess.Model)
	case "/reset":
		n.ChatSessions.Reset(sess.ID)
		return "Session cleared."
	case "/think":
		level := "medium"
		if len(parts) > 1 {
			level = parts[1]
		}
		n.ChatSessions.UpdateThinking(sess.ID, level)
		return fmt.Sprintf("Thinking level set to: %s", level)
	case "/usage":
		return fmt.Sprintf("Session: %s\nTokens In: %d | Tokens Out: %d | Cost: $%.4f\nModel: %s",
			sess.ID[:12], sess.TokensIn, sess.TokensOut, sess.CostUSD, sess.Model)
	case "/models":
		models := router.Models()
		var out string
		for _, m := range models {
			marker := " "
			if m.ID == sess.Model {
				marker = "*"
			}
			out += fmt.Sprintf("%s %s (%s) — $%.2f/$%.2f per 1M tokens\n", marker, m.Name, m.ID, m.CostPerMIn, m.CostPerMOut)
		}
		return out
	default:
		return fmt.Sprintf("Unknown command: %s\nAvailable: /status, /reset, /think [off|low|medium|high], /usage, /models", parts[0])
	}
}

// --- Chat Settings API ---

func apiChatSettingsGet(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := n.Config
		providers := map[string]map[string]any{
			"anthropic": {
				"has_key":  cfg.Chat.Providers.Anthropic.EnvKey() != "",
				"from_env": cfg.Chat.Providers.Anthropic.APIKey == "" && cfg.Chat.Providers.Anthropic.EnvKey() != "",
				"env_var":  "ANTHROPIC_API_KEY",
			},
			"google": {
				"has_key":  cfg.Chat.Providers.Google.EnvKey() != "",
				"from_env": cfg.Chat.Providers.Google.APIKey == "" && cfg.Chat.Providers.Google.EnvKey() != "",
				"env_var":  "GOOGLE_API_KEY",
			},
			"openai": {
				"has_key":  cfg.Chat.Providers.OpenAI.EnvKey() != "",
				"from_env": cfg.Chat.Providers.OpenAI.APIKey == "" && cfg.Chat.Providers.OpenAI.EnvKey() != "",
				"env_var":  "OPENAI_API_KEY",
			},
			"ollama": {
				"has_key":  true,
				"host":     cfg.Chat.Providers.Ollama.Host,
				"from_env": false,
			},
		}
		writeJSON(w, map[string]any{
			"default_model": cfg.Chat.DefaultModel,
			"providers":     providers,
		})
	}
}

func apiChatSettingsUpdate(n *node.Node, router *chat.Router) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DefaultModel string `json:"default_model"`
			Anthropic    string `json:"anthropic_key"`
			Google       string `json:"google_key"`
			OpenAI       string `json:"openai_key"`
			OllamaHost   string `json:"ollama_host"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", 400)
			return
		}

		cfg := n.Config

		// Only update fields that are provided (non-empty)
		if req.DefaultModel != "" {
			cfg.Chat.DefaultModel = req.DefaultModel
		}
		if req.Anthropic != "" {
			cfg.Chat.Providers.Anthropic.APIKey = req.Anthropic
		}
		if req.Google != "" {
			cfg.Chat.Providers.Google.APIKey = req.Google
		}
		if req.OpenAI != "" {
			cfg.Chat.Providers.OpenAI.APIKey = req.OpenAI
		}
		if req.OllamaHost != "" {
			cfg.Chat.Providers.Ollama.Host = req.OllamaHost
		}

		if err := cfg.Save(); err != nil {
			http.Error(w, "save config: "+err.Error(), 500)
			return
		}

		// Re-register providers with new keys
		router.ConfigureProviders(cfg)

		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func apiChatSettingsTest() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Provider string `json:"provider"`
			APIKey   string `json:"api_key"`
			Host     string `json:"host"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", 400)
			return
		}

		var testErr error
		switch req.Provider {
		case "anthropic":
			testErr = testAnthropicKey(req.APIKey)
		case "google":
			testErr = testGoogleKey(req.APIKey)
		case "openai":
			testErr = testOpenAIKey(req.APIKey)
		case "ollama":
			host := req.Host
			if host == "" {
				host = "http://localhost:11434"
			}
			testErr = testOllamaConnection(host)
		default:
			http.Error(w, "unknown provider", 400)
			return
		}

		if testErr != nil {
			writeJSON(w, map[string]any{"valid": false, "error": testErr.Error()})
		} else {
			writeJSON(w, map[string]any{"valid": true})
		}
	}
}

func testAnthropicKey(apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("no API key provided")
	}
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", strings.NewReader(`{"model":"claude-haiku-4-5-20251001","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return fmt.Errorf("invalid API key")
	}
	if resp.StatusCode >= 400 && resp.StatusCode != 429 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func testGoogleKey(apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("no API key provided")
	}
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models?key=%s", apiKey)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("connection failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 400 || resp.StatusCode == 403 {
		return fmt.Errorf("invalid API key")
	}
	if resp.StatusCode >= 400 && resp.StatusCode != 429 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func testOpenAIKey(apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("no API key provided")
	}
	req, _ := http.NewRequest("GET", "https://api.openai.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return fmt.Errorf("invalid API key")
	}
	if resp.StatusCode >= 400 && resp.StatusCode != 429 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func testOllamaConnection(host string) error {
	resp, err := http.Get(host + "/api/tags")
	if err != nil {
		return fmt.Errorf("cannot connect to Ollama at %s: %v", host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Ollama returned HTTP %d", resp.StatusCode)
	}
	return nil
}
