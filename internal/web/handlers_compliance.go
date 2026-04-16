package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/k8nstantin/OpenPraxis/internal/action"
	"github.com/k8nstantin/OpenPraxis/internal/node"

	"github.com/gorilla/mux"
)

// checkVisceralCompliance compares an action against visceral rules using polarity-aware detection.
// Prohibition rules use embedding similarity (high similarity = violation).
// Instruction rules use forbidden pattern matching (using a forbidden tool = violation).
// Permission rules are skipped (they only authorize, never restrict).
func checkVisceralCompliance(n *node.Node, sessionID, toolName string, toolInput any) {
	rules, err := n.Index.ListByType("visceral", 100)
	if err != nil || len(rules) == 0 {
		return
	}

	// Build action description for embedding
	inputStr := ""
	switch v := toolInput.(type) {
	case string:
		inputStr = v
	case map[string]any:
		if cmd, ok := v["command"].(string); ok {
			inputStr = cmd
		} else if path, ok := v["file_path"].(string); ok {
			inputStr = path
		} else {
			b, _ := json.Marshal(v)
			inputStr = string(b)
		}
	}
	if inputStr == "" {
		return
	}

	actionDesc := fmt.Sprintf("%s: %s", toolName, inputStr)
	if len(actionDesc) > 500 {
		actionDesc = actionDesc[:500]
	}

	// Compare against each visceral rule using polarity-aware strategy
	ctx := context.Background()
	var actionVec []float32 // lazy-embed only when needed for prohibition rules

	for _, rule := range rules {
		polarity := action.ClassifyPolarity(rule.L2)
		marker := ""
		if len(rule.ID) >= 12 {
			marker = rule.ID[:12]
		}

		switch polarity {
		case action.PolarityPermission:
			// Permission rules ("allow curl") — never restrict, skip
			ensureRulePattern(n, rule.ID, polarity, rule.L2)
			continue

		case action.PolarityInstruction:
			// Instruction rules — check forbidden patterns, NOT embedding similarity
			rp := ensureRulePattern(n, rule.ID, polarity, rule.L2)

			// Check if the action uses a forbidden tool/technique
			if rp != nil && len(rp.ForbiddenPatterns) > 0 {
				if matched, found := action.CheckForbiddenPatterns(actionDesc, rp.ForbiddenPatterns); found {
					if err := n.Actions.RecordAmnesia(sessionID, n.PeerID(), "", "", rule.ID, marker, rule.L2,
						toolName, inputStr, 1.0, action.MatchTypeForbiddenPattern, matched); err != nil {
						slog.Warn("record amnesia failed", "match_type", "forbidden_pattern", "error", err)
					}
				}
			}

		case action.PolarityProhibition:
			// Prohibition rules — use embedding similarity (existing approach)
			ensureRulePattern(n, rule.ID, polarity, rule.L2)

			if actionVec == nil {
				actionVec, err = n.Embedder.EmbedQuery(ctx, actionDesc)
				if err != nil {
					return
				}
			}

			ruleVec, err := n.Embedder.EmbedQuery(ctx, rule.L2)
			if err != nil {
				continue
			}

			sim := cosineSim(actionVec, ruleVec)
			if sim >= 0.6 {
				if err := n.Actions.RecordAmnesia(sessionID, n.PeerID(), "", "", rule.ID, marker, rule.L2,
					toolName, inputStr, sim, action.MatchTypeSimilarity, ""); err != nil {
					slog.Warn("record amnesia failed", "match_type", "similarity", "error", err)
				}
			}
		}
	}
}

// ensureRulePattern loads or auto-extracts patterns for a rule and persists them.
func ensureRulePattern(n *node.Node, ruleID, polarity, ruleText string) *action.RulePattern {
	rp, _ := n.Actions.GetRulePattern(ruleID)
	if rp != nil {
		return rp
	}
	required, forbidden := action.ExtractPatterns(ruleText)
	if err := n.Actions.SetRulePattern(ruleID, polarity, required, forbidden, true); err != nil {
		slog.Warn("set rule pattern failed", "rule_id", ruleID, "error", err)
	}
	return &action.RulePattern{
		RuleID:            ruleID,
		Polarity:          polarity,
		RequiredPatterns:  required,
		ForbiddenPatterns: forbidden,
		AutoExtracted:     true,
	}
}

