package mcp

import (
	"context"
	"fmt"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/k8nstantin/OpenPraxis/internal/comments"
	"github.com/k8nstantin/OpenPraxis/internal/product"
)

func (s *Server) registerProductTools() {
	s.mcp.AddTool(
		mcplib.NewTool("product_create",
			mcplib.WithDescription("Create a product — top-level organizational entity grouping manifests. Hierarchy: peer → product → manifest → task."),
			mcplib.WithString("title", mcplib.Required(), mcplib.Description("Product title (e.g. 'OpenPraxis', 'Gryphon Data Lake')")),
			mcplib.WithString("description", mcplib.Description("One-line description")),
			mcplib.WithString("status", mcplib.Description("draft, open, closed, archive. Default: draft")),
			mcplib.WithString("tags", mcplib.Description("Comma-separated tags")),
		),
		s.handleProductCreate,
	)

	s.mcp.AddTool(
		mcplib.NewTool("product_list",
			mcplib.WithDescription("List all products with aggregated cost from linked manifests → tasks."),
			mcplib.WithString("status", mcplib.Description("Filter: draft, open, closed, archive")),
		),
		s.handleProductList,
	)

	s.mcp.AddTool(
		mcplib.NewTool("product_get",
			mcplib.WithDescription("Get a product by ID (first 12 chars). Returns full detail with aggregated cost."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Product ID")),
		),
		s.handleProductGet,
	)

	s.mcp.AddTool(
		mcplib.NewTool("product_update",
			mcplib.WithDescription("Update a product — modify title, description, status, or tags."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Product ID")),
			mcplib.WithString("title", mcplib.Description("New title")),
			mcplib.WithString("description", mcplib.Description("New description")),
			mcplib.WithString("status", mcplib.Description("draft, open, closed, archive")),
			mcplib.WithString("tags", mcplib.Description("Comma-separated tags")),
		),
		s.handleProductUpdate,
	)

	s.mcp.AddTool(
		mcplib.NewTool("product_delete",
			mcplib.WithDescription("Soft-delete a product. Does not delete linked manifests — they become standalone."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Product ID")),
			mcplib.WithBoolean("confirm", mcplib.Required(), mcplib.Description("Must be true to confirm deletion")),
		),
		s.handleProductDelete,
	)

	s.mcp.AddTool(
		mcplib.NewTool("product_dep_add",
			mcplib.WithDescription("Add a product→product dependency edge. Rejects self-loops and cycles (including transitive — the graph can go arbitrarily deep). The error names the rejected pair so you can fix your graph."),
			mcplib.WithString("product_id", mcplib.Required(), mcplib.Description("Product that will wait (ID)")),
			mcplib.WithString("depends_on_product_id", mcplib.Required(), mcplib.Description("Product that must close first (ID)")),
		),
		s.handleProductDepAdd,
	)

	s.mcp.AddTool(
		mcplib.NewTool("product_dep_remove",
			mcplib.WithDescription("Remove a product→product dependency edge. Idempotent."),
			mcplib.WithString("product_id", mcplib.Required(), mcplib.Description("Source product (ID)")),
			mcplib.WithString("depends_on_product_id", mcplib.Required(), mcplib.Description("Dep to remove (ID)")),
		),
		s.handleProductDepRemove,
	)

	s.mcp.AddTool(
		mcplib.NewTool("product_dep_list",
			mcplib.WithDescription("List product dependencies. direction=out (default) returns products this one depends on; direction=in returns products that depend on this one; direction=both returns both."),
			mcplib.WithString("product_id", mcplib.Required(), mcplib.Description("Product ID")),
			mcplib.WithString("direction", mcplib.Description("out | in | both (default: out)")),
		),
		s.handleProductDepList,
	)
}

// resolveProductPair validates a source + target product UUID pair
// via Products.Get (post marker rip-out: full UUID only). Mirrors the
// manifest resolver so the tool-handler layer is consistent across
// tiers.
func (s *Server) resolveProductPair(src, dst string) (srcID, dstID, errMsg string) {
	srcP, _ := s.node.Products.Get(src)
	if srcP == nil {
		return "", "", fmt.Sprintf("product not found: %s", src)
	}
	dstP, _ := s.node.Products.Get(dst)
	if dstP == nil {
		return "", "", fmt.Sprintf("dependency product not found: %s", dst)
	}
	return srcP.ID, dstP.ID, ""
}

func (s *Server) handleProductDepAdd(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	srcID, dstID, msg := s.resolveProductPair(argStr(a, "product_id"), argStr(a, "depends_on_product_id"))
	if msg != "" {
		return errResult("%s", msg), nil
	}
	if err := s.node.Products.AddDep(ctx, srcID, dstID, s.sessionSource(ctx)); err != nil {
		return errResult("%v", err), nil
	}
	src, _ := s.node.Products.Get(srcID)
	dst, _ := s.node.Products.Get(dstID)
	return textResult(fmt.Sprintf("Dep added: [%s] %s → [%s] %s",
		src.ID, src.Title, dst.ID, dst.Title)), nil
}

func (s *Server) handleProductDepRemove(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	srcID, dstID, msg := s.resolveProductPair(argStr(a, "product_id"), argStr(a, "depends_on_product_id"))
	if msg != "" {
		return errResult("%s", msg), nil
	}
	if err := s.node.Products.RemoveDep(ctx, srcID, dstID); err != nil {
		return errResult("remove dep: %v", err), nil
	}
	src, _ := s.node.Products.Get(srcID)
	dst, _ := s.node.Products.Get(dstID)
	return textResult(fmt.Sprintf("Dep removed: [%s] %s → [%s] %s",
		src.ID, src.Title, dst.ID, dst.Title)), nil
}

func (s *Server) handleProductDepList(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	p, _ := s.node.Products.Get(argStr(a, "product_id"))
	if p == nil {
		return errResult("product not found: %s", argStr(a, "product_id")), nil
	}
	direction := argStr(a, "direction")
	if direction == "" {
		direction = "out"
	}

	formatDeps := func(rows []product.Dep) string {
		if len(rows) == 0 {
			return "  (none)"
		}
		var out []string
		for _, d := range rows {
			out = append(out, fmt.Sprintf("  [%s] %s — %s", d.ID, d.Title, d.Status))
		}
		return strings.Join(out, "\n")
	}

	var output string
	switch direction {
	case "out":
		deps, err := s.node.Products.ListDeps(ctx, p.ID)
		if err != nil {
			return errResult("list deps: %v", err), nil
		}
		output = fmt.Sprintf("[%s] %s depends on:\n%s\n", p.ID, p.Title, formatDeps(deps))
	case "in":
		dependents, err := s.node.Products.ListDependents(ctx, p.ID)
		if err != nil {
			return errResult("list dependents: %v", err), nil
		}
		output = fmt.Sprintf("[%s] %s is depended on by:\n%s\n", p.ID, p.Title, formatDeps(dependents))
	case "both":
		deps, err := s.node.Products.ListDeps(ctx, p.ID)
		if err != nil {
			return errResult("list deps: %v", err), nil
		}
		dependents, err := s.node.Products.ListDependents(ctx, p.ID)
		if err != nil {
			return errResult("list dependents: %v", err), nil
		}
		output = fmt.Sprintf("[%s] %s depends on:\n%s\n\n[%s] %s is depended on by:\n%s\n",
			p.ID, p.Title, formatDeps(deps), p.ID, p.Title, formatDeps(dependents))
	default:
		return errResult("direction must be one of: out, in, both (got %q)", direction), nil
	}
	return textResult(output), nil
}

func (s *Server) handleProductCreate(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	title := argStr(a, "title")
	desc := argStr(a, "description")
	status := argStr(a, "status")
	tags := splitCSV(argStr(a, "tags"))

	p, err := s.node.Products.Create(title, desc, status, s.node.PeerID(), tags)
	if err != nil {
		return errResult("create product: %v", err), nil
	}

	return textResult(fmt.Sprintf("Product created [%s]: %s\nStatus: %s | Tags: %v",
		p.ID, p.Title, p.Status, p.Tags)), nil
}

func (s *Server) handleProductList(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	status := argStr(a, "status")

	products, err := s.node.Products.List(status, 50)
	if err != nil {
		return errResult("list products: %v", err), nil
	}

	if len(products) == 0 {
		return textResult("No products found."), nil
	}

	var output string
	for i, p := range products {
		tags := ""
		if len(p.Tags) > 0 {
			tags = " | Tags: " + strings.Join(p.Tags, ", ")
		}
		output += fmt.Sprintf("%d. [%s] %s — %s (%s%s)\n   Manifests: %d | Tasks: %d | Turns: %d | Cost: $%.4f\n",
			i+1, p.ID, p.Title, p.Description, p.Status, tags,
			p.TotalManifests, p.TotalTasks, p.TotalTurns, p.TotalCost)
	}

	return textResult(output), nil
}

func (s *Server) handleProductGet(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")

	p, err := s.node.Products.Get(id)
	if err != nil {
		return errResult("get product: %v", err), nil
	}
	if p == nil {
		return textResult("Product not found."), nil
	}

	tags := "none"
	if len(p.Tags) > 0 {
		tags = strings.Join(p.Tags, ", ")
	}

	return textResult(fmt.Sprintf("[%s] %s\nStatus: %s | Tags: %s\nDescription: %s\nCreated: %s | Updated: %s\nManifests: %d | Tasks: %d | Turns: %d | Cost: $%.4f",
		p.ID, p.Title, p.Status, tags, p.Description,
		p.CreatedAt.Format("2006-01-02 15:04"), p.UpdatedAt.Format("2006-01-02 15:04"),
		p.TotalManifests, p.TotalTasks, p.TotalTurns, p.TotalCost)), nil
}

func (s *Server) handleProductUpdate(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")

	existing, err := s.node.Products.Get(id)
	if err != nil || existing == nil {
		return errResult("product not found"), nil
	}

	title := argStr(a, "title")
	if title == "" {
		title = existing.Title
	}
	desc := argStr(a, "description")
	if desc == "" {
		desc = existing.Description
	}
	status := argStr(a, "status")
	if status == "" {
		status = existing.Status
	}
	tagsStr := argStr(a, "tags")
	tags := existing.Tags
	if tagsStr != "" {
		tags = splitCSV(tagsStr)
	}

	if status == "archive" && existing.Status != "archive" {
		if err := s.node.ValidateArchiveProduct(existing.ID); err != nil {
			return errResult("%v", err), nil
		}
	}
	// Append a description_revision comment on description changes before the
	// denormalised UPDATE (DV/M2). The helper no-ops when desc is unchanged.
	if argStr(a, "description") != "" {
		if _, err := s.node.RecordDescriptionChange(ctx, comments.TargetProduct, existing.ID, desc, ""); err != nil {
			return errResult("record revision: %v", err), nil
		}
	}
	if err := s.node.Products.Update(existing.ID, title, desc, status, tags); err != nil {
		return errResult("update product: %v", err), nil
	}

	return textResult(fmt.Sprintf("Product updated [%s]: %s (%s)",
		existing.ID, title, status)), nil
}

func (s *Server) handleProductDelete(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")
	confirm := argBool(a, "confirm")

	if !confirm {
		return errResult("confirm must be true to delete"), nil
	}

	existing, err := s.node.Products.Get(id)
	if err != nil || existing == nil {
		return errResult("product not found"), nil
	}

	if err := s.node.Products.Delete(existing.ID); err != nil {
		return errResult("delete product: %v", err), nil
	}

	return textResult(fmt.Sprintf("Product deleted [%s]: %s", existing.ID, existing.Title)), nil
}
