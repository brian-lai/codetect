package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"codetect/internal/logging"
	"codetect/internal/registry"
)

// RunRegistry dispatches to registry subcommands: list, add, remove, stats.
func RunRegistry(args []string) ExitCode {
	logger := logging.Default("codetect")

	if len(args) == 0 {
		printRegistryUsage(os.Stderr)
		return ExitError
	}

	switch args[0] {
	case "list":
		return registryList(logger)
	case "add":
		return registryAdd(logger, args[1:])
	case "remove":
		return registryRemove(logger, args[1:])
	case "stats":
		return registryStats(logger)
	case "help", "--help", "-h":
		printRegistryUsage(os.Stdout)
		return ExitOK
	default:
		fmt.Fprintf(os.Stderr, "codetect: unknown registry subcommand %q\n", args[0])
		printRegistryUsage(os.Stderr)
		return ExitUnknownArg
	}
}

func printRegistryUsage(w io.Writer) {
	fmt.Fprintf(w, "Usage: codetect registry <command>\n\n")
	fmt.Fprintf(w, "Commands:\n")
	fmt.Fprintf(w, "  list      List registered projects\n")
	fmt.Fprintf(w, "  add       Add current directory to registry\n")
	fmt.Fprintf(w, "  remove    Remove current directory from registry\n")
	fmt.Fprintf(w, "  stats     Show aggregate statistics\n")
}

func registryList(logger interface{ Error(msg string, args ...any) }) ExitCode {
	reg, err := registry.NewRegistry()
	if err != nil {
		logger.Error("failed to load registry", "error", err)
		return ExitError
	}

	projects := reg.List()
	if len(projects) == 0 {
		fmt.Println("No projects registered. Run 'codetect init && codetect index' in a project directory.")
		return ExitOK
	}

	for _, p := range projects {
		lastIndexed := "never"
		if p.LastIndexed != nil {
			lastIndexed = p.LastIndexed.Format("2006-01-02 15:04")
		}
		watched := ""
		if p.WatchEnabled {
			watched = " [watched]"
		}
		fmt.Printf("  %s%s\n    path: %s\n    last indexed: %s\n    embeddings: %d\n\n",
			p.Name, watched, p.Path, lastIndexed, p.IndexStats.Embeddings)
	}
	return ExitOK
}

func registryAdd(logger interface{ Error(msg string, args ...any) }, args []string) ExitCode {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	reg, err := registry.NewRegistry()
	if err != nil {
		logger.Error("failed to load registry", "error", err)
		return ExitError
	}

	if err := reg.Add(path); err != nil {
		logger.Error("failed to add project", "error", err)
		return ExitError
	}

	fmt.Printf("Added %s to registry.\n", path)
	return ExitOK
}

func registryRemove(logger interface{ Error(msg string, args ...any) }, args []string) ExitCode {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	reg, err := registry.NewRegistry()
	if err != nil {
		logger.Error("failed to load registry", "error", err)
		return ExitError
	}

	if err := reg.Remove(path); err != nil {
		logger.Error("failed to remove project", "error", err)
		return ExitError
	}

	fmt.Printf("Removed %s from registry.\n", path)
	return ExitOK
}

func registryStats(logger interface{ Error(msg string, args ...any) }) ExitCode {
	reg, err := registry.NewRegistry()
	if err != nil {
		logger.Error("failed to load registry", "error", err)
		return ExitError
	}

	stats := reg.AggregateStats()
	data, _ := json.MarshalIndent(stats, "", "  ")
	fmt.Println(string(data))
	return ExitOK
}