func cosineSim(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// checkManifestDelusion checks if an action is unrelated to any active manifest.
// Low similarity to ALL manifests = agent is going off-spec (delusional).
func checkManifestDelusion(n *node.Node, sessionID, toolName string, toolInput any) {
	manifests, err := n.Manifests.List("open", 10)
	if err != nil || len(manifests) == 0 {
		return
	}

	// Build action description
	inputStr := ""
	switch v := toolInput.(type) {
	case string:
		inputStr = v
	case map[string]any:
		if cmd, ok := v["command"].(string); ok {
			inputStr = cmd
		} else if path, ok := v["file_path"].(string); ok {
			inputStr = path
		} else {
			b, _ := json.Marshal(v)
			inputStr = string(b)
		}
	}
	if inputStr == "" || len(inputStr) < 20 {
		return // skip trivial actions
	}

	actionDesc := fmt.Sprintf("%s: %s", toolName, inputStr)
	if len(actionDesc) > 500 {
		actionDesc = actionDesc[:500]
	}

	ctx := context.Background()
	actionVec, err := n.Embedder.EmbedQuery(ctx, actionDesc)
	if err != nil {
		return
	}

	// Check similarity against each active manifest
	for _, m := range manifests {
		// Embed the manifest description + title (not full content — too long)
		manifestDesc := m.Title + ": " + m.Description
		manifestVec, err := n.Embedder.EmbedQuery(ctx, manifestDesc)
		if err != nil {
			continue
		}

		sim := cosineSim(actionVec, manifestVec)

		// If similarity is very low (< 0.3), the action is unrelated to the manifest
		// This means the agent is doing something off-spec
		if sim < 0.3 {
			marker := ""
			if len(m.ID) >= 12 {
				marker = m.ID[:12]
			}
			reason := fmt.Sprintf("Action has %.0f%% similarity to manifest '%s' — agent may be going off-spec", sim*100, m.Title)
			if err := n.Manifests.RecordDelusion(sessionID, n.PeerID(), "", "", m.ID, marker, m.Title, toolName, inputStr, sim, reason); err != nil {
				slog.Warn("record delusion failed", "error", err)
			}
		}
	}
}

func apiAmnesiaByPeer(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		events, err := n.Actions.ListAmnesia(status, 10000)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		type aItem struct {
			ID             int     `json:"id"`
			SessionID      string  `json:"session_id"`
			ActionID       string  `json:"action_id"`
			TaskID         string  `json:"task_id"`
			RuleMarker     string  `json:"rule_marker"`
			RuleText       string  `json:"rule_text"`
			ToolName       string  `json:"tool_name"`
			ToolInput      string  `json:"tool_input"`
			Score          float64 `json:"score"`
			MatchType      string  `json:"match_type"`
			MatchedPattern string  `json:"matched_pattern"`
			Status         string  `json:"status"`
			CreatedAt      string  `json:"created_at"`
		}
		type peerGroup struct {
			PeerID  string  `json:"peer_id"`
			Count   int     `json:"count"`
			Events  []aItem `json:"events"`
		}
		peers := make(map[string][]aItem)
		peerOrder := []string{}
		for _, a := range events {
			pid := a.SourceNode
			if pid == "" {
				pid = n.PeerID()
			}
			input := a.ToolInput
			if len(input) > 300 {
				input = input[:300] + "..."
			}
			if _, ok := peers[pid]; !ok {
				peerOrder = append(peerOrder, pid)
			}
			matchType := a.MatchType
			if matchType == "" {
				matchType = action.MatchTypeSimilarity
			}
			peers[pid] = append(peers[pid], aItem{
				ID: a.ID, SessionID: a.SessionID, ActionID: a.ActionID, TaskID: a.TaskID,
				RuleMarker: a.RuleMarker, RuleText: a.RuleText,
				ToolName: a.ToolName, ToolInput: input, Score: a.Score,
				MatchType: matchType, MatchedPattern: a.MatchedPattern,
				Status: a.Status, CreatedAt: a.CreatedAt.UTC().Format(time.RFC3339),
			})
		}
		var result []peerGroup
		for _, pid := range peerOrder {
			items := peers[pid]
			result = append(result, peerGroup{PeerID: pid, Count: len(items), Events: items})
		}
		writeJSON(w, result)
	}
}

