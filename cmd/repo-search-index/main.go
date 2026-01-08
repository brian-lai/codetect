package main

import (
	"fmt"
	"os"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "index":
		// Phase 1: no-op indexer
		// In Phase 2, this will invoke universal-ctags and build a SQLite symbol index
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: repo-search-index index <path>")
			os.Exit(1)
		}
		path := os.Args[2]
		fmt.Fprintf(os.Stderr, "[repo-search-index] indexing %s (no-op in Phase 1)\n", path)
		// Success - nothing to do in Phase 1
		os.Exit(0)

	case "version":
		fmt.Printf("repo-search-index v%s\n", version)

	case "help", "-h", "--help":
		printUsage()

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`repo-search-index - Codebase indexer for repo-search MCP

Usage:
  repo-search-index index <path>   Index a repository (no-op in Phase 1)
  repo-search-index version        Print version
  repo-search-index help           Show this help

Phase 1: This command is a placeholder. Real indexing (ctags → SQLite)
         will be implemented in Phase 2.`)
}
