package tools

import (
	"encoding/json"
	"fmt"

	"codetect/internal/mcp"
	"codetect/internal/search/symbols"
)

// RegisterReferenceTools registers the symbol reference-related MCP tools (Phase 2b)
func RegisterReferenceTools(server *mcp.Server) {
	registerFindReferences(server)
	registerFindCallers(server)
	registerFindImplementations(server)
}

func registerFindReferences(server *mcp.Server) {
	tool := mcp.Tool{
		Name:        "find_references",
		Description: "Find all references to a symbol (calls, type usages)",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"symbol": {
					Type:        "string",
					Description: "Symbol name to find references for",
				},
				"kind": {
					Type:        "string",
					Description: "Filter by reference kind: call, type_ref, all (default: all)",
				},
				"limit": {
					Type:        "number",
					Description: "Max results (default: 50)",
				},
			},
			Required: []string{"symbol"},
		},
	}

	handler := func(args map[string]any) (*mcp.ToolsCallResult, error) {
		symbol, ok := args["symbol"].(string)
		if !ok || symbol == "" {
			return nil, fmt.Errorf("symbol is required")
		}

		kind := "all"
		if k, ok := args["kind"].(string); ok {
			kind = k
		}

		limit := 50
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		// Open index
		idx, err := openIndex()
		if err != nil {
			return &mcp.ToolsCallResult{
				Content: []mcp.Content{{
					Type: "text",
					Text: fmt.Sprintf(`{"available": false, "error": %q}`, err.Error()),
				}},
			}, nil
		}
		defer idx.Close()

		// Query references
		refs, err := idx.QueryRefsByName(symbol, kind, limit)
		if err != nil {
			return nil, fmt.Errorf("querying references: %w", err)
		}

		result := FindReferencesResult{
			Symbol:     symbol,
			References: refs,
			Count:      len(refs),
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

func registerFindCallers(server *mcp.Server) {
	tool := mcp.Tool{
		Name:        "find_callers",
		Description: "Find all functions that call the given function",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"symbol": {
					Type:        "string",
					Description: "Function name to find callers for",
				},
				"limit": {
					Type:        "number",
					Description: "Max results (default: 20)",
				},
			},
			Required: []string{"symbol"},
		},
	}

	handler := func(args map[string]any) (*mcp.ToolsCallResult, error) {
		symbol, ok := args["symbol"].(string)
		if !ok || symbol == "" {
			return nil, fmt.Errorf("symbol is required")
		}

		limit := 20
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		// Open index
		idx, err := openIndex()
		if err != nil {
			return &mcp.ToolsCallResult{
				Content: []mcp.Content{{
					Type: "text",
					Text: fmt.Sprintf(`{"available": false, "error": %q}`, err.Error()),
				}},
			}, nil
		}
		defer idx.Close()

		// Query call references (kind="call")
		refs, err := idx.QueryRefsByName(symbol, "call", limit)
		if err != nil {
			return nil, fmt.Errorf("querying callers: %w", err)
		}

		// Build caller info from refs
		callers := make([]CallerInfo, len(refs))
		for i, ref := range refs {
			callers[i] = CallerInfo{
				Scope:      ref.SourceScope,
				Path:       ref.SourcePath,
				Line:       ref.SourceLine,
				Kind:       "call",
				QualifiedName: ref.QualifiedName,
			}
		}

		result := FindCallersResult{
			Symbol:  symbol,
			Callers: callers,
			Count:   len(callers),
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

func registerFindImplementations(server *mcp.Server) {
	tool := mcp.Tool{
		Name:        "find_implementations",
		Description: "Find types that implement an interface or extend a class",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.Property{
				"symbol": {
					Type:        "string",
					Description: "Interface or base class name",
				},
				"limit": {
					Type:        "number",
					Description: "Max results (default: 20)",
				},
			},
			Required: []string{"symbol"},
		},
	}

	handler := func(args map[string]any) (*mcp.ToolsCallResult, error) {
		symbol, ok := args["symbol"].(string)
		if !ok || symbol == "" {
			return nil, fmt.Errorf("symbol is required")
		}

		limit := 20
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}

		// Open index
		idx, err := openIndex()
		if err != nil {
			return &mcp.ToolsCallResult{
				Content: []mcp.Content{{
					Type: "text",
					Text: fmt.Sprintf(`{"available": false, "error": %q}`, err.Error()),
				}},
			}, nil
		}
		defer idx.Close()

		// Query type relations
		rels, err := idx.QueryImplementations(symbol, limit)
		if err != nil {
			return nil, fmt.Errorf("querying implementations: %w", err)
		}

		// Convert to implementation info
		impls := make([]ImplementationInfo, len(rels))
		for i, rel := range rels {
			impls[i] = ImplementationInfo{
				ChildType:  rel.ChildType,
				ParentType: rel.ParentType,
				Relation:   rel.Relation,
				Path:       rel.Path,
				Line:       rel.Line,
			}
		}

		result := FindImplementationsResult{
			Symbol:          symbol,
			Implementations: impls,
			Count:           len(impls),
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

// Result types for Phase 2b tools

type FindReferencesResult struct {
	Symbol     string              `json:"symbol"`
	References []symbols.SymbolRef `json:"references"`
	Count      int                 `json:"count"`
}

type FindCallersResult struct {
	Symbol  string       `json:"symbol"`
	Callers []CallerInfo `json:"callers"`
	Count   int          `json:"count"`
}

type CallerInfo struct {
	Scope         string `json:"scope"`
	Path          string `json:"path"`
	Line          int    `json:"line"`
	Kind          string `json:"kind"`
	QualifiedName string `json:"qualified_name,omitempty"`
}

type FindImplementationsResult struct {
	Symbol          string               `json:"symbol"`
	Implementations []ImplementationInfo `json:"implementations"`
	Count           int                  `json:"count"`
}

type ImplementationInfo struct {
	ChildType  string `json:"child_type"`
	ParentType string `json:"parent_type"`
	Relation   string `json:"relation"`
	Path       string `json:"path"`
	Line       int    `json:"line"`
}
