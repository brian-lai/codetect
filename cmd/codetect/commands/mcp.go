package commands

import (
	"os"

	"codetect/internal/logging"
	"codetect/internal/mcp"
	"codetect/internal/tools"
)

// RunMCP starts the MCP server on stdio.
// Moved verbatim from cmd/codetect/main.go.
func RunMCP(_ []string) ExitCode {
	logger := logging.Default("codetect")

	server := mcp.NewServer("codetect", Version)

	toolsConfig := tools.DefaultConfigWithEnrichment()
	if toolsConfig.Pool != nil {
		defer toolsConfig.Pool.Close()
	}

	tools.RegisterAll(server, toolsConfig)

	logger.Info("starting MCP server", "name", "codetect", "version", Version)

	if err := server.Run(); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
	return ExitOK
}
