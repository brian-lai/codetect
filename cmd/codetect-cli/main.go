package main

import (
	"fmt"
	"io"
	"os"
)

const version = "3.7.7"

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
		fmt.Fprintf(stdout, "codetect-cli v%s\n", version)
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

// Stubs — implemented in subsequent steps
func runSearch(args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "error: search not yet implemented")
	return 1
}

func runFile(args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "error: file not yet implemented")
	return 1
}

func runSymbols(args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "error: symbols not yet implemented")
	return 1
}

func runHybrid(args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "error: hybrid not yet implemented")
	return 1
}
