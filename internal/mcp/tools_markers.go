package mcp

import (
	"context"
	"fmt"

	"openpraxis/internal/marker"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) handleMarkerFlag(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	targetID := argStr(a, "target_id")
	targetType := argStr(a, "target_type")
	message := argStr(a, "message")

	if targetID == "" || targetType == "" || message == "" {
		return errResult("target_id, target_type, and message are required"), nil
	}
	if targetType != "memory" && targetType != "conversation" {
		return errResult("target_type must be 'memory' or 'conversation'"), nil
	}

	// Resolve target path/title for display
	targetPath := ""
	if targetType == "memory" {
		mem, _ := s.node.Index.GetByID(targetID)
		if mem != nil {
			targetPath = mem.Path
		}
	} else {
		conv, _ := s.node.Conversations.GetByID(targetID)
		if conv != nil {
			targetPath = conv.Title
		}
	}

	toNode := argStr(a, "to_node")
	priority := argStr(a, "priority")

	m := marker.NewMarker(targetID, targetType, targetPath, s.node.PeerID(), toNode, message, priority)
	if err := s.node.Markers.Save(m); err != nil {
		return errResult("save marker: %v", err), nil
	}

	dest := toNode
	if dest == "" || dest == "all" {
		dest = "all peers"
	}
	return textResult(fmt.Sprintf("Flagged %s %s for %s: %s\nMarker ID: %s",
		targetType, targetPath, dest, message, m.ID)), nil
}

func (s *Server) handleMarkerList(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	status := argStr(a, "status")

	markers, err := s.node.Markers.ListForNode(s.node.PeerID(), status, 50)
	if err != nil {
		return errResult("list markers: %v", err), nil
	}

	if len(markers) == 0 {
		return textResult("No markers."), nil
	}

	var output string
	for _, m := range markers {
		icon := "  "
		if m.Priority == "high" {
			icon = "! "
		} else if m.Priority == "urgent" {
			icon = "!!"
		}
		output += fmt.Sprintf("%s[%s] %s %s\n   From: %s | %s\n   %s\n\n",
			icon, m.Status, m.TargetType, m.TargetPath,
			m.FromNode, m.CreatedAt.Format("2006-01-02 15:04"),
			m.Message)
	}

	return textResult(output), nil
}

func (s *Server) handleMarkerDone(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	a := args(req)
	id := argStr(a, "id")
	if id == "" {
		return errResult("id is required"), nil
	}

	if err := s.node.Markers.MarkDone(id); err != nil {
		return errResult("mark done: %v", err), nil
	}

	return textResult(fmt.Sprintf("Marker %s marked as done.", id)), nil
}
