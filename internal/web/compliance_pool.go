package web

import (
	"context"
	"log/slog"
	"sync"

	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/settings"
)

// complianceChecksEnabled reads the compliance_checks_enabled knob at
// system scope. The hook handler doesn't know the task id of the tool
// call (PostToolUse doesn't carry it) so we resolve at system scope.
// Operators who want per-product opt-out should set the knob at product
// scope AND disable at system scope as a belt-and-suspenders until the
// hook carries task_id.
func complianceChecksEnabled(ctx context.Context, n *node.Node) bool {
	if n == nil || n.SettingsStore == nil {
		return true // fail-open: keep current behaviour when store missing
	}
	entry, err := n.SettingsStore.Get(ctx, settings.ScopeSystem, "", "compliance_checks_enabled")
	if err != nil || entry.Value == "" {
		return true // default true, per catalog
	}
	// Value is JSON-encoded string per the catalog enum convention.
	return entry.Value == `"true"` || entry.Value == "true"
}

// compliancePool is a bounded worker pool that drains PostToolUse
// compliance + delusion checks. Replaces the prior `go checkX(...)` pattern
// which spawned an unbounded goroutine per tool call — with a chatty agent
// that's up to 7/sec × (N rules + M manifests) embedding calls stacking up
// and pegging serve CPU.
//
// Sizing is deliberately small (4 workers) because the bottleneck is
// Ollama's embedding throughput, not CPU contention. The channel is
// sized so a burst of up to 256 tool calls can queue before we start
// dropping checks — dropping is the right call over blocking the
// hook-handler response.

type complianceJob struct {
	kind      string // "visceral" | "delusion"
	sessionID string
	toolName  string
	toolInput any
}

var (
	compliancePoolOnce sync.Once
	compliancePoolCh   chan complianceJob
)

// startCompliancePool lazy-starts the worker pool on first enqueue.
func startCompliancePool(n *node.Node) {
	compliancePoolOnce.Do(func() {
		compliancePoolCh = make(chan complianceJob, 256)
		const workers = 4
		for i := 0; i < workers; i++ {
			go func() {
				for job := range compliancePoolCh {
					switch job.kind {
					case "visceral":
						checkVisceralCompliance(n, job.sessionID, job.toolName, job.toolInput)
					case "delusion":
						checkManifestDelusion(n, job.sessionID, job.toolName, job.toolInput)
					}
				}
			}()
		}
	})
}

// enqueueComplianceCheck submits a check to the pool. Non-blocking: if the
// queue is full (256 in-flight), the check is dropped with a warn log. This
// is a deliberate choice — dropping a compliance check on bursts is better
// than blocking the hook handler response or unbounded-goroutine leaks.
func enqueueComplianceCheck(n *node.Node, kind, sessionID, toolName string, toolInput any) {
	startCompliancePool(n)
	job := complianceJob{kind: kind, sessionID: sessionID, toolName: toolName, toolInput: toolInput}
	select {
	case compliancePoolCh <- job:
	default:
		slog.Warn("compliance pool queue full, dropping check",
			"kind", kind, "tool", toolName, "session", sessionID[:min(12, len(sessionID))])
	}
}