func apiAmnesia(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		events, err := n.Actions.ListAmnesia(status, 10000)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, events)
	}
}

func apiAmnesiaUpdate(n *node.Node, newStatus string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := mux.Vars(r)["id"]
		var id int
		fmt.Sscanf(idStr, "%d", &id)
		if err := n.Actions.UpdateStatus(id, newStatus); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": newStatus})
	}
}

func apiDelusionsByPeer(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		events, err := n.Manifests.ListDelusions(status, 200)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		type dItem struct {
			ID             int     `json:"id"`
			SessionID      string  `json:"session_id"`
			ActionID       string  `json:"action_id"`
			TaskID         string  `json:"task_id"`
			ManifestMarker string  `json:"manifest_marker"`
			ManifestTitle  string  `json:"manifest_title"`
			ToolName       string  `json:"tool_name"`
			ToolInput      string  `json:"tool_input"`
			Score          float64 `json:"score"`
			Reason         string  `json:"reason"`
			Status         string  `json:"status"`
			CreatedAt      string  `json:"created_at"`
		}
		type peerGroup struct {
			PeerID    string  `json:"peer_id"`
			Count     int     `json:"count"`
			Delusions []dItem `json:"delusions"`
		}
		peers := make(map[string][]dItem)
		peerOrder := []string{}
		for _, d := range events {
			pid := d.SourceNode
			if pid == "" {
				pid = n.PeerID()
			}
			input := d.ToolInput
			if len(input) > 300 {
				input = input[:300] + "..."
			}
			if _, ok := peers[pid]; !ok {
				peerOrder = append(peerOrder, pid)
			}
			peers[pid] = append(peers[pid], dItem{
				ID: d.ID, SessionID: d.SessionID, ActionID: d.ActionID, TaskID: d.TaskID,
				ManifestMarker: d.ManifestMarker, ManifestTitle: d.ManifestTitle,
				ToolName: d.ToolName, ToolInput: input, Score: d.Score,
				Reason: d.Reason, Status: d.Status,
				CreatedAt: d.CreatedAt.UTC().Format(time.RFC3339),
			})
		}
		var result []peerGroup
		for _, pid := range peerOrder {
			items := peers[pid]
			result = append(result, peerGroup{PeerID: pid, Count: len(items), Delusions: items})
		}
		writeJSON(w, result)
	}
}

func apiDelusions(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		events, err := n.Manifests.ListDelusions(status, 50)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, events)
	}
}

func apiDelusionUpdate(n *node.Node, newStatus string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := mux.Vars(r)["id"]
		var id int
		fmt.Sscanf(idStr, "%d", &id)
		if err := n.Manifests.UpdateDelusionStatus(id, newStatus); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": newStatus})
	}
}

