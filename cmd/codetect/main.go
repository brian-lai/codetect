package main

import (
	"os"

	"codetect/internal/logging"
	"codetect/internal/mcp"
	"codetect/internal/tools"
)

const (
	serverName    = "codetect"
	serverVersion = "3.3.2"
)

func main() {
	logger := logging.Default("codetect")

	server := mcp.NewServer(serverName, serverVersion)

	toolsConfig := tools.DefaultConfigWithEnrichment()
	if toolsConfig.Pool != nil {
		defer toolsConfig.Pool.Close()
	}

	tools.RegisterAll(server, toolsConfig)

	logger.Info("starting MCP server", "name", serverName, "version", serverVersion)

	if err := server.Run(); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
