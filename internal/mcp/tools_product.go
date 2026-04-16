package mcp

import (
	"context"
	"fmt"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
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
			mcplib.WithDescription("Get a product by ID or marker (first 12 chars). Returns full detail with aggregated cost."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Product ID or marker")),
		),
		s.handleProductGet,
	)

	s.mcp.AddTool(
		mcplib.NewTool("product_update",
			mcplib.WithDescription("Update a product — modify title, description, status, or tags."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Product ID or marker")),
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
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("Product ID or marker")),
			mcplib.WithBoolean("confirm", mcplib.Required(), mcplib.Description("Must be true to confirm deletion")),
		),
		s.handleProductDelete,
	)
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
		p.Marker, p.Title, p.Status, p.Tags)), nil
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
			i+1, p.Marker, p.Title, p.Description, p.Status, tags,
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
		p.Marker, p.Title, p.Status, tags, p.Description,
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
	if err := s.node.Products.Update(existing.ID, title, desc, status, tags); err != nil {
		return errResult("update product: %v", err), nil
	}

	return textResult(fmt.Sprintf("Product updated [%s]: %s (%s)",
		existing.Marker, title, status)), nil
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

	return textResult(fmt.Sprintf("Product deleted [%s]: %s", existing.Marker, existing.Title)), nil
}
