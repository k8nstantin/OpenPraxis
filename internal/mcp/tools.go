package mcp

import (
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// args extracts the arguments map from a CallToolRequest.
func args(req mcplib.CallToolRequest) map[string]any {
	if m, ok := req.Params.Arguments.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func argStr(a map[string]any, key string) string {
	v, _ := a[key].(string)
	return v
}

func argFloat(a map[string]any, key string) float64 {
	v, _ := a[key].(float64)
	return v
}

func argBool(a map[string]any, key string) bool {
	v, _ := a[key].(bool)
	return v
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}
