package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"codetect/internal/mcp"
	"codetect/internal/tools"
)

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

func newServer() *mcp.Server {
	server := mcp.NewServer("codetect-cli", cliVersion)
	toolsConfig := tools.DefaultConfigWithEnrichment()
	tools.RegisterAll(server, toolsConfig)
	return server
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
	return runSearchWithServer(args, newServer(), stdout, stderr)
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

func runFile(args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "error: file not yet implemented")
	return 1
}

// --- symbols ---

func runSymbols(args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "error: symbols not yet implemented")
	return 1
}

// --- hybrid ---

func runHybrid(args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "error: hybrid not yet implemented")
	return 1
}
