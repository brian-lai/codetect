# HTTP API Design for codetect

**Date:** 2026-02-03
**Purpose:** Design RESTful API wrapper around MCP tools for ecosystem growth
**Designer:** Claude Code

---

## Executive Summary

This document specifies a RESTful HTTP API that wraps codetect's MCP tools, enabling integration with non-MCP tools and services. The API follows REST principles, uses JSON for all payloads, and supports both local (no auth) and cloud (API key) deployment modes.

**Key Features:**
- ✅ All 6 MCP tools exposed as HTTP endpoints
- ✅ OpenAPI 3.0 specification for automatic client generation
- ✅ Flexible authentication (none for local, API keys for cloud)
- ✅ Rate limiting support for hosted tier
- ✅ WebSocket support for streaming search results (future)

**Timeline:** Phase 1e implementation (3-4 weeks)

---

## 1. Design Principles

### REST-First Architecture

**Why REST over RPC?**
- ✅ **Familiar:** Most developers know REST
- ✅ **Tooling:** cURL, Postman, HTTPie work out-of-the-box
- ✅ **Cacheable:** HTTP caching works naturally
- ✅ **Language-agnostic:** Any HTTP client works

**Alternative considered:** gRPC (rejected for complexity)

### JSON All the Way

**Request format:** JSON (`Content-Type: application/json`)
**Response format:** JSON (`Content-Type: application/json`)
**Error format:** JSON with RFC 7807 Problem Details

**Why not support other formats?**
- Simplicity: One format to test and document
- JSON is universal: Every language has JSON support
- MCP uses JSON: Natural mapping

### Versioning Strategy

**URL-based versioning:** `/api/v1/...`

**Why URL versioning?**
- Clear and explicit
- Easy to route and cache
- Industry standard (Stripe, GitHub, Twilio all use it)

**Version lifecycle:**
- v1: Initial release (Phase 1e)
- v2: Future breaking changes (if needed)
- Support N-1 versions (e.g., v1 and v2 simultaneously)

---

## 2. Authentication & Authorization

### Local Mode (No Auth)

**Scenario:** User runs `codetect serve` on localhost

**Configuration:**
```yaml
# .codetect.yaml
server:
  host: 127.0.0.1
  port: 8765
  auth: none
```

**Behavior:**
- No authentication required
- Only accepts connections from localhost
- Binds to `127.0.0.1` (not `0.0.0.0`)
- Fast and simple for personal use

**Security:** Localhost-only binding prevents remote access

### Cloud Mode (API Key)

**Scenario:** codetect hosted as a service (future paid tier)

**Configuration:**
```yaml
server:
  host: 0.0.0.0
  port: 8765
  auth: api_key
  rate_limit:
    requests_per_minute: 60
```

**Authentication:**
- **Header:** `Authorization: Bearer <api-key>`
- **Key format:** `ctk_<random>` (e.g., `ctk_a1b2c3d4e5f6`)
- **Key generation:** `codetect api-key create --name "My App"`
- **Key storage:** SQLite table `api_keys` (hashed with bcrypt)

**Example Request:**
```bash
curl -H "Authorization: Bearer ctk_a1b2c3d4e5f6" \
     -H "Content-Type: application/json" \
     -d '{"query": "semantic search", "limit": 10}' \
     https://codetect.example.com/api/v1/search/semantic
```

**Key Management:**
```bash
# Create API key
codetect api-key create --name "CI/CD Pipeline"
# Output: ctk_xyz123... (save this, it won't be shown again)

# List API keys
codetect api-key list
# Output: ID | Name            | Created    | Last Used
#         1  | CI/CD Pipeline  | 2026-02-03 | 2026-02-03

# Revoke API key
codetect api-key revoke <key-id>
```

### Rate Limiting

**Algorithm:** Token bucket (standard rate limiting)

**Default Limits:**

| Plan | Requests/min | Burst | Tier |
|------|--------------|-------|------|
| Local | Unlimited | N/A | Free |
| Cloud Free | 60 | 10 | Free |
| Cloud Pro | 300 | 50 | $10/mo |
| Cloud Enterprise | Custom | Custom | Custom |

**Rate Limit Headers (RFC 6585):**
```
RateLimit-Limit: 60
RateLimit-Remaining: 45
RateLimit-Reset: 1736345678
```

