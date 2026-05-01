// Package commands dispatches subcommands for the unified `codetect` binary.
//
// Each subcommand is implemented in its own file:
//   - mcp.go    → RunMCP (MCP server on stdio)
//   - init.go   → RunInit (write .mcp.json)
//   - index.go  → RunIndex (index a project)
//   - embed.go  → RunEmbed (generate embeddings)
//   - stats.go  → RunStats (show index statistics)
//   - doctor.go → RunDoctor (health checks)
//   - daemon.go → RunDaemon (daemon start/stop/status)
//   - registry.go → RunRegistry (project registry)
//   - migrate.go  → RunMigrate (SQLite→Postgres migration)
//   - version.go  → RunVersion (print version)
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
