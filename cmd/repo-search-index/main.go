package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"repo-search/internal/search/symbols"
)

const version = "0.2.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "index":
		runIndex(os.Args[2:])

	case "stats":
		runStats(os.Args[2:])

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

func runIndex(args []string) {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	force := fs.Bool("force", false, "Force full reindex")
	fs.Parse(args)

	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid path: %v\n", err)
		os.Exit(1)
	}

	// Check if ctags is available
	if !symbols.CtagsAvailable() {
		fmt.Fprintln(os.Stderr, "[repo-search-index] warning: universal-ctags not found")
		fmt.Fprintln(os.Stderr, "[repo-search-index] symbol indexing will be skipped")
		fmt.Fprintln(os.Stderr, "[repo-search-index] install with: brew install universal-ctags (macOS)")
		os.Exit(0)
	}

	// Create .repo_search directory
	indexDir := filepath.Join(absPath, ".repo_search")
	if err := os.MkdirAll(indexDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error: creating index directory: %v\n", err)
		os.Exit(1)
	}

	dbPath := filepath.Join(indexDir, "symbols.db")
	fmt.Fprintf(os.Stderr, "[repo-search-index] indexing %s\n", absPath)
	fmt.Fprintf(os.Stderr, "[repo-search-index] database: %s\n", dbPath)

	start := time.Now()

	// Open or create index
	idx, err := symbols.NewIndex(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: opening index: %v\n", err)
		os.Exit(1)
	}
	defer idx.Close()

	// Run indexing
	if *force {
		fmt.Fprintln(os.Stderr, "[repo-search-index] running full reindex...")
		if err := idx.FullReindex(absPath); err != nil {
			fmt.Fprintf(os.Stderr, "error: indexing failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Fprintln(os.Stderr, "[repo-search-index] running incremental index...")
		if err := idx.Update(absPath); err != nil {
			fmt.Fprintf(os.Stderr, "error: indexing failed: %v\n", err)
			os.Exit(1)
		}
	}

	// Print stats
	symbolCount, fileCount, err := idx.Stats()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not get stats: %v\n", err)
	} else {
		elapsed := time.Since(start)
		fmt.Fprintf(os.Stderr, "[repo-search-index] indexed %d symbols from %d files in %v\n",
			symbolCount, fileCount, elapsed.Round(time.Millisecond))
	}
}

func runStats(args []string) {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid path: %v\n", err)
		os.Exit(1)
	}

	dbPath := filepath.Join(absPath, ".repo_search", "symbols.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "error: no index found (run 'index' first)")
		os.Exit(1)
	}

	idx, err := symbols.NewIndex(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: opening index: %v\n", err)
		os.Exit(1)
	}
	defer idx.Close()

	symbolCount, fileCount, err := idx.Stats()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: getting stats: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Index: %s\n", dbPath)
	fmt.Printf("Symbols: %d\n", symbolCount)
	fmt.Printf("Files: %d\n", fileCount)
}

func printUsage() {
	fmt.Println(`repo-search-index - Codebase indexer for repo-search MCP

Usage:
  repo-search-index index [--force] [path]   Index a repository
  repo-search-index stats [path]             Show index statistics
  repo-search-index version                  Print version
  repo-search-index help                     Show this help

Options:
  --force    Force full reindex (default: incremental)

The index is stored in .repo_search/symbols.db relative to the indexed path.

Requirements:
  - universal-ctags (for symbol extraction)

Install ctags:
  macOS:   brew install universal-ctags
  Ubuntu:  apt install universal-ctags
  Fedora:  dnf install ctags`)
}
