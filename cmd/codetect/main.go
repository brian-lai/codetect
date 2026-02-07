package main

import (
	"os"

	"codetect/internal/logging"
	"codetect/internal/mcp"
	"codetect/internal/tools"
)

const (
	serverName    = "codetect"
	serverVersion = "2.2.2"
)

func main() {
	logger := logging.Default("codetect")

	server := mcp.NewServer(serverName, serverVersion)

	// Phase 2a: Enable rich context enrichment by default
	// This reduces token usage by ~40% by including scope info and context lines
	// in search results, avoiding full file reads.
	toolsConfig := tools.DefaultConfigWithEnrichment()

	// Register all tools with enrichment enabled
	tools.RegisterAll(server, toolsConfig)

	logger.Info("starting MCP server", "name", serverName, "version", serverVersion)

	if err := server.Run(); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
