// Package commands dispatches subcommands for the unified `codetect` binary.
//
// STUB — created by /para:plan for 2026-05-01-codetect-tier1-unbreak.
// Implementation is provided by plan phase 1. Each Dispatch* function currently
// returns 501 / "not implemented" so that wiring can be tested before moving
// the real logic from cmd/codetect-index, cmd/codetect-daemon, and
// cmd/migrate-to-postgres into this package.
package commands

import (
	"fmt"
	"io"
	"os"
)

// ExitCode is the process exit code for a subcommand. See spec §1.2 and D7
// in the master plan. NOTE: exit 2 is overloaded between "unknown subcommand"
// (Dispatch-level parse failure) and "embedding catastrophe" (health check).
// This is acceptable because the two are disambiguated by stderr output: a
// parse failure prints "unknown subcommand" immediately, while an embedding
// failure prints the spec §3.3 banner. CI systems that only check exit status
// will correctly treat both as failures; the distinction matters only for
// human diagnosis.
type ExitCode int

const (
	ExitOK ExitCode = 0
	// ExitError is the generic error code (e.g. init file exists, index open fails).
	ExitError ExitCode = 1
	// ExitDegraded indicates a partial embedding failure (0 < health_ratio < 0.80).
	// Banner + sentinel are written. The partial index is still usable.
	ExitDegraded ExitCode = 1
	// ExitEmbeddingFail indicates total embedding failure (ChunksEmbedded == 0
	// && ChunksCreated > 0). Banner + sentinel written with severity=failed.
	ExitEmbeddingFail ExitCode = 2
	// ExitUnknownArg is returned for an unrecognized subcommand or flag.
	// Collides with ExitEmbeddingFail on purpose — see type doc.
	ExitUnknownArg ExitCode = 2
)

// Dispatch routes os.Args[1:] to the appropriate subcommand handler.
// argv[0] is the subcommand name; argv[1:] are flags/positionals for that command.
// Returns the exit code to pass to os.Exit.
func Dispatch(argv []string) ExitCode {
	if len(argv) == 0 {
		return RunMCP(nil)
	}

	sub, rest := argv[0], argv[1:]
	switch sub {
	case "mcp", "serve":
		return RunMCP(rest)
	case "init":
		return RunInit(rest)
	case "index":
		return RunIndex(rest)
	case "embed":
		return RunEmbed(rest)
	case "stats":
		return RunStats(rest)
	case "doctor":
		return RunDoctor(rest)
	case "daemon":
		return RunDaemon(rest)
	case "registry":
		return RunRegistry(rest)
	case "migrate-to-postgres":
		return RunMigrate(rest)
	case "version", "-v", "--version":
		return RunVersion(rest)
	case "help", "-h", "--help":
		PrintHelp(os.Stdout)
		return ExitOK
	default:
		fmt.Fprintf(os.Stderr, "codetect: unknown subcommand %q\n", sub)
		PrintHelp(os.Stderr)
		return ExitUnknownArg
	}
}

// RunMCP starts the MCP server on stdio. No flags; reads stdin until EOF.
func RunMCP(args []string) ExitCode {
	panic("not implemented: cmd/codetect/commands.RunMCP")
}

// RunInit writes .mcp.json in cwd from templates/mcp.json.
// Flags: --force (overwrite existing .mcp.json).
func RunInit(args []string) ExitCode {
	panic("not implemented: cmd/codetect/commands.RunInit")
}

// RunIndex performs incremental or full indexing of the given path (default ".").
// Flags match today's codetect-index index: --force/-f, --clear-cache, --v1
// (removed in phase 4), --verbose/-v, --json, --parallel/-j.
// Exit code 2 if embedding health check trips (phase 3).
func RunIndex(args []string) ExitCode {
	panic("not implemented: cmd/codetect/commands.RunIndex")
}

// RunEmbed generates embeddings for an existing chunked index.
// Flags match today's codetect-index embed.
// Exit code 2 if health check trips.
func RunEmbed(args []string) ExitCode {
	panic("not implemented: cmd/codetect/commands.RunEmbed")
}

// RunStats prints index statistics. Flags: --json.
func RunStats(args []string) ExitCode {
	panic("not implemented: cmd/codetect/commands.RunStats")
}

// RunDoctor runs system health checks. See spec §3.6.
// Exit code 0 healthy, 1 unhealthy (sentinel present or check failed), 2 fatal.
func RunDoctor(args []string) ExitCode {
	panic("not implemented: cmd/codetect/commands.RunDoctor")
}

// RunDaemon dispatches to daemon subcommands: start, stop, status, logs.
func RunDaemon(args []string) ExitCode {
	panic("not implemented: cmd/codetect/commands.RunDaemon")
}

// RunRegistry dispatches to registry subcommands: list, add, remove, stats.
func RunRegistry(args []string) ExitCode {
	panic("not implemented: cmd/codetect/commands.RunRegistry")
}

// RunMigrate runs the SQLite→Postgres migration utility (formerly cmd/migrate-to-postgres).
func RunMigrate(args []string) ExitCode {
	panic("not implemented: cmd/codetect/commands.RunMigrate")
}

// RunVersion prints the version string and exits.
func RunVersion(args []string) ExitCode {
	panic("not implemented: cmd/codetect/commands.RunVersion")
}

// PrintHelp writes help text listing all subcommands to w.
func PrintHelp(w io.Writer) {
	fmt.Fprint(w, helpText)
}

const helpText = `codetect - fast, token-efficient code search for LLMs

Usage:
  codetect                  Start MCP server on stdio (same as 'codetect mcp')
  codetect mcp              Start MCP server on stdio
  codetect init             Create .mcp.json in current directory
  codetect index [path]     Index a project (symbols + embeddings)
  codetect embed [path]     Generate embeddings for an existing index
  codetect stats [path]     Show index statistics
  codetect doctor [path]    Run system health checks
  codetect daemon <cmd>     Manage the background indexing daemon (start|stop|status)
  codetect registry <cmd>   Manage the project registry
  codetect version          Print version
  codetect help             Show this help

Run 'codetect <command> --help' for command-specific flags.

Environment:
  CODETECT_EMBEDDING_PROVIDER   ollama (default) | litellm | off
  CODETECT_EMBEDDING_MODEL      Model name (default: nomic-embed-text)
  CODETECT_DB_TYPE              sqlite (default) | postgres
  CODETECT_DB_DSN               PostgreSQL DSN when CODETECT_DB_TYPE=postgres
`
