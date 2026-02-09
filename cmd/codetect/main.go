package main

import (
	"os"

	"codetect/internal/logging"
	"codetect/internal/mcp"
	"codetect/internal/server"
	"codetect/internal/tools"
)

const (
	serverName    = "codetect"
	serverVersion = "4.0.0-dev"
)

func main() {
	logger := logging.Default("codetect")

	// Initialize session-scoped components once at startup
	ctx, err := server.NewContext()
	if err != nil {
		logger.Error("failed to initialize", "error", err)
		os.Exit(1)
	}
	defer ctx.Close()

	srv := mcp.NewServer(serverName, serverVersion)

	// Register v4 tools: search + get_file
	tools.RegisterAll(srv, ctx)

	logger.Info("starting MCP server",
		"name", serverName,
		"version", serverVersion,
		"semantic", ctx.SemanticOK,
		"symbols", ctx.SymbolsOK,
	)

	if err := srv.Run(); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
