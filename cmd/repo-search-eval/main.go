// repo-search-eval runs evaluation tests comparing MCP vs non-MCP performance.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"repo-search/evals"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		runEval(os.Args[2:])
	case "report":
		showReport(os.Args[2:])
	case "list":
		listCases(os.Args[2:])
	case "version":
		fmt.Printf("repo-search-eval v%s\n", version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func runEval(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	repoPath := fs.String("repo", ".", "Path to repository to evaluate")
	casesDir := fs.String("cases", "evals/cases", "Directory containing test case JSONL files")
	outputDir := fs.String("output", "evals/results", "Output directory for results")
	categories := fs.String("category", "", "Filter by category (comma-separated: search,navigate,understand)")
	timeout := fs.Duration("timeout", 5*time.Minute, "Timeout per test case")
	verbose := fs.Bool("verbose", false, "Verbose output")
	fs.Parse(args)

	config := evals.DefaultConfig()
	config.RepoPath = *repoPath
	config.OutputDir = *outputDir
	config.Timeout = *timeout
	config.Verbose = *verbose

	if *categories != "" {
		config.Categories = strings.Split(*categories, ",")
	}

	// Convert to absolute paths
	absRepoPath, err := filepath.Abs(config.RepoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid repo path: %v\n", err)
		os.Exit(1)
	}
	config.RepoPath = absRepoPath

	absCasesDir, err := filepath.Abs(*casesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid cases dir: %v\n", err)
		os.Exit(1)
	}

	// Create runner and load test cases
	runner := evals.NewRunner(config)
	cases, err := runner.LoadTestCases(absCasesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading test cases: %v\n", err)
		os.Exit(1)
	}

	if len(cases) == 0 {
		fmt.Fprintln(os.Stderr, "no test cases found")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Running %d test cases against %s\n", len(cases), absRepoPath)
	fmt.Fprintf(os.Stderr, "This will run each test case twice (with and without MCP)...\n\n")

	// Run evaluation
	ctx := context.Background()
	report, err := runner.RunAll(ctx, cases)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error running evaluation: %v\n", err)
		os.Exit(1)
	}

	// Validate results
	validator := evals.NewValidator()
	validator.ValidateAll(cases, report)

	// Generate summary
	reporter := evals.NewReporter()
	reporter.GenerateSummary(report)

	// Save results
	if err := runner.SaveResults(report); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save results: %v\n", err)
	}

	// Print report
	reporter.PrintReportToStdout(report)
}

func showReport(args []string) {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	resultsPath := fs.String("results", "", "Path to results JSON file")
	fs.Parse(args)

	if *resultsPath == "" {
		// Find most recent results file
		files, err := filepath.Glob("evals/results/*-results.json")
		if err != nil || len(files) == 0 {
			fmt.Fprintln(os.Stderr, "error: no results file found. Use -results flag or run 'eval run' first.")
			os.Exit(1)
		}
		*resultsPath = files[len(files)-1] // Most recent
	}

	reporter := evals.NewReporter()
	report, err := reporter.LoadReport(*resultsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading report: %v\n", err)
		os.Exit(1)
	}

	reporter.PrintReportToStdout(report)
}

func listCases(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	casesDir := fs.String("cases", "evals/cases", "Directory containing test case JSONL files")
	category := fs.String("category", "", "Filter by category")
	fs.Parse(args)

	config := evals.DefaultConfig()
	if *category != "" {
		config.Categories = []string{*category}
	}

	runner := evals.NewRunner(config)
	cases, err := runner.LoadTestCases(*casesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading test cases: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Test Cases (%d total):\n\n", len(cases))
	for _, tc := range cases {
		fmt.Printf("  %-15s [%-10s] %s\n", tc.ID, tc.Category, tc.Description)
	}
}

func printUsage() {
	fmt.Println(`repo-search-eval - Evaluate MCP vs non-MCP performance

Usage:
  repo-search-eval <command> [options]

Commands:
  run      Run evaluation test cases
  report   Display a saved report
  list     List available test cases
  version  Print version
  help     Show this help

Run Options:
  --repo <path>      Repository to evaluate (default: .)
  --cases <dir>      Test cases directory (default: evals/cases)
  --output <dir>     Output directory (default: evals/results)
  --category <cat>   Filter by category (search,navigate,understand)
  --timeout <dur>    Timeout per test (default: 5m)
  --verbose          Verbose output

Report Options:
  --results <path>   Path to results JSON file

Examples:
  # Run all tests on current directory
  repo-search-eval run

  # Run only search tests on a specific repo
  repo-search-eval run --repo /path/to/project --category search

  # View the most recent report
  repo-search-eval report

  # List available test cases
  repo-search-eval list`)
}
