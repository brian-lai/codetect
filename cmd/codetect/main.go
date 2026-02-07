package main

import (
	"os"

	"codetect/internal/logging"
	"codetect/internal/mcp"
	"codetect/internal/tools"
)

const (
	serverName    = "codetect"
	serverVersion = "2.1.1"
)

func main() {
	logger := logging.Default("codetect")

	server := mcp.NewServer(serverName, serverVersion)

	// Register all tools
	// Phase 2a: Pass nil for backward compatibility (no enrichment by default)
	// To enable enrichment, create tools.Config with an Enricher and pass it here
	tools.RegisterAll(server, nil)

	logger.Info("starting MCP server", "name", serverName, "version", serverVersion)

	if err := server.Run(); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
