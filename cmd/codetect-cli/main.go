package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"codetect/internal/mcp"
	"codetect/internal/tools"
)

// Keep in sync with cmd/codetect/main.go serverVersion
const cliVersion = "3.7.7"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "error: no command specified")
		printUsage(stderr)
		return 1
	}

	switch args[0] {
	case "search":
		return runSearch(args[1:], stdout, stderr)
	case "file":
		return runFile(args[1:], stdout, stderr)
	case "symbols":
		return runSymbols(args[1:], stdout, stderr)
	case "hybrid":
		return runHybrid(args[1:], stdout, stderr)
	case "version":
		fmt.Fprintf(stdout, "codetect-cli v%s\n", cliVersion)
		return 0
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "error: unknown command %q\n", args[0])
		printUsage(stderr)
		return 1
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: codetect-cli <command> [args]

Commands:
  search   Regex search via ripgrep (wraps search_keyword)
  file     Read file contents with optional line range (wraps get_file)
  symbols  Find or list symbol definitions (wraps symbols)
  hybrid   Hybrid keyword + semantic search (wraps hybrid_search_v2)
  version  Print version
  help     Show this help`)
}

// newServerWithCleanup creates a server with all tools registered and returns a cleanup function.
func newServerWithCleanup() (*mcp.Server, func()) {
	server := mcp.NewServer("codetect-cli", cliVersion)
	toolsConfig := tools.DefaultConfigWithEnrichment()
	tools.RegisterAll(server, toolsConfig)
	cleanup := func() {
		if toolsConfig.Pool != nil {
			toolsConfig.Pool.Close()
		}
	}
	return server, cleanup
}

// callTool invokes a registered tool and writes the result to stdout.
func callTool(server *mcp.Server, name string, args map[string]any, stdout, stderr io.Writer) int {
	result, err := server.CallTool(name, args)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if result.IsError {
		for _, c := range result.Content {
			fmt.Fprintln(stderr, c.Text)
		}
		return 1
	}
	for _, c := range result.Content {
		fmt.Fprintln(stdout, c.Text)
	}
	return 0
}

// --- search ---

func buildSearchArgs(args []string) (map[string]any, error) {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	topK := fs.Int("top-k", 0, "Max results (default: 10)")
	detail := fs.String("detail", "", "Response detail: minimal, standard, rich")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() == 0 {
		return nil, fmt.Errorf("query is required")
	}
	m := map[string]any{"query": fs.Arg(0)}
	if *topK > 0 {
		m["top_k"] = float64(*topK)
	}
	if *detail != "" {
		m["detail"] = *detail
	}
	return m, nil
}

func runSearch(args []string, stdout, stderr io.Writer) int {
	server, cleanup := newServerWithCleanup()
	defer cleanup()
	return runSearchWithServer(args, server, stdout, stderr)
}

func runSearchWithServer(args []string, server *mcp.Server, stdout, stderr io.Writer) int {
	m, err := buildSearchArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return callTool(server, "search_keyword", m, stdout, stderr)
}

// --- file ---

func buildFileArgs(args []string) (map[string]any, error) {
	fs := flag.NewFlagSet("file", flag.ContinueOnError)
	startLine := fs.Int("start-line", 0, "Start line, 1-indexed")
	endLine := fs.Int("end-line", 0, "End line, 1-indexed")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() == 0 {
		return nil, fmt.Errorf("path is required")
	}
	m := map[string]any{"path": fs.Arg(0)}
	if *startLine > 0 {
		m["start_line"] = float64(*startLine)
	}
	if *endLine > 0 {
		m["end_line"] = float64(*endLine)
	}
	return m, nil
}

func runFile(args []string, stdout, stderr io.Writer) int {
	server, cleanup := newServerWithCleanup()
	defer cleanup()
	return runFileWithServer(args, server, stdout, stderr)
}

func runFileWithServer(args []string, server *mcp.Server, stdout, stderr io.Writer) int {
	m, err := buildFileArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return callTool(server, "get_file", m, stdout, stderr)
}

// --- symbols ---

func buildSymbolsArgs(args []string) (map[string]any, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("subcommand required: find or list")
	}

	mode := args[0]
	switch mode {
	case "find":
		fs := flag.NewFlagSet("symbols find", flag.ContinueOnError)
		kind := fs.String("kind", "", "Symbol kind filter: function, type, class, etc.")
		limit := fs.Int("limit", 0, "Max results (default: 20)")
		if err := fs.Parse(args[1:]); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return nil, fmt.Errorf("name is required for find mode")
		}
		m := map[string]any{"mode": "find", "name": fs.Arg(0)}
		if *kind != "" {
			m["kind"] = *kind
		}
		if *limit > 0 {
			m["limit"] = float64(*limit)
		}
		return m, nil

	case "list":
		fs := flag.NewFlagSet("symbols list", flag.ContinueOnError)
		if err := fs.Parse(args[1:]); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return nil, fmt.Errorf("path is required for list mode")
		}
		return map[string]any{"mode": "list", "path": fs.Arg(0)}, nil

	default:
		return nil, fmt.Errorf("unknown symbols subcommand %q: use find or list", mode)
	}
}

func runSymbols(args []string, stdout, stderr io.Writer) int {
	server, cleanup := newServerWithCleanup()
	defer cleanup()
	return runSymbolsWithServer(args, server, stdout, stderr)
}

func runSymbolsWithServer(args []string, server *mcp.Server, stdout, stderr io.Writer) int {
	m, err := buildSymbolsArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return callTool(server, "symbols", m, stdout, stderr)
}

// --- hybrid ---

func buildHybridArgs(args []string) (map[string]any, error) {
	fs := flag.NewFlagSet("hybrid", flag.ContinueOnError)
	limit := fs.Int("limit", 0, "Max results (default: 10)")
	rerank := fs.Bool("rerank", false, "Enable reranking")
	detail := fs.String("detail", "", "Response detail: minimal, standard, rich")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() == 0 {
		return nil, fmt.Errorf("query is required")
	}
	m := map[string]any{"query": fs.Arg(0)}
	if *limit > 0 {
		m["limit"] = float64(*limit)
	}
	if *rerank {
		m["rerank"] = true
	}
	if *detail != "" {
		m["detail"] = *detail
	}
	return m, nil
}

func runHybrid(args []string, stdout, stderr io.Writer) int {
	server, cleanup := newServerWithCleanup()
	defer cleanup()
	return runHybridWithServer(args, server, stdout, stderr)
}

func runHybridWithServer(args []string, server *mcp.Server, stdout, stderr io.Writer) int {
	m, err := buildHybridArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return callTool(server, "hybrid_search_v2", m, stdout, stderr)
}
