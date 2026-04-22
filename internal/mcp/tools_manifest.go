package mcp

import (
	"context"
	"fmt"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/manifest"
)

func (s *Server) registerManifestTools() {
	s.mcp.AddTool(
		mcplib.NewTool("manifest_create",
			mcplib.WithDescription("Create a development manifest — a detailed spec document for a tool, feature, or module. Manifests are shared across sessions and can reference Jira tickets."),
			mcplib.WithString("title", mcplib.Required(), mcplib.Description("Manifest title (e.g. 'OpenPraxis Visceral Compliance Engine')")),
			mcplib.WithString("description", mcplib.Required(), mcplib.Description("One-line description")),
			mcplib.WithString("content", mcplib.Required(), mcplib.Description("Full spec in markdown — architecture, modules, features, requirements")),
			mcplib.WithString("status", mcplib.Description("draft, open, closed, archive. Default: draft")),
			mcplib.WithString("project_id", mcplib.Description("Project ID or marker to assign manifest to (optional)")),
			mcplib.WithString("depends_on", mcplib.Description("Comma-separated manifest IDs or markers this manifest depends on (optional)")),
			mcplib.WithString("jira_refs", mcplib.Description("Comma-separated Jira tickets (e.g. 'ENG-4816,ENG-5266')")),
			mcplib.WithString("tags", mcplib.Description("Comma-separated tags")),
		),
		s.handleManifestCreate,
	)

	s.mcp.AddTool(
		mcplib.NewTool("manifest_get",
			mcplib.WithDescription("Get a manifest by ID (8-char marker or full UUID)."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Manifest ID or marker")),
		),
		s.handleManifestGet,
	)

	s.mcp.AddTool(
		mcplib.NewTool("manifest_update",
			mcplib.WithDescription("Update a manifest — modify content, status, or references. Bumps version."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Manifest ID or marker")),
			mcplib.WithString("title", mcplib.Description("New title")),
			mcplib.WithString("description", mcplib.Description("New description")),
			mcplib.WithString("content", mcplib.Description("New content (replaces entire content)")),
			mcplib.WithString("status", mcplib.Description("draft, open, closed, archive")),
			mcplib.WithString("project_id", mcplib.Description("Project ID or marker to assign manifest to")),
			mcplib.WithString("depends_on", mcplib.Description("Comma-separated manifest IDs or markers this manifest depends on")),
			mcplib.WithString("jira_refs", mcplib.Description("Comma-separated Jira tickets")),
			mcplib.WithString("tags", mcplib.Description("Comma-separated tags")),
		),
		s.handleManifestUpdate,
	)

	s.mcp.AddTool(
		mcplib.NewTool("manifest_list",
			mcplib.WithDescription("List all manifests. Filter by status."),
			mcplib.WithString("status", mcplib.Description("Filter: draft, open, closed, archive")),
		),
		s.handleManifestList,
	)

	s.mcp.AddTool(
		mcplib.NewTool("manifest_search",
			mcplib.WithDescription("Search manifests by keyword in title, description, content, Jira refs, or tags."),
			mcplib.WithString("query", mcplib.Required(), mcplib.Description("Search keyword or phrase")),
		),
		s.handleManifestSearch,
	)

	s.mcp.AddTool(
		mcplib.NewTool("manifest_delete",
			mcplib.WithDescription("Delete a manifest."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Manifest ID or marker")),
		),
		s.handleManifestDelete,
	)

	s.mcp.AddTool(
		mcplib.NewTool("manifest_dep_add",
			mcplib.WithDescription("Add a manifest→manifest dependency edge. Rejects self-loops and cycles; the error names the rejected pair so you can fix your graph. Triggers auto-activation downstream if the target manifest is already closed."),
			mcplib.WithString("manifest_id", mcplib.Required(), mcplib.Description("Manifest that will wait (ID or 12-char marker)")),
			mcplib.WithString("depends_on_manifest_id", mcplib.Required(), mcplib.Description("Manifest that must close first (ID or marker)")),
		),
		s.handleManifestDepAdd,
	)

	s.mcp.AddTool(
		mcplib.NewTool("manifest_dep_remove",
			mcplib.WithDescription("Remove a manifest→manifest dependency edge. Idempotent. If the removal makes the source manifest fully satisfied, any tasks sitting in 'waiting' due to the manifest block flip to 'pending' (Option B — operator must explicitly arm them)."),
			mcplib.WithString("manifest_id", mcplib.Required(), mcplib.Description("Source manifest (ID or marker)")),
			mcplib.WithString("depends_on_manifest_id", mcplib.Required(), mcplib.Description("Dep to remove (ID or marker)")),
		),
		s.handleManifestDepRemove,
	)

	s.mcp.AddTool(
		mcplib.NewTool("manifest_dep_list",
			mcplib.WithDescription("List manifest dependencies. direction=out (default) returns manifests this one depends on; direction=in returns manifests that depend on this one; direction=both returns both lists."),
			mcplib.WithString("manifest_id", mcplib.Required(), mcplib.Description("Manifest ID or marker")),
			mcplib.WithString("direction", mcplib.Description("out | in | both (default: out)")),
		),
		s.handleManifestDepList,
	)
}

func (s *Server) registerLinkTools() {
	s.mcp.AddTool(
		mcplib.NewTool("link_idea_manifest",
			mcplib.WithDescription("Link an idea to a manifest. The idea spawned or is implemented by the manifest."),
			mcplib.WithString("idea_id", mcplib.Required(), mcplib.Description("Idea ID or 8-char marker")),
			mcplib.WithString("manifest_id", mcplib.Required(), mcplib.Description("Manifest ID or 8-char marker")),
		),
		s.handleLinkIdeaManifest,
	)

	s.mcp.AddTool(
		mcplib.NewTool("unlink_idea_manifest",
			mcplib.WithDescription("Remove a link between an idea and manifest."),
			mcplib.WithString("idea_id", mcplib.Required(), mcplib.Description("Idea ID or marker")),
			mcplib.WithString("manifest_id", mcplib.Required(), mcplib.Description("Manifest ID or marker")),
		),
		s.handleUnlinkIdeaManifest,
	)
}

func (s *Server) handleLinkIdeaManifest(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	ideaID := argStr(a, "idea_id")
	manifestID := argStr(a, "manifest_id")

	// Resolve short markers to full IDs
	idea, _ := s.node.Ideas.Get(ideaID)
	if idea == nil {
		return errResult("idea not found: %s", ideaID), nil
	}
	manifest, _ := s.node.Manifests.Get(manifestID)
	if manifest == nil {
		return errResult("manifest not found: %s", manifestID), nil
	}

	if err := s.node.Manifests.LinkIdeaToManifest(idea.ID, manifest.ID); err != nil {
		return errResult("link failed: %v", err), nil
	}

	return textResult(fmt.Sprintf("Linked: idea [%s] %s → manifest [%s] %s",
		idea.Marker, idea.Title, manifest.Marker, manifest.Title)), nil
}

func (s *Server) handleUnlinkIdeaManifest(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	ideaID := argStr(a, "idea_id")
	manifestID := argStr(a, "manifest_id")

	idea, _ := s.node.Ideas.Get(ideaID)
	if idea == nil {
		return errResult("idea not found"), nil
	}
	manifest, _ := s.node.Manifests.Get(manifestID)
	if manifest == nil {
		return errResult("manifest not found"), nil
	}

	if err := s.node.Manifests.UnlinkIdeaFromManifest(idea.ID, manifest.ID); err != nil {
		return errResult("unlink failed: %v", err), nil
	}

	return textResult(fmt.Sprintf("Unlinked: idea [%s] from manifest [%s]", idea.Marker, manifest.Marker)), nil
}

func (s *Server) handleManifestCreate(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	title := argStr(a, "title")
	desc := argStr(a, "description")
	content := argStr(a, "content")
	status := argStr(a, "status")
	jiraRefs := splitCSV(argStr(a, "jira_refs"))
	tags := splitCSV(argStr(a, "tags"))

	projectID, err := s.node.ResolveProductID(argStr(a, "project_id"))
	if err != nil {
		return errResult("%v", err), nil
	}
	dependsOn, err := s.node.ResolveManifestDependsOn(argStr(a, "depends_on"), "")
	if err != nil {
		return errResult("%v", err), nil
	}
	m, err := s.node.Manifests.Create(title, desc, content, status, s.sessionSource(ctx), s.node.PeerID(), projectID, dependsOn, jiraRefs, tags)
	if err != nil {
		return errResult("create manifest: %v", err), nil
	}

	return textResult(fmt.Sprintf("Manifest created [%s]: %s\nStatus: %s | Version: %d\nJira: %v",
		m.Marker, m.Title, m.Status, m.Version, m.JiraRefs)), nil
}

func (s *Server) handleManifestGet(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")

	m, err := s.node.Manifests.Get(id)
	if err != nil {
		return errResult("get manifest: %v", err), nil
	}
	if m == nil {
		return textResult("Manifest not found."), nil
	}

	jira := "none"
	if len(m.JiraRefs) > 0 {
		jira = strings.Join(m.JiraRefs, ", ")
	}

	deps := "none"
	if m.DependsOn != "" {
		titles := s.node.ResolveDependsOnTitles(m.DependsOn)
		depParts := make([]string, len(titles))
		ids := m.ParseDependsOn()
		for i, t := range titles {
			marker := ids[i]
			if len(marker) >= 12 {
				marker = marker[:12]
			}
			depParts[i] = fmt.Sprintf("[%s] %s", marker, t)
		}
		deps = strings.Join(depParts, ", ")
	}

	return textResult(fmt.Sprintf("[%s] %s\nStatus: %s | Version: %d | Author: %s\nJira: %s\nDepends on: %s\nDescription: %s\nCreated: %s | Updated: %s\n\n%s",
		m.Marker, m.Title, m.Status, m.Version, m.Author, jira, deps, m.Description,
		m.CreatedAt.Format("2006-01-02 15:04"), m.UpdatedAt.Format("2006-01-02 15:04"),
		m.Content)), nil
}

func (s *Server) handleManifestUpdate(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")

	existing, err := s.node.Manifests.Get(id)
	if err != nil || existing == nil {
		return errResult("manifest not found"), nil
	}

	title := argStr(a, "title")
	if title == "" {
		title = existing.Title
	}
	desc := argStr(a, "description")
	if desc == "" {
		desc = existing.Description
	}
	content := argStr(a, "content")
	if content == "" {
		content = existing.Content
	}
	status := argStr(a, "status")
	if status == "" {
		status = existing.Status
	}
	jiraStr := argStr(a, "jira_refs")
	jiraRefs := existing.JiraRefs
	if jiraStr != "" {
		jiraRefs = splitCSV(jiraStr)
	}
	projectID := existing.ProjectID
	if raw := argStr(a, "project_id"); raw != "" {
		projectID, err = s.node.ResolveProductID(raw)
		if err != nil {
			return errResult("%v", err), nil
		}
	}
	tagsStr := argStr(a, "tags")
	tags := existing.Tags
	if tagsStr != "" {
		tags = splitCSV(tagsStr)
	}
	dependsOn := existing.DependsOn
	if raw := argStr(a, "depends_on"); raw != "" {
		dependsOn, err = s.node.ResolveManifestDependsOn(raw, existing.ID)
		if err != nil {
			return errResult("%v", err), nil
		}
	}

	if status == "archive" && existing.Status != "archive" {
		if err := s.node.ValidateArchiveManifest(existing.ID); err != nil {
			return errResult("%v", err), nil
		}
	}
	// Append a description_revision comment on content changes before the
	// denormalised UPDATE (DV/M2). Manifest's spec text lives in Content;
	// the short Description summary is not tracked for history in v1.
	if argStr(a, "content") != "" {
		if _, err := s.node.RecordDescriptionChange(ctx, comments.TargetManifest, existing.ID, content, ""); err != nil {
			return errResult("record revision: %v", err), nil
		}
	}
	if err := s.node.Manifests.Update(existing.ID, title, desc, content, status, projectID, dependsOn, jiraRefs, tags); err != nil {
		return errResult("update manifest: %v", err), nil
	}

	return textResult(fmt.Sprintf("Manifest updated [%s]: %s (v%d → v%d)",
		existing.Marker, title, existing.Version, existing.Version+1)), nil
}

func (s *Server) handleManifestList(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	status := argStr(a, "status")

	manifests, err := s.node.Manifests.List(status, 50)
	if err != nil {
		return errResult("list manifests: %v", err), nil
	}

	if len(manifests) == 0 {
		return textResult("No manifests found."), nil
	}

	var output string
	for i, m := range manifests {
		jira := ""
		if len(m.JiraRefs) > 0 {
			jira = " | Jira: " + strings.Join(m.JiraRefs, ", ")
		}
		deps := ""
		if m.DependsOn != "" {
			titles := s.node.ResolveDependsOnTitles(m.DependsOn)
			deps = " | Deps: " + strings.Join(titles, ", ")
		}
		output += fmt.Sprintf("%d. [%s] %s — %s (v%d, %s%s%s)\n",
			i+1, m.Marker, m.Title, m.Description, m.Version, m.Status, jira, deps)
	}

	return textResult(output), nil
}

func (s *Server) handleManifestSearch(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	query := argStr(a, "query")
	if query == "" {
		return errResult("query is required"), nil
	}

	results, err := s.node.Manifests.Search(query, 10)
	if err != nil {
		return errResult("search manifests: %v", err), nil
	}

	if len(results) == 0 {
		return textResult("No manifests found matching: " + query), nil
	}

	var output string
	for i, m := range results {
		jira := ""
		if len(m.JiraRefs) > 0 {
			jira = " | Jira: " + strings.Join(m.JiraRefs, ", ")
		}
		deps := ""
		if m.DependsOn != "" {
			titles := s.node.ResolveDependsOnTitles(m.DependsOn)
			deps = " | Deps: " + strings.Join(titles, ", ")
		}
		output += fmt.Sprintf("%d. [%s] %s — %s (v%d, %s%s%s)\n",
			i+1, m.Marker, m.Title, m.Description, m.Version, m.Status, jira, deps)
	}

	return textResult(output), nil
}

func (s *Server) handleManifestDelete(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")

	if err := s.node.Manifests.Delete(id); err != nil {
		return errResult("delete manifest: %v", err), nil
	}

	return textResult(fmt.Sprintf("Manifest deleted.")), nil
}

// resolveManifestPair accepts marker-or-id inputs for a source + target
// manifest, looks up both, and returns the full UUIDs. Centralizes the
// same "Manifests.Get accepts prefixes, dep tables want full IDs"
// translation that the dep-add/remove/list handlers all need.
func (s *Server) resolveManifestPair(src, dst string) (srcID, dstID string, errMsg string) {
	srcM, _ := s.node.Manifests.Get(src)
	if srcM == nil {
		return "", "", fmt.Sprintf("manifest not found: %s", src)
	}
	dstM, _ := s.node.Manifests.Get(dst)
	if dstM == nil {
		return "", "", fmt.Sprintf("dependency manifest not found: %s", dst)
	}
	return srcM.ID, dstM.ID, ""
}

func (s *Server) handleManifestDepAdd(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	srcID, dstID, msg := s.resolveManifestPair(argStr(a, "manifest_id"), argStr(a, "depends_on_manifest_id"))
	if msg != "" {
		return errResult("%s", msg), nil
	}
	if err := s.node.Manifests.AddDep(ctx, srcID, dstID, s.sessionSource(ctx)); err != nil {
		// ErrCycle / ErrSelfLoop / generic failures all surface their
		// error text directly — the whole point is the operator (or
		// agent) seeing WHICH edge was refused.
		return errResult("%v", err), nil
	}
	src, _ := s.node.Manifests.Get(srcID)
	dst, _ := s.node.Manifests.Get(dstID)
	return textResult(fmt.Sprintf("Dep added: [%s] %s → [%s] %s",
		src.Marker, src.Title, dst.Marker, dst.Title)), nil
}

func (s *Server) handleManifestDepRemove(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	srcID, dstID, msg := s.resolveManifestPair(argStr(a, "manifest_id"), argStr(a, "depends_on_manifest_id"))
	if msg != "" {
		return errResult("%s", msg), nil
	}
	if err := s.node.Manifests.RemoveDep(ctx, srcID, dstID); err != nil {
		return errResult("remove dep: %v", err), nil
	}
	src, _ := s.node.Manifests.Get(srcID)
	dst, _ := s.node.Manifests.Get(dstID)
	return textResult(fmt.Sprintf("Dep removed: [%s] %s → [%s] %s (downstream waiting tasks may have been rehabbed to 'pending').",
		src.Marker, src.Title, dst.Marker, dst.Title)), nil
}

func (s *Server) handleManifestDepList(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	m, _ := s.node.Manifests.Get(argStr(a, "manifest_id"))
	if m == nil {
		return errResult("manifest not found: %s", argStr(a, "manifest_id")), nil
	}
	direction := argStr(a, "direction")
	if direction == "" {
		direction = "out"
	}

	formatDeps := func(rows []manifest.Dep) string {
		if len(rows) == 0 {
			return "  (none)"
		}
		var out []string
		for _, d := range rows {
			out = append(out, fmt.Sprintf("  [%s] %s — %s", d.Marker, d.Title, d.Status))
		}
		return strings.Join(out, "\n")
	}

	var output string
	switch direction {
	case "out":
		deps, err := s.node.Manifests.ListDeps(ctx, m.ID)
		if err != nil {
			return errResult("list deps: %v", err), nil
		}
		output = fmt.Sprintf("[%s] %s depends on:\n%s\n", m.Marker, m.Title, formatDeps(deps))
	case "in":
		dependents, err := s.node.Manifests.ListDependents(ctx, m.ID)
		if err != nil {
			return errResult("list dependents: %v", err), nil
		}
		output = fmt.Sprintf("[%s] %s is depended on by:\n%s\n", m.Marker, m.Title, formatDeps(dependents))
	case "both":
		deps, err := s.node.Manifests.ListDeps(ctx, m.ID)
		if err != nil {
			return errResult("list deps: %v", err), nil
		}
		dependents, err := s.node.Manifests.ListDependents(ctx, m.ID)
		if err != nil {
			return errResult("list dependents: %v", err), nil
		}
		output = fmt.Sprintf("[%s] %s depends on:\n%s\n\n[%s] %s is depended on by:\n%s\n",
			m.Marker, m.Title, formatDeps(deps), m.Marker, m.Title, formatDeps(dependents))
	default:
		return errResult("direction must be one of: out, in, both (got %q)", direction), nil
	}
	return textResult(output), nil
}
