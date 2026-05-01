package commands

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
)

//go:embed templates/mcp.json
var mcpJSONTemplate []byte

// RunInit writes .mcp.json in the current directory from templates/mcp.json.
// Flags: --force (overwrite existing .mcp.json).
func RunInit(args []string) ExitCode {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	force := fs.Bool("force", false, "Overwrite existing .mcp.json")
	fs.Parse(args)

	dest := ".mcp.json"
	if _, err := os.Stat(dest); err == nil && !*force {
		fmt.Fprintf(os.Stderr, "codetect: '.mcp.json' already exists; use --force to overwrite\n")
		return ExitError
	}

	if err := os.WriteFile(dest, mcpJSONTemplate, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "codetect: writing .mcp.json: %v\n", err)
		return ExitError
	}

	fmt.Printf("Created .mcp.json\n")
	fmt.Printf("Now run: codetect index\n")
	return ExitOK
}
