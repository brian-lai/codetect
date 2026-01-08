BINARY=dist/repo-search
INDEXER=dist/repo-search-index

.PHONY: build mcp index doctor clean test

# Build both binaries
build:
	@mkdir -p dist
	go build -o $(BINARY) ./cmd/repo-search
	go build -o $(INDEXER) ./cmd/repo-search-index

# Run MCP server (used by .mcp.json)
mcp: build
	@./$(BINARY)

# Run indexer (no-op in Phase 1)
index: build
	@./$(INDEXER) index .

# Check dependencies
doctor:
	@echo "Checking dependencies..."
	@command -v go >/dev/null 2>&1 || { echo "❌ missing: go"; exit 1; }
	@echo "✓ go: $$(go version | cut -d' ' -f3)"
	@command -v rg >/dev/null 2>&1 || { echo "❌ missing: ripgrep (rg)"; exit 1; }
	@echo "✓ ripgrep: $$(rg --version | head -1)"
	@echo ""
	@echo "All dependencies satisfied ✓"

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf dist .repo_search
