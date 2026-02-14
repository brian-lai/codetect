package tools

import (
	"encoding/json"
	"fmt"
	"os"

	"codetect/internal/mcp"
	"codetect/internal/search/files"
	"codetect/internal/search/keyword"
)

// RegisterAll registers all available tools on the MCP server
// Phase 2a: Now accepts optional Config for dependency injection (e.g., Enricher).
// Pass nil for config to use defaults (no enrichment, backward compatible).
func RegisterAll(server *mcp.Server, config *Config) {
	if config == nil {
		config = DefaultConfig()
	}
	registerSearchKeyword(server, config)
	registerGetFile(server)
	RegisterSymbolTools(server)
	RegisterV2SemanticTools(server, config)
}

func registerSearchKeyword(server *mcp.Server, config *Config) {
	tool := mcp.Tool{
		Name:        "search_keyword",
		Description: "Regex search via ripgrep. Returns file paths, line numbers, and snippets.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"query": {
					Type:        "string",
					Description: "Search pattern (regex supported)",
				},
				"top_k": {
					Type:        "number",
					Description: "Max results (default: 10)",
				},
				"include_context": {
					Type:        "boolean",
					Description: "Add surrounding scope context (default: true)",
				},
			},
			Required: []string{"query"},
		},
	}

	handler := func(args map[string]any) (*mcp.ToolsCallResult, error) {
		query, ok := args["query"].(string)
		if !ok || query == "" {
			return nil, fmt.Errorf("query is required")
		}

		topK := 10
		if tk, ok := args["top_k"].(float64); ok {
			topK = int(tk)
		}

		// Phase 2a: Check if context enrichment requested
		var includeContext *bool
		if ic, ok := args["include_context"].(bool); ok {
			includeContext = &ic
		}

		// Get current working directory as root
		root, err := os.Getwd()
		if err != nil {
			root = "."
		}

		result, err := keyword.Search(query, root, topK)
		if err != nil {
			return nil, err
		}

		// Phase 2a: Enrich results if enricher available
		if config.Enricher != nil {
			if err := config.Enricher.EnrichKeywordResults(result.Results, includeContext); err != nil {
				// Log but don't fail - enrichment is optional
			}
		}

		// Serialize results to JSON
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

	server.RegisterTool(tool, handler)
}

func registerGetFile(server *mcp.Server) {
	tool := mcp.Tool{
		Name:        "get_file",
		Description: "Read file contents with optional line range.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"path": {
					Type:        "string",
					Description: "File path (relative or absolute)",
				},
				"start_line": {
					Type:        "number",
					Description: "Start line, 1-indexed (omit for beginning)",
				},
				"end_line": {
					Type:        "number",
					Description: "End line, 1-indexed (omit for end)",
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

		// Serialize results to JSON
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

	server.RegisterTool(tool, handler)
}
