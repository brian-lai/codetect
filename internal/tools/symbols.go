package tools

import (
	"encoding/json"
	"fmt"

	"codetect/internal/mcp"
	"codetect/internal/search/symbols"
)

// RegisterSymbolTools registers the consolidated symbols MCP tool
func RegisterSymbolTools(server *mcp.Server, config *Config) {
	registerSymbols(server, config)
}

func registerSymbols(server *mcp.Server, config *Config) {
	tool := mcp.Tool{
		Name:        "symbols",
		Description: "Find symbols by name or list all definitions in a file.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"mode": {
					Type:        "string",
					Description: "find (search by name, default) or list (all defs in file)",
				},
				"name": {
					Type:        "string",
					Description: "Symbol name for find mode (partial match supported)",
				},
				"path": {
					Type:        "string",
					Description: "File path for list mode",
				},
				"kind": {
					Type:        "string",
					Description: "Filter: function, type, class, struct, interface, variable, constant",
				},
				"limit": {
					Type:        "number",
					Description: "Max results (default: 20, find mode only)",
				},
			},
		},
	}

	handler := func(args map[string]any) (*mcp.ToolsCallResult, error) {
		mode := "find"
		if m, ok := args["mode"].(string); ok && m != "" {
			mode = m
		}

		// Validate mode and mode-specific parameters before touching the index
		switch mode {
		case "find":
			if name, ok := args["name"].(string); !ok || name == "" {
				return nil, fmt.Errorf("name is required for find mode")
			}
		case "list":
			if path, ok := args["path"].(string); !ok || path == "" {
				return nil, fmt.Errorf("path is required for list mode")
			}
		default:
			return nil, fmt.Errorf("invalid mode %q: use find or list", mode)
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

		switch mode {
		case "find":
			return handleFindSymbol(args, idx)
		case "list":
			return handleListDefs(args, idx)
		default:
			return nil, fmt.Errorf("invalid mode %q: use find or list", mode)
		}
	}

	server.RegisterTool(tool, handler)
}

func handleFindSymbol(args map[string]any, idx *symbols.Index) (*mcp.ToolsCallResult, error) {
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("name is required for find mode")
	}

	kind := ""
	if k, ok := args["kind"].(string); ok {
		kind = k
	}

	limit := 20
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
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

func handleListDefs(args map[string]any, idx *symbols.Index) (*mcp.ToolsCallResult, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("path is required for list mode")
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
