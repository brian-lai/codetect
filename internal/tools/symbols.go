package tools

import (
	"encoding/json"
	"fmt"

	"codetect/internal/mcp"
	"codetect/internal/search/symbols"
)

// RegisterSymbolTools registers the symbol-related MCP tools
func RegisterSymbolTools(server *mcp.Server, config *Config) {
	registerFindSymbol(server, config)
	registerListDefsInFile(server, config)
}

func registerFindSymbol(server *mcp.Server, config *Config) {
	tool := mcp.Tool{
		Name:        "find_symbol",
		Description: "Find symbol definitions by name with fuzzy matching.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"name": {
					Type:        "string",
					Description: "Symbol name (partial match supported)",
				},
				"kind": {
					Type:        "string",
					Description: "Filter: function, type, class, struct, interface, variable, constant",
				},
				"limit": {
					Type:        "number",
					Description: "Max results (default: 20)",
				},
			},
			Required: []string{"name"},
		},
	}

	handler := func(args map[string]any) (*mcp.ToolsCallResult, error) {
		name, ok := args["name"].(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("name is required")
		}

		kind := ""
		if k, ok := args["kind"].(string); ok {
			kind = k
		}

		limit := 20
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		if config.Pool == nil {
			return nil, fmt.Errorf("resource pool not initialized")
		}

		idx, err := config.Pool.SymbolIndex()
		if err != nil {
			return &mcp.ToolsCallResult{
				Content: []mcp.Content{{
					Type: "text",
					Text: fmt.Sprintf(`{"available": false, "error": %q}`, err.Error()),
				}},
			}, nil
		}

		syms, err := idx.FindSymbol(name, kind, limit)
		if err != nil {
			return nil, fmt.Errorf("searching symbols: %w", err)
		}

		result := symbols.FindSymbolResult{
			Symbols: syms,
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

	server.RegisterTool(tool, handler)
}

func registerListDefsInFile(server *mcp.Server, config *Config) {
	tool := mcp.Tool{
		Name:        "list_defs_in_file",
		Description: "List all definitions in a file (functions, types, variables).",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"path": {
					Type:        "string",
					Description: "File path",
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

		if config.Pool == nil {
			return nil, fmt.Errorf("resource pool not initialized")
		}

		idx, err := config.Pool.SymbolIndex()
		if err != nil {
			return &mcp.ToolsCallResult{
				Content: []mcp.Content{{
					Type: "text",
					Text: fmt.Sprintf(`{"available": false, "error": %q}`, err.Error()),
				}},
			}, nil
		}

		syms, err := idx.ListDefsInFile(path)
		if err != nil {
			return nil, fmt.Errorf("listing symbols: %w", err)
		}

		result := symbols.ListDefsResult{
			Path:    path,
			Symbols: syms,
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

	server.RegisterTool(tool, handler)
}
