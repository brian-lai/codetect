package tools

import (
	"encoding/json"
	"fmt"

	"codetect/internal/mcp"
	"codetect/internal/search/files"
	"codetect/internal/server"
)

// RegisterAll registers the v4 tool set: search + get_file.
// All other tools (search_keyword, search_semantic, hybrid_search,
// hybrid_search_v2, find_symbol, list_defs_in_file) are removed.
// Their functionality is subsumed by the unified search tool.
func RegisterAll(srv *mcp.Server, ctx *server.Context) {
	RegisterSearch(srv, ctx)
	registerGetFile(srv)
}

func registerGetFile(srv *mcp.Server) {
	tool := mcp.Tool{
		Name:        "get_file",
		Description: "Read the contents of a file, optionally specifying a line range.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"path": {
					Type:        "string",
					Description: "Path to the file (relative or absolute)",
				},
				"start_line": {
					Type:        "number",
					Description: "First line to read (1-indexed, inclusive). Omit to start from beginning.",
				},
				"end_line": {
					Type:        "number",
					Description: "Last line to read (1-indexed, inclusive). Omit to read to end.",
				},
			},
			Required: []string{"path"},
		},
	}

	handler := func(args map[string]any) (*mcp.ToolsCallResult, error) {
		path, ok := args["path"].(string)
		if !ok || path == "" {
			return nil, fmt.Errorf("path is required")
		}

		startLine := 0
		if sl, ok := args["start_line"].(float64); ok {
			startLine = int(sl)
		}

		endLine := 0
		if el, ok := args["end_line"].(float64); ok {
			endLine = int(el)
		}

		result, err := files.GetFile(path, startLine, endLine)
		if err != nil {
			return nil, err
		}

		data, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}

		return &mcp.ToolsCallResult{
			Content: []mcp.Content{{
				Type: "text",
				Text: string(data),
			}},
		}, nil
	}

	srv.RegisterTool(tool, handler)
}