func apiVisceralByPeer(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rules, err := n.Index.ListByType("visceral", 100)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		type rItem struct {
			ID                string   `json:"id"`
			Marker            string   `json:"marker"`
			Text              string   `json:"text"`
			Source            string   `json:"source"`
			Polarity          string   `json:"polarity"`
			RequiredPatterns  []string `json:"required_patterns"`
			ForbiddenPatterns []string `json:"forbidden_patterns"`
		}
		type peerGroup struct {
			PeerID string  `json:"peer_id"`
			Count  int     `json:"count"`
			Rules  []rItem `json:"rules"`
		}
		peers := make(map[string][]rItem)
		peerOrder := []string{}
		for _, m := range rules {
			pid := m.SourceNode
			if pid == "" {
				pid = n.PeerID()
			}
			marker := ""
			if len(m.ID) >= 12 {
				marker = m.ID[:12]
			}

			// Get polarity and patterns
			polarity := action.ClassifyPolarity(m.L2)
			var required, forbidden []string
			rp, _ := n.Actions.GetRulePattern(m.ID)
			if rp != nil {
				required = rp.RequiredPatterns
				forbidden = rp.ForbiddenPatterns
			} else {
				required, forbidden = action.ExtractPatterns(m.L2)
			}
			if required == nil {
				required = []string{}
			}
			if forbidden == nil {
				forbidden = []string{}
			}

			if _, ok := peers[pid]; !ok {
				peerOrder = append(peerOrder, pid)
			}
			peers[pid] = append(peers[pid], rItem{
				ID: m.ID, Marker: marker, Text: m.L2, Source: m.SourceAgent,
				Polarity: polarity, RequiredPatterns: required, ForbiddenPatterns: forbidden,
			})
		}
		var result []peerGroup
		for _, pid := range peerOrder {
			items := peers[pid]
			result = append(result, peerGroup{PeerID: pid, Count: len(items), Rules: items})
		}
		writeJSON(w, result)
	}
}

func apiVisceralList(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rules, err := n.Index.ListByType("visceral", 100)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, rules)
	}
}

func apiVisceralConfirmations(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		confs, err := n.Actions.ListConfirmations(20)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, confs)
	}
}

func apiVisceralAdd(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Rule string `json:"rule"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Rule == "" {
			http.Error(w, "rule is required", 400)
			return
		}

		mem, err := n.StoreMemory(r.Context(), req.Rule, "/visceral/rules/", "visceral", "global", "", "visceral", "dashboard", nil)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, mem)
	}
}

func apiVisceralDelete(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		mem, _ := n.Index.GetByID(id)
		if mem == nil {
			mem, _ = n.Index.GetByIDPrefix(id)
		}
		if mem == nil {
			http.Error(w, "not found", 404)
			return
		}
		if err := n.DeleteMemory(mem.ID); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "deleted"})
	}
}

// --- Rule Patterns (Amnesia v2) ---

func apiRulePatterns(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		patterns, err := n.Actions.ListRulePatterns()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if patterns == nil {
			patterns = []action.RulePattern{}
		}
		writeJSON(w, patterns)
	}
}

func apiRulePatternGet(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ruleID := mux.Vars(r)["rule_id"]
		rp, err := n.Actions.GetRulePattern(ruleID)
		if err != nil {
			// Try prefix match
			rp = findPatternByPrefix(n, ruleID)
		}
		if rp == nil {
			http.Error(w, "not found", 404)
			return
		}
		writeJSON(w, rp)
	}
}

func apiRulePatternUpdate(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ruleID := mux.Vars(r)["rule_id"]
		var req struct {
			RequiredPatterns  []string `json:"required_patterns"`
			ForbiddenPatterns []string `json:"forbidden_patterns"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}

		// Get existing pattern to preserve polarity
		rp, _ := n.Actions.GetRulePattern(ruleID)
		if rp == nil {
			rp = findPatternByPrefix(n, ruleID)
		}
		polarity := action.PolarityProhibition
		if rp != nil {
			polarity = rp.Polarity
		}

		if err := n.Actions.SetRulePattern(ruleID, polarity, req.RequiredPatterns, req.ForbiddenPatterns, false); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "updated"})
	}
}

func findPatternByPrefix(n *node.Node, prefix string) *action.RulePattern {
	patterns, err := n.Actions.ListRulePatterns()
	if err != nil {
		return nil
	}
	for _, p := range patterns {
		if strings.HasPrefix(p.RuleID, prefix) {
			return &p
		}
	}
	return nil
}
