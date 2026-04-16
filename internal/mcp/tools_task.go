package mcp

import (
	"context"
	"fmt"
	"log/slog"

	"openpraxis/internal/task"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) registerTaskTools() {
	s.mcp.AddTool(
		mcplib.NewTool("task_create",
			mcplib.WithDescription("Create a new task. Tasks can optionally be linked to a manifest. Standalone tasks (no manifest) are also supported."),
			mcplib.WithString("title", mcplib.Required(), mcplib.Description("Task title")),
			mcplib.WithString("description", mcplib.Description("Task description")),
			mcplib.WithString("schedule", mcplib.Description("Schedule: 'once', '5m', '1h', 'at:ISO8601'. Default: once")),
			mcplib.WithString("manifest_id", mcplib.Description("Manifest ID or marker to link task to (optional — omit for standalone task)")),
			mcplib.WithString("agent", mcplib.Description("Agent type: claude-code, cursor, etc. Default: claude-code")),
			mcplib.WithNumber("max_turns", mcplib.Description("Max agent turns. Default: 50")),
			mcplib.WithString("depends_on", mcplib.Description("Task ID that must complete before this runs")),
		),
		s.handleTaskCreate,
	)

	s.mcp.AddTool(
		mcplib.NewTool("task_list",
			mcplib.WithDescription("List tasks with optional filters. Returns most recent first."),
			mcplib.WithString("status", mcplib.Description("Filter by status: running, scheduled, waiting, pending, completed, failed, cancelled")),
			mcplib.WithString("manifest_id", mcplib.Description("Filter by manifest ID or marker")),
			mcplib.WithNumber("limit", mcplib.Description("Max results. Default: 20")),
		),
		s.handleTaskList,
	)

	s.mcp.AddTool(
		mcplib.NewTool("task_get",
			mcplib.WithDescription("Get full task detail: metadata, linked manifests, run history, actions, amnesia flags, and delusion flags."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Task ID or 8-char marker")),
		),
		s.handleTaskGet,
	)

	s.mcp.AddTool(
		mcplib.NewTool("task_start",
			mcplib.WithDescription("Schedule and start a task. Optionally override the schedule."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Task ID or marker")),
			mcplib.WithString("schedule", mcplib.Description("Override schedule (e.g. 'once', '5m', 'at:2026-04-12T15:00:00Z')")),
		),
		s.handleTaskStart,
	)

	s.mcp.AddTool(
		mcplib.NewTool("task_cancel",
			mcplib.WithDescription("Cancel a scheduled or running task."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Task ID or marker")),
		),
		s.handleTaskCancel,
	)

	s.mcp.AddTool(
		mcplib.NewTool("task_link_manifest",
			mcplib.WithDescription("Link a task to an additional manifest (many-to-many). The task can already have a primary manifest — this adds a secondary link."),
			mcplib.WithString("task_id", mcplib.Required(), mcplib.Description("Task ID or marker")),
			mcplib.WithString("manifest_id", mcplib.Required(), mcplib.Description("Manifest ID or marker")),
		),
		s.handleTaskLinkManifest,
	)

	s.mcp.AddTool(
		mcplib.NewTool("task_unlink_manifest",
			mcplib.WithDescription("Remove a link between a task and a manifest."),
			mcplib.WithString("task_id", mcplib.Required(), mcplib.Description("Task ID or marker")),
			mcplib.WithString("manifest_id", mcplib.Required(), mcplib.Description("Manifest ID or marker")),
		),
		s.handleTaskUnlinkManifest,
	)

	s.mcp.AddTool(
		mcplib.NewTool("task_pause",
			mcplib.WithDescription("Pause a running task. Sends SIGSTOP to the agent process, freezing it in place. The task can be resumed later."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Task ID or marker")),
		),
		s.handleTaskPause,
	)

	s.mcp.AddTool(
		mcplib.NewTool("task_resume",
			mcplib.WithDescription("Resume a paused task. Sends SIGCONT to the agent process, continuing from where it was frozen."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Task ID or marker")),
		),
		s.handleTaskResume,
	)
}

func (s *Server) handleTaskCreate(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	title := argStr(a, "title")
	if title == "" {
		return errResult("title is required"), nil
	}

	description := argStr(a, "description")
	schedule := argStr(a, "schedule")
	manifestID := argStr(a, "manifest_id")
	agent := argStr(a, "agent")
	maxTurns := int(argFloat(a, "max_turns"))
	dependsOn := argStr(a, "depends_on")

	// Resolve manifest marker to full ID if provided
	if manifestID != "" {
		m, err := s.node.Manifests.Get(manifestID)
		if err != nil || m == nil {
			return errResult("manifest not found: %s", manifestID), nil
		}
		manifestID = m.ID
	}

	// Resolve depends_on marker to full ID if provided
	if dependsOn != "" {
		dep, err := s.node.Tasks.Get(dependsOn)
		if err != nil || dep == nil {
			return errResult("depends_on task not found: %s", dependsOn), nil
		}
		dependsOn = dep.ID
	}

	t, err := s.node.Tasks.Create(manifestID, title, description, schedule, agent, s.node.PeerID(), s.sessionSource(ctx), dependsOn, maxTurns)
	if err != nil {
		return errResult("create task: %v", err), nil
	}

	// If manifest was provided, also add to link table for many-to-many
	if manifestID != "" {
		if err := s.node.Tasks.LinkManifest(t.ID, manifestID); err != nil {
			slog.Warn("link manifest to task failed", "error", err)
		}
	}

	manifestLabel := "standalone"
	if manifestID != "" {
		manifestLabel = manifestID[:12]
	}

	return textResult(fmt.Sprintf("Task created [%s]: %s\nManifest: %s | Schedule: %s | Agent: %s | Max turns: %d",
		t.Marker, t.Title, manifestLabel, t.Schedule, t.Agent, t.MaxTurns)), nil
}

func (s *Server) handleTaskList(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	status := argStr(a, "status")
	manifestID := argStr(a, "manifest_id")
	limit := int(argFloat(a, "limit"))
	if limit <= 0 {
		limit = 20
	}

	var tasks []*task.Task
	var err error

	if manifestID != "" {
		// Resolve marker
		m, merr := s.node.Manifests.Get(manifestID)
		if merr != nil || m == nil {
			return errResult("manifest not found: %s", manifestID), nil
		}
		// Get tasks with primary manifest_id
		tasks, err = s.node.Tasks.ListByManifest(m.ID, limit)
		if err != nil {
			return errResult("list tasks: %v", err), nil
		}
		// Also get tasks linked via link table and merge
		linked, _ := s.node.Tasks.ListTasksByLinkedManifest(m.ID, limit)
		seen := make(map[string]bool)
		for _, t := range tasks {
			seen[t.ID] = true
		}
		for _, t := range linked {
			if !seen[t.ID] {
				tasks = append(tasks, t)
			}
		}
	} else {
		tasks, err = s.node.Tasks.List(status, limit)
		if err != nil {
			return errResult("list tasks: %v", err), nil
		}
	}

	if len(tasks) == 0 {
		return textResult("No tasks found."), nil
	}

	var output string
	for i, t := range tasks {
		manifest := "standalone"
		if t.ManifestID != "" {
			manifest = t.ManifestID[:min(8, len(t.ManifestID))]
		}
		output += fmt.Sprintf("%d. [%s] %s — %s (manifest: %s, schedule: %s, runs: %d)\n",
			i+1, t.Marker, t.Title, t.Status, manifest, t.Schedule, t.RunCount)
	}

	return textResult(output), nil
}

func (s *Server) handleTaskGet(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")
	if id == "" {
		return errResult("id is required"), nil
	}

	t, err := s.node.Tasks.Get(id)
	if err != nil {
		return errResult("get task: %v", err), nil
	}
	if t == nil {
		return textResult("Task not found."), nil
	}

	// Build comprehensive detail
	var output string
	manifest := "standalone"
	if t.ManifestID != "" {
		manifest = t.ManifestID[:min(8, len(t.ManifestID))]
		m, _ := s.node.Manifests.Get(t.ManifestID)
		if m != nil {
			manifest = fmt.Sprintf("[%s] %s", m.Marker, m.Title)
		}
	}

	output += fmt.Sprintf("Task [%s]: %s\n", t.Marker, t.Title)
	output += fmt.Sprintf("Status: %s | Agent: %s | Schedule: %s\n", t.Status, t.Agent, t.Schedule)
	output += fmt.Sprintf("Manifest: %s\n", manifest)
	if t.Description != "" {
		output += fmt.Sprintf("Description: %s\n", t.Description)
	}
	if t.DependsOn != "" {
		output += fmt.Sprintf("Depends on: %s\n", t.DependsOn[:min(8, len(t.DependsOn))])
	}
	output += fmt.Sprintf("Runs: %d | Max turns: %d\n", t.RunCount, t.MaxTurns)
	output += fmt.Sprintf("Created: %s | Updated: %s\n", t.CreatedAt.Format("2006-01-02 15:04"), t.UpdatedAt.Format("2006-01-02 15:04"))
	if t.LastRunAt != "" {
		output += fmt.Sprintf("Last run: %s\n", t.LastRunAt)
	}
	if t.NextRunAt != "" {
		output += fmt.Sprintf("Next run: %s\n", t.NextRunAt)
	}

	// Linked manifests (many-to-many)
	linkedManifests, _ := s.node.Tasks.ListLinkedManifests(t.ID)
	if len(linkedManifests) > 0 {
		output += "\nLinked manifests:\n"
		for _, mid := range linkedManifests {
			label := mid[:min(8, len(mid))]
			m, _ := s.node.Manifests.Get(mid)
			if m != nil {
				label = fmt.Sprintf("[%s] %s", m.Marker, m.Title)
			}
			output += fmt.Sprintf("  - %s\n", label)
		}
	}

	// Recent runs
	runs, _ := s.node.Tasks.ListRuns(t.ID, 5)
	if len(runs) > 0 {
		output += fmt.Sprintf("\nRecent runs (%d):\n", len(runs))
		for _, r := range runs {
			outSnippet := r.Output
			if len(outSnippet) > 100 {
				outSnippet = outSnippet[:100] + "..."
			}
			output += fmt.Sprintf("  Run #%d: %s (%d actions, %d lines) %s\n    %s\n",
				r.RunNumber, r.Status, r.Actions, r.Lines, r.StartedAt.Format("2006-01-02 15:04"), outSnippet)
		}
	}

	// Last output
	if t.LastOutput != "" {
		lastOut := t.LastOutput
		if len(lastOut) > 500 {
			lastOut = lastOut[:500] + "..."
		}
		output += fmt.Sprintf("\nLast output:\n%s\n", lastOut)
	}

	return textResult(output), nil
}

func (s *Server) handleTaskStart(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")
	if id == "" {
		return errResult("id is required"), nil
	}

	t, err := s.node.Tasks.Get(id)
	if err != nil || t == nil {
		return errResult("task not found: %s", id), nil
	}

	schedule := argStr(a, "schedule")
	if schedule == "" {
		schedule = t.Schedule
	}

	if err := s.node.Tasks.ScheduleTask(t.ID, schedule); err != nil {
		return errResult("schedule task: %v", err), nil
	}

	return textResult(fmt.Sprintf("Task [%s] scheduled: %s (schedule: %s)", t.Marker, t.Title, schedule)), nil
}

func (s *Server) handleTaskCancel(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")
	if id == "" {
		return errResult("id is required"), nil
	}

	t, err := s.node.Tasks.Get(id)
	if err != nil || t == nil {
		return errResult("task not found: %s", id), nil
	}

	if err := s.node.Tasks.UpdateStatus(t.ID, "cancelled"); err != nil {
		return errResult("cancel task: %v", err), nil
	}

	return textResult(fmt.Sprintf("Task [%s] cancelled: %s", t.Marker, t.Title)), nil
}

func (s *Server) handleTaskLinkManifest(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	taskID := argStr(a, "task_id")
	manifestID := argStr(a, "manifest_id")

	t, err := s.node.Tasks.Get(taskID)
	if err != nil || t == nil {
		return errResult("task not found: %s", taskID), nil
	}

	m, err := s.node.Manifests.Get(manifestID)
	if err != nil || m == nil {
		return errResult("manifest not found: %s", manifestID), nil
	}

	if err := s.node.Tasks.LinkManifest(t.ID, m.ID); err != nil {
		return errResult("link failed: %v", err), nil
	}

	return textResult(fmt.Sprintf("Linked: task [%s] %s → manifest [%s] %s",
		t.Marker, t.Title, m.Marker, m.Title)), nil
}

func (s *Server) handleTaskUnlinkManifest(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	taskID := argStr(a, "task_id")
	manifestID := argStr(a, "manifest_id")

	t, err := s.node.Tasks.Get(taskID)
	if err != nil || t == nil {
		return errResult("task not found: %s", taskID), nil
	}

	m, err := s.node.Manifests.Get(manifestID)
	if err != nil || m == nil {
		return errResult("manifest not found: %s", manifestID), nil
	}

	if err := s.node.Tasks.UnlinkManifest(t.ID, m.ID); err != nil {
		return errResult("unlink failed: %v", err), nil
	}

	return textResult(fmt.Sprintf("Unlinked: task [%s] from manifest [%s]", t.Marker, m.Marker)), nil
}

func (s *Server) handleTaskPause(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")
	if id == "" {
		return errResult("id is required"), nil
	}

	t, err := s.node.Tasks.Get(id)
	if err != nil || t == nil {
		return errResult("task not found: %s", id), nil
	}

	if s.node.GetRunner() == nil {
		return errResult("task runner not initialized"), nil
	}

	if err := s.node.GetRunner().Pause(t.ID); err != nil {
		return errResult("pause task: %v", err), nil
	}

	return textResult(fmt.Sprintf("Task [%s] paused: %s (SIGSTOP sent to agent process)", t.Marker, t.Title)), nil
}

func (s *Server) handleTaskResume(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")
	if id == "" {
		return errResult("id is required"), nil
	}

	t, err := s.node.Tasks.Get(id)
	if err != nil || t == nil {
		return errResult("task not found: %s", id), nil
	}

	if s.node.GetRunner() == nil {
		return errResult("task runner not initialized"), nil
	}

	if err := s.node.GetRunner().Resume(t.ID); err != nil {
		return errResult("resume task: %v", err), nil
	}

	return textResult(fmt.Sprintf("Task [%s] resumed: %s (SIGCONT sent to agent process)", t.Marker, t.Title)), nil
}