**429 Response:**
```json
{
  "error": {
    "type": "rate_limit_exceeded",
    "title": "Rate Limit Exceeded",
    "status": 429,
    "detail": "You have exceeded 60 requests per minute. Try again in 30 seconds.",
    "retry_after": 30
  }
}
```

---

## 3. API Endpoints

### Endpoint Summary

| Endpoint | Method | MCP Tool | Description |
|----------|--------|----------|-------------|
| `/api/v1/search/keyword` | POST | `search_keyword` | Fast regex search via ripgrep |
| `/api/v1/search/semantic` | POST | `search_semantic` | Semantic search via embeddings |
| `/api/v1/search/hybrid` | POST | `hybrid_search_v2` | Hybrid search with RRF fusion |
| `/api/v1/files/{path}` | GET | `get_file` | Read file with line-range slicing |
| `/api/v1/symbols/find` | POST | `find_symbol` | Find symbol definitions |
| `/api/v1/symbols/list` | POST | `list_defs_in_file` | List symbols in a file |
| `/api/v1/projects` | GET | - | List indexed projects (registry) |
| `/api/v1/projects/{id}/status` | GET | - | Project indexing status |
| `/api/v1/health` | GET | - | Health check |
| `/api/v1/version` | GET | - | API version info |

---

### 3.1 Search: Keyword

**Endpoint:** `POST /api/v1/search/keyword`

**MCP Tool:** `search_keyword`

**Description:** Fast regex search using ripgrep

