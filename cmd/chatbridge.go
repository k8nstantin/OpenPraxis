package cmd

import (
	"context"
	"strings"

	"github.com/k8nstantin/OpenPraxis/internal/chat"
	"github.com/k8nstantin/OpenPraxis/internal/node"
)

func joinRefs(refs []string) string {
	return strings.Join(refs, ", ")
}

// nodeBridge adapts *node.Node to satisfy chat.NodeBridge,
// breaking the import cycle (node imports chat, chat defines the interface).
type nodeBridge struct {
	n *node.Node
}

func newNodeBridge(n *node.Node) chat.NodeBridge {
	return &nodeBridge{n: n}
}

func (b *nodeBridge) SearchMemories(ctx context.Context, query string, limit int) ([]chat.MemoryResult, error) {
	results, err := b.n.SearchMemories(ctx, query, limit, "", "", "")
	if err != nil {
		return nil, err
	}
	var out []chat.MemoryResult
	for _, r := range results {
		out = append(out, chat.MemoryResult{
			ID:    r.Memory.ID,
			Path:  r.Memory.Path,
			L0:    r.Memory.L0,
			L1:    r.Memory.L1,
			L2:    r.Memory.L2,
			Type:  r.Memory.Type,
			Score: r.Score,
		})
	}
	return out, nil
}

func (b *nodeBridge) RecallMemory(id string) (*chat.MemoryResult, error) {
	// Try full ID first
	mem, err := b.n.Index.GetByID(id)
	if err != nil || mem == nil {
		// Try prefix/marker lookup
		mem, err = b.n.Index.GetByIDPrefix(id)
		if err != nil || mem == nil {
			return nil, nil
		}
	}
	return &chat.MemoryResult{
		ID: mem.ID, Path: mem.Path, L0: mem.L0, L1: mem.L1, L2: mem.L2, Type: mem.Type,
	}, nil
}

func (b *nodeBridge) ListManifests(status string, limit int) ([]chat.ManifestSummary, error) {
	manifests, err := b.n.Manifests.List(status, limit)
	if err != nil {
		return nil, err
	}
	var out []chat.ManifestSummary
	for _, m := range manifests {
		out = append(out, chat.ManifestSummary{
			Marker: m.Marker, Title: m.Title, Status: m.Status, JiraRef: joinRefs(m.JiraRefs),
		})
	}
	return out, nil
}

func (b *nodeBridge) GetManifest(id string) (*chat.ManifestDetail, error) {
	m, err := b.n.Manifests.Get(id)
	if err != nil || m == nil {
		return nil, err
	}
	return &chat.ManifestDetail{
		Marker: m.Marker, Title: m.Title, Status: m.Status,
		Version: m.Version, Author: m.Author, JiraRef: joinRefs(m.JiraRefs), Content: m.Content,
	}, nil
}

func (b *nodeBridge) ListTasks(status string, limit int) ([]chat.TaskSummary, error) {
	tasks, err := b.n.Tasks.List(status, limit)
	if err != nil {
		return nil, err
	}
	var out []chat.TaskSummary
	for _, t := range tasks {
		out = append(out, chat.TaskSummary{
			Marker: t.Marker, Title: t.Title, Status: t.Status, Schedule: t.Schedule,
		})
	}
	return out, nil
}

func (b *nodeBridge) GetTask(id string) (*chat.TaskDetail, error) {
	t, err := b.n.Tasks.Get(id)
	if err != nil || t == nil {
		return nil, err
	}
	return &chat.TaskDetail{
		Marker: t.Marker, Title: t.Title, Status: t.Status, Schedule: t.Schedule,
		Agent: t.Agent, ManifestID: t.ManifestID, Description: t.Description,
	}, nil
}

func (b *nodeBridge) SearchConversations(ctx context.Context, query string, limit int) ([]chat.ConversationResult, error) {
	results, err := b.n.SearchConversations(ctx, query, limit, "", "")
	if err != nil {
		return nil, err
	}
	var out []chat.ConversationResult
	for _, r := range results {
		out = append(out, chat.ConversationResult{
			Title: r.Conversation.Title, Agent: r.Conversation.Agent,
			TurnCount: r.Conversation.TurnCount, Score: r.Score,
		})
	}
	return out, nil
}

func (b *nodeBridge) ListVisceralRules() ([]chat.VisceralRule, error) {
	rules, err := b.n.Index.ListByType("visceral", 100)
	if err != nil {
		return nil, err
	}
	var out []chat.VisceralRule
	for _, r := range rules {
		marker := ""
		if len(r.ID) >= 8 {
			marker = r.ID[:12]
		}
		out = append(out, chat.VisceralRule{Marker: marker, Text: r.L2})
	}
	return out, nil
}