**Request:**
```json
{
  "query": "function\\s+\\w+",
  "path": "src/",
  "type": "go",
  "output_mode": "content",
  "context": 2,
  "limit": 20
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | ✅ | Regex pattern to search for |
| `path` | string | ❌ | Limit search to directory (default: all) |
| `type` | string | ❌ | File type (e.g., "go", "js", "py") |
| `output_mode` | string | ❌ | "content", "files_with_matches", "count" (default: "files_with_matches") |
| `context` | integer | ❌ | Lines of context around matches (default: 0) |
| `limit` | integer | ❌ | Max results to return (default: 100) |

**Response (200 OK):**
```json
{
  "results": [
    {
      "file": "internal/search/semantic.go",
      "line": 45,
      "column": 1,
      "match": "function Search(query string) ([]Result, error) {",
      "context_before": ["", "// Search performs semantic search"],
      "context_after": ["\treturn nil, nil", "}"]
    }
  ],
  "total": 42,
  "truncated": false,
  "duration_ms": 23
}
```

**Error Responses:**

| Status | Type | Description |
|--------|------|-------------|
| 400 | `invalid_query` | Invalid regex pattern |
| 500 | `search_error` | ripgrep execution failed |

---

### 3.2 Search: Semantic

**Endpoint:** `POST /api/v1/search/semantic`

**MCP Tool:** `search_semantic`

**Description:** Semantic search using embeddings

**Request:**
```json
{
  "query": "How does authentication work?",
  "limit": 10,
  "min_score": 0.7
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | ✅ | Natural language search query |
| `limit` | integer | ❌ | Max results (default: 10, max: 50) |
| `min_score` | float | ❌ | Minimum similarity score 0-1 (default: 0.0) |

**Response (200 OK):**
```json
{
  "results": [
    {
      "chunk": "Authentication is implemented using JWT tokens...",
      "file": "internal/auth/jwt.go",
      "start_line": 23,
      "end_line": 45,
      "score": 0.89
    },
    {
      "chunk": "OAuth2 flow begins when the user clicks 'Login'...",
      "file": "internal/auth/oauth.go",
      "start_line": 12,
      "end_line": 34,
      "score": 0.82
    }
  ],
  "total": 10,
  "duration_ms": 87
}
```

**Error Responses:**

| Status | Type | Description |
|--------|------|-------------|
| 400 | `invalid_limit` | Limit exceeds maximum (50) |
| 503 | `embeddings_unavailable` | Embedding service not available |

---

### 3.3 Search: Hybrid

**Endpoint:** `POST /api/v1/search/hybrid`

**MCP Tool:** `hybrid_search_v2`

**Description:** Hybrid search with RRF fusion (keyword + semantic)

**Request:**
```json
{
  "query": "authentication middleware",
  "limit": 20,
  "rerank": true
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | ✅ | Search query (used for both keyword and semantic) |
| `limit` | integer | ❌ | Max results (default: 20, max: 50) |
| `rerank` | boolean | ❌ | Enable cross-encoder reranking (default: false) |

**Response (200 OK):**
```json
{
  "results": [
    {
      "chunk": "AuthMiddleware validates JWT tokens...",
      "file": "internal/middleware/auth.go",
      "start_line": 15,
      "end_line": 42,
      "score": 0.94,
      "sources": ["semantic", "keyword"],
      "reranked": true
    }
  ],
  "total": 20,
  "semantic_results": 15,
  "keyword_results": 12,
  "fused_results": 18,
  "duration_ms": 156
}
```

**Error Responses:**

| Status | Type | Description |
|--------|------|-------------|
| 503 | `reranker_unavailable` | Reranking requested but service unavailable |

---

### 3.4 Files: Get File

**Endpoint:** `GET /api/v1/files/{path}`

**MCP Tool:** `get_file`

**Description:** Read file contents with optional line-range slicing

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | URL-encoded file path (e.g., `src%2Fmain.go`) |

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `start_line` | integer | First line to read (1-indexed, inclusive) |
| `end_line` | integer | Last line to read (1-indexed, inclusive) |

**Example Request:**
```
GET /api/v1/files/src%2Fmain.go?start_line=10&end_line=20
```

**Response (200 OK):**
```json
{
  "path": "src/main.go",
  "content": "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Hello\")\n}",
  "lines": [
    "package main",
    "",
    "import \"fmt\"",
    "",
    "func main() {",
    "\tfmt.Println(\"Hello\")",
    "}"
  ],
  "start_line": 10,
  "end_line": 20,
  "total_lines": 150,
  "truncated": false
}
```

**Error Responses:**

| Status | Type | Description |
|--------|------|-------------|
| 404 | `file_not_found` | File does not exist |
| 403 | `file_forbidden` | File outside indexed paths |

---

### 3.5 Symbols: Find Symbol

**Endpoint:** `POST /api/v1/symbols/find`

**MCP Tool:** `find_symbol`

**Description:** Find symbol definitions (functions, types, classes)

**Request:**
```json
{
  "name": "NewServer",
  "kind": "function",
  "limit": 10
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | ✅ | Symbol name to search for (supports partial matching) |
| `kind` | string | ❌ | Symbol kind filter: "function", "type", "class", etc. |
| `limit` | integer | ❌ | Max results (default: 50, max: 100) |

**Response (200 OK):**
```json
{
  "results": [
    {
      "name": "NewServer",
      "kind": "function",
      "file": "internal/server/server.go",
      "line": 23,
      "signature": "func NewServer(config Config) (*Server, error)",
      "language": "Go"
    }
  ],
  "total": 1,
  "duration_ms": 12
}
```

---

### 3.6 Symbols: List Definitions

**Endpoint:** `POST /api/v1/symbols/list`

**MCP Tool:** `list_defs_in_file`

**Description:** List all symbol definitions in a specific file

**Request:**
```json
{
  "path": "internal/server/server.go"
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | ✅ | File path to list symbols from |

**Response (200 OK):**
```json
{
  "file": "internal/server/server.go",
  "symbols": [
    {
      "name": "Server",
      "kind": "struct",
      "line": 15,
      "signature": "type Server struct { ... }"
    },
    {
      "name": "NewServer",
      "kind": "function",
      "line": 23,
      "signature": "func NewServer(config Config) (*Server, error)"
    },
    {
      "name": "Start",
      "kind": "method",
      "line": 45,
      "signature": "func (s *Server) Start() error"
    }
  ],
  "total": 12,
  "duration_ms": 8
}
```

---

### 3.7 Projects: List

**Endpoint:** `GET /api/v1/projects`

**Description:** List all indexed projects from registry

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `status` | string | Filter by status: "indexed", "indexing", "error" |

**Example Request:**
```
GET /api/v1/projects?status=indexed
```

**Response (200 OK):**
```json
{
  "projects": [
    {
      "id": "codetect-abc123",
      "name": "codetect",
      "path": "/Users/brian/dev/codetect",
      "status": "indexed",
      "last_indexed": "2026-02-03T05:30:00Z",
      "file_count": 82,
      "chunk_count": 1248,
      "embedding_count": 1248,
      "db_size_mb": 45.3
    }
  ],
  "total": 1
}
```

---

### 3.8 Projects: Status

**Endpoint:** `GET /api/v1/projects/{id}/status`

**Description:** Get detailed status of a specific project

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | string | Project ID from registry |

**Example Request:**
```
GET /api/v1/projects/codetect-abc123/status
```

**Response (200 OK):**
```json
{
  "id": "codetect-abc123",
  "name": "codetect",
  "path": "/Users/brian/dev/codetect",
  "status": "indexed",
  "last_indexed": "2026-02-03T05:30:00Z",
  "indexing": {
    "files_processed": 82,
    "chunks_created": 1248,
    "embeddings_generated": 1248,
    "duration_seconds": 120
  },
  "database": {
    "type": "sqlite",
    "path": ".codetect/index.db",
    "size_mb": 45.3
  },
  "health": "healthy"
}
```

---

### 3.9 Health Check

**Endpoint:** `GET /api/v1/health`

**Description:** Health check for monitoring and load balancers

**Response (200 OK):**
```json
{
  "status": "healthy",
  "version": "2.0.2",
  "uptime_seconds": 12345,
  "checks": {
    "database": "ok",
    "embeddings": "ok",
    "registry": "ok"
  }
}
```

**Response (503 Service Unavailable):**
```json
{
  "status": "unhealthy",
  "version": "2.0.2",
  "uptime_seconds": 12345,
  "checks": {
    "database": "ok",
    "embeddings": "error: ollama not running",
    "registry": "ok"
  }
}
```

---

### 3.10 Version Info

**Endpoint:** `GET /api/v1/version`

**Description:** Get API and codetect version information

**Response (200 OK):**
```json
{
  "api_version": "v1",
  "codetect_version": "2.0.2",
  "git_commit": "a1b2c3d",
  "build_date": "2026-02-01T12:00:00Z",
  "go_version": "go1.25.1"
}
```

---

## 4. Error Handling

### RFC 7807 Problem Details

All errors follow [RFC 7807](https://tools.ietf.org/html/rfc7807) Problem Details format:

```json
{
  "error": {
    "type": "https://codetect.dev/errors/invalid_query",
    "title": "Invalid Query",
    "status": 400,
    "detail": "Regex pattern is invalid: missing closing bracket",
    "instance": "/api/v1/search/keyword",
    "request_id": "req_abc123"
  }
}
```

### Standard Error Types

| Type | Status | Description |
|------|--------|-------------|
| `invalid_request` | 400 | Malformed request (invalid JSON, missing required fields) |
| `invalid_query` | 400 | Invalid search query (regex syntax error, etc.) |
| `not_found` | 404 | Resource not found (file, project, etc.) |
| `unauthorized` | 401 | Authentication required but not provided |
| `forbidden` | 403 | Authenticated but not authorized for resource |
| `rate_limit_exceeded` | 429 | Too many requests |
| `search_error` | 500 | Internal search failure |
| `service_unavailable` | 503 | Dependency unavailable (Ollama, database, etc.) |

---

## 5. OpenAPI 3.0 Specification

**File:** `docs/openapi.yaml`

**Generation:** `codetect api spec > openapi.yaml`

**Purpose:**
- Automatic client generation (Go, Python, TypeScript, etc.)
- API documentation (Swagger UI, Redoc)
- Request validation
- Mock server generation

**Example (abbreviated):**

```yaml
openapi: 3.0.0
info:
  title: codetect API
  version: 1.0.0
  description: RESTful API for code search and retrieval
  contact:
    name: codetect Support
    url: https://github.com/brian-lai/codetect
servers:
  - url: http://localhost:8765/api/v1
    description: Local development
  - url: https://api.codetect.dev/api/v1
    description: Production (cloud tier)

paths:
  /search/keyword:
    post:
      summary: Keyword search
      operationId: searchKeyword
      tags: [Search]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/KeywordSearchRequest'
      responses:
        '200':
          description: Search results
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/KeywordSearchResponse'
        '400':
          $ref: '#/components/responses/BadRequest'
        '500':
          $ref: '#/components/responses/InternalError'

components:
  schemas:
    KeywordSearchRequest:
      type: object
      required: [query]
      properties:
        query:
          type: string
          example: "function\\s+\\w+"
        path:
          type: string
          example: "src/"
        type:
          type: string
          enum: [go, js, py, java, rust]
        limit:
          type: integer
          minimum: 1
          maximum: 100
          default: 20
    # ... more schemas ...

  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
      bearerFormat: API Key

security:
  - BearerAuth: []
```

---

## 6. Implementation Architecture

### HTTP Server Stack

**Framework:** net/http (standard library) + Chi router

**Why Chi?**
- Lightweight (no dependencies)
- Context-aware (works well with Go contexts)
- Middleware-friendly
- Compatible with stdlib

**Alternative considered:** Gin (rejected for extra dependencies)

### Layer Architecture

```
┌─────────────────────────────────────┐
│  HTTP Layer (Chi router)            │
│  - Request parsing                  │
│  - Response serialization           │
│  - Middleware (auth, logging)       │
└────────────┬────────────────────────┘
             │
             ↓
┌─────────────────────────────────────┐
│  Service Layer (Business logic)     │
│  - Search orchestration             │
│  - Registry management              │
└────────────┬────────────────────────┘
             │
             ↓
┌─────────────────────────────────────┐
│  MCP Adapter (Wraps MCP tools)      │
│  - MCP stdio communication          │
│  - JSON-RPC translation             │
└────────────┬────────────────────────┘
             │
             ↓
┌─────────────────────────────────────┐
│  MCP Server (Existing codetect)     │
│  - search_keyword, semantic, etc.   │
└─────────────────────────────────────┘
```

### Directory Structure

```
cmd/
├── codetect-api/      # New HTTP API server
│   └── main.go

internal/
├── api/               # New HTTP API package
│   ├── server.go      # HTTP server setup
│   ├── router.go      # Chi router configuration
│   ├── middleware.go  # Auth, rate limit, logging
│   ├── handlers/      # HTTP handlers
│   │   ├── search.go
│   │   ├── files.go
│   │   ├── symbols.go
│   │   └── projects.go
│   └── types.go       # Request/response types
├── auth/              # New auth package
│   ├── apikey.go      # API key management
│   └── ratelimit.go   # Rate limiting
└── mcp/               # Existing MCP package (reuse)
    └── client.go      # MCP stdio client
```

---

## 7. Deployment

### Local Deployment

```bash
# Start HTTP API server
codetect serve --port 8765

# In another terminal, test it
curl http://localhost:8765/api/v1/health
```

### Cloud Deployment (Future)

**Docker:**
```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o codetect-api cmd/codetect-api/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/codetect-api /usr/local/bin/
EXPOSE 8765
CMD ["codetect-api", "serve", "--host", "0.0.0.0", "--port", "8765"]
```

**Kubernetes:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: codetect-api
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: codetect-api
        image: codetect:2.0.2
        ports:
        - containerPort: 8765
        env:
        - name: CODETECT_AUTH
          value: "api_key"
        - name: CODETECT_RATE_LIMIT
          value: "60"
        livenessProbe:
          httpGet:
            path: /api/v1/health
            port: 8765
          initialDelaySeconds: 5
          periodSeconds: 10
```

---

## 8. Integration Examples

### cURL

```bash
# Keyword search
curl -X POST http://localhost:8765/api/v1/search/keyword \
  -H "Content-Type: application/json" \
  -d '{"query": "func main", "type": "go", "limit": 5}'

# Semantic search
curl -X POST http://localhost:8765/api/v1/search/semantic \
  -H "Content-Type: application/json" \
  -d '{"query": "How does authentication work?", "limit": 10}'

# Get file
curl http://localhost:8765/api/v1/files/src%2Fmain.go?start_line=1&end_line=20
```

### Python Client

```python
import requests

class CodetectClient:
    def __init__(self, base_url="http://localhost:8765", api_key=None):
        self.base_url = base_url
        self.headers = {"Content-Type": "application/json"}
        if api_key:
            self.headers["Authorization"] = f"Bearer {api_key}"

    def search_semantic(self, query, limit=10):
        resp = requests.post(
            f"{self.base_url}/api/v1/search/semantic",
            headers=self.headers,
            json={"query": query, "limit": limit}
        )
        resp.raise_for_status()
        return resp.json()

# Usage
client = CodetectClient()
results = client.search_semantic("authentication middleware")
for result in results['results']:
    print(f"{result['file']}:{result['start_line']} - {result['score']:.2f}")
```

### TypeScript Client (Auto-generated)

```bash
# Generate TypeScript client from OpenAPI spec
npx openapi-typescript-codegen --input openapi.yaml --output ./src/api
```

```typescript
import { CodetectClient } from './api';

const client = new CodetectClient({
  BASE: 'http://localhost:8765/api/v1',
  TOKEN: process.env.CODETECT_API_KEY,
});

const results = await client.search.searchSemantic({
  query: 'authentication middleware',
  limit: 10,
});

console.log(results.results);
```

### VS Code Extension

**Use Case:** Inline search in VS Code sidebar

```typescript
// extension.ts
import * as vscode from 'vscode';
import fetch from 'node-fetch';

export function activate(context: vscode.ExtensionContext) {
  let disposable = vscode.commands.registerCommand(
    'codetect.searchSemantic',
    async () => {
      const query = await vscode.window.showInputBox({
        prompt: 'Enter search query',
      });

      const response = await fetch('http://localhost:8765/api/v1/search/semantic', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ query, limit: 10 }),
      });

      const data = await response.json();

      // Show results in Quick Pick
      const items = data.results.map((r: any) => ({
        label: `${r.file}:${r.start_line}`,
        description: `Score: ${r.score.toFixed(2)}`,
        detail: r.chunk,
      }));

      const selected = await vscode.window.showQuickPick(items);
      if (selected) {
        // Open file at line
        const uri = vscode.Uri.file(selected.label.split(':')[0]);
        const doc = await vscode.workspace.openTextDocument(uri);
        await vscode.window.showTextDocument(doc);
      }
    }
  );

  context.subscriptions.push(disposable);
}
```

---

## 9. Testing Strategy

### Unit Tests

**Test HTTP handlers in isolation:**

```go
func TestSearchSemanticHandler(t *testing.T) {
    // Create mock MCP client
    mockMCP := &MockMCPClient{
        SearchSemanticFunc: func(query string, limit int) ([]Result, error) {
            return []Result{{Chunk: "test", Score: 0.9}}, nil
        },
    }

    // Create handler
    handler := NewSearchHandler(mockMCP)

    // Test request
    req := httptest.NewRequest("POST", "/api/v1/search/semantic", strings.NewReader(`{"query":"test"}`))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()

    handler.ServeHTTP(rec, req)

    assert.Equal(t, 200, rec.Code)
    // ... assert response body
}
```

### Integration Tests

**Test full HTTP API → MCP flow:**

```bash
# Start test server
go run cmd/codetect-api/main.go serve --port 9999 &
API_PID=$!

# Run tests
curl -s http://localhost:9999/api/v1/health | jq .status
# Output: "healthy"

curl -X POST http://localhost:9999/api/v1/search/semantic \
  -H "Content-Type: application/json" \
  -d '{"query":"test","limit":1}' | jq .total
# Output: 1

# Cleanup
kill $API_PID
```

### End-to-End Tests

**Test with real MCP server:**

```bash
# Start MCP server
codetect serve --mcp &
MCP_PID=$!

# Start HTTP API
codetect serve --http --port 8765 &
HTTP_PID=$!

# Run e2e tests
pytest tests/e2e/test_api.py

# Cleanup
kill $HTTP_PID $MCP_PID
```

---

## 10. Success Criteria

**Phase 1e is complete when:**

- ✅ All 6 MCP tools exposed via HTTP endpoints
- ✅ OpenAPI 3.0 spec generated and documented
- ✅ Local mode (no auth) works out-of-the-box
- ✅ API key authentication implemented (for future cloud tier)
- ✅ Rate limiting infrastructure in place
- ✅ At least one example integration (Python client or VS Code extension)
- ✅ Integration tests pass with 90%+ coverage
- ✅ Documentation complete (README, API reference)

**Optional (defer to Phase 2):**
- WebSocket support for streaming results
- GraphQL endpoint (alternative to REST)
- gRPC support for high-performance clients

---

## Conclusion

This HTTP API design wraps codetect's MCP tools in a RESTful interface, enabling integration with non-MCP tools and services. The design prioritizes simplicity (JSON, REST), flexibility (local and cloud modes), and ecosystem growth (OpenAPI spec for client generation).

**Next Steps:**
1. Review this design with stakeholders
2. Create OpenAPI spec (generate from this doc)
3. Implement Phase 1e (3-4 weeks)
4. Build example VS Code extension

**Timeline:** Phase 1e implementation in 3-4 weeks after Phase 1a, 1c, 1d complete.
