# .codetectignore Specification

**Date:** 2026-02-03
**Purpose:** Define .codetectignore file format and behavior for Phase 1d
**Designer:** Claude Code

---

## Executive Summary

`.codetectignore` is a purpose-built exclusion file for codetect indexing, using .gitignore-compatible syntax. It allows users to exclude files/directories from indexing and embedding without modifying `.gitignore`, addressing use cases where gitignored files should be indexed (e.g., `vendor/` in some projects) or where indexed files should be excluded (e.g., generated code, test fixtures).

**Key Features:**
- ✅ .gitignore-compatible syntax (no learning curve)
- ✅ Independent of .gitignore (can include gitignored files, exclude tracked files)
- ✅ Hierarchical (repo root + subdirectories + global `~/.codetectignore`)
- ✅ Applies to indexing + embedding (both stages)
- ✅ Fast pattern matching (compiled once, reused)

**Timeline:** Phase 1d implementation (1 week)

---

## 1. File Format

### Syntax

`.codetectignore` uses [.gitignore syntax](https://git-scm.com/docs/gitignore) with no extensions.

**Pattern Types:**

| Pattern | Matches | Example |
|---------|---------|---------|
| `filename` | Filename in any directory | `*.min.js` → matches `dist/app.min.js` |
| `dir/` | Directory (trailing slash) | `vendor/` → matches all `vendor/*` files |
| `path/to/file` | Specific path | `src/generated.go` → only this file |
| `*.ext` | File extension wildcard | `*.log` → matches all `.log` files |
| `!pattern` | Negation (explicitly include) | `!vendor/important/` → include this dir |
| `#comment` | Comment line (ignored) | `# Exclude test fixtures` |
| ` ` (blank) | Empty line (ignored) | ` ` |

**Glob Syntax Supported:**

- `*` - Matches any characters except `/`
- `**` - Matches any characters including `/` (e.g., `**/generated/*`)
- `?` - Matches single character
- `[abc]` - Matches one of listed characters
- `[a-z]` - Matches character range

**NOT Supported (gitignore-specific):**

- `\` escape sequences (use literal characters)
- Advanced `**` behavior differences (keep it simple)

### Example File

```gitignore
# .codetectignore - Exclude patterns from codetect indexing

# Comments start with #
# Blank lines are ignored

# Generated code
*.generated.ts
*.generated.go
*_pb.ts
*_pb.go
schema.graphql.ts

# Minified/compiled files
*.min.js
*.min.css
*.bundle.js
*.map

# Build artifacts
dist/
build/
out/
target/

# Framework-specific cache
.next/
.nuxt/
.vuepress/
.docusaurus/

# Vendor directories (usually tracked in Git)
vendor/
node_modules/
third_party/

# Test fixtures (data files used in tests)
fixtures/
__snapshots__/
testdata/

# Documentation that doesn't help code search
docs/diagrams/
*.excalidraw
*.drawio

# Large data files
*.csv
*.json
*.xml
*.yaml
*.yml

# Include exceptions (! prefix negates)
!config.yaml
!package.json
!vendor/critical-lib/

# Exclude specific deep paths
**/migrations/*.sql
**/proto/*.proto
```

---

## 2. Behavior Specification

### 2.1 When Exclusions Apply

**.codetectignore patterns apply to:**
- ✅ **Indexing** (file scanning, chunking, Merkle tree)
- ✅ **Embedding** (generating vector embeddings)
- ✅ **MCP tool queries** (excluded files never appear in results)

**.codetectignore patterns do NOT apply to:**
- ❌ **Git operations** (independent of Git)
- ❌ **File watching** (daemon still watches for changes, just skips indexing)

**Key Insight:** Excluded files are **completely invisible** to codetect search. They're not indexed, not embedded, and won't appear in any results.

### 2.2 File Discovery & Hierarchy

**Search Order (most specific to most general):**

1. **`.codetectignore` in repo root** (highest priority)
2. **`.codetectignore` in parent directories** (up to repo root)
3. **`~/.codetectignore` (global)** (applies to all projects)

**Merge Strategy:** Patterns from all levels are combined (OR logic). A file is excluded if ANY pattern matches.

**Example Hierarchy:**

```
~/
├── .codetectignore        # Global: exclude *.log everywhere
└── dev/
    └── myproject/
        ├── .codetectignore # Project: exclude dist/, *.min.js
        └── src/
            └── generated/
                └── api.ts  # Excluded by project .codetectignore
```

### 2.3 Relationship with .gitignore

**Independence:** `.codetectignore` is **independent** of `.gitignore`.

**Four Scenarios:**

| File Status | .gitignore | .codetectignore | Indexed? | Use Case |
|-------------|------------|-----------------|----------|----------|
| Tracked | No | No | ✅ Yes | Normal code files |
| Tracked | No | Yes | ❌ No | Exclude tracked generated code |
| Ignored | Yes | No | ✅ Yes | Include `vendor/` if needed |
| Ignored | Yes | Yes | ❌ No | Exclude `node_modules/` (default) |

**Example 1: Include gitignored vendor directory**

```gitignore
# .gitignore
vendor/

# .codetectignore
# (empty - vendor/ will be indexed)
```

**Result:** `vendor/` files are indexed by codetect despite being gitignored.

**Example 2: Exclude tracked generated code**

```gitignore
# .gitignore
# (empty - generated.go is tracked)

# .codetectignore
*.generated.go
```

**Result:** `*.generated.go` files are NOT indexed despite being tracked in Git.

### 2.4 Precedence Rules

**Order of evaluation:**

1. **Read all .codetectignore files** (project + global)
2. **Compile patterns** into matcher
3. **For each file during scan:**
   - Check if file path matches ANY pattern
   - If match: EXCLUDE
   - If negation match (`!pattern`): INCLUDE (override previous exclusion)

**Negation Example:**

```gitignore
# Exclude all vendor/
vendor/

# But include this specific library
!vendor/critical-lib/
```

**Result:**
- `vendor/foo/` → Excluded
- `vendor/critical-lib/` → Included (negation overrides)
- `vendor/critical-lib/bar.go` → Included

### 2.5 Pattern Matching Rules

**Absolute vs Relative Paths:**

- Patterns are matched against **relative paths** from repo root
- Example: Pattern `dist/` matches `./dist/` and `./src/dist/`
- To match only root-level: `/dist/` (leading slash)

**Directory Matching:**

- Pattern with trailing `/` matches only directories
- Pattern without `/` matches files or directories
- Example: `test/` matches directory, `test` matches file or directory

**Wildcard Behavior:**

- `*` does NOT match `/` (single-level wildcard)
- `**` DOES match `/` (multi-level wildcard)
- Example: `*.js` matches `app.js` but NOT `src/app.js`
- Example: `**/*.js` matches `app.js` AND `src/app.js`

---

## 3. Implementation Strategy

### 3.1 Library Choice

**Use:** [github.com/sabhiram/go-gitignore](https://github.com/sabhiram/go-gitignore)

**Why?**
- ✅ Mature (5+ years old, 1k+ stars)
- ✅ .gitignore-compatible syntax
- ✅ Fast (compiled patterns)
- ✅ Well-tested
- ✅ MIT license

**Alternative considered:** Custom parser (rejected for complexity)

### 3.2 Integration Points

**File scanning (indexing):**

```go
// internal/indexer/indexer.go

type IndexOptions struct {
    Force    bool
    Verbose  bool
    Progress ProgressCallback
    Ignore   *ignore.GitIgnore  // NEW: .codetectignore patterns
}

func (idx *Indexer) scanFiles(ctx context.Context, opts IndexOptions) ([]string, error) {
    var files []string

    err := filepath.WalkDir(idx.repoRoot, func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            return err
        }

        // Get relative path for pattern matching
        relPath, _ := filepath.Rel(idx.repoRoot, path)

        // Check .codetectignore patterns
        if opts.Ignore != nil && opts.Ignore.MatchesPath(relPath) {
            if d.IsDir() {
                return filepath.SkipDir  // Skip entire directory
            }
            return nil  // Skip file
        }

        // ... rest of scan logic ...
    })

    return files, err
}
```

**Pattern loading:**

```go
// internal/indexer/ignore.go

func LoadCodetectIgnore(repoRoot string) (*ignore.GitIgnore, error) {
    // Check for project .codetectignore
    projectIgnoreFile := filepath.Join(repoRoot, ".codetectignore")
    if _, err := os.Stat(projectIgnoreFile); err == nil {
        return ignore.CompileIgnoreFile(projectIgnoreFile)
    }

    // Check for global ~/.codetectignore
    homeDir, _ := os.UserHomeDir()
    globalIgnoreFile := filepath.Join(homeDir, ".codetectignore")
    if _, err := os.Stat(globalIgnoreFile); err == nil {
        return ignore.CompileIgnoreFile(globalIgnoreFile)
    }

    // No .codetectignore found, return nil (no exclusions)
    return nil, nil
}

func LoadCodetectIgnoreHierarchy(repoRoot string) (*ignore.GitIgnore, error) {
    var patterns []string

    // 1. Load global ~/.codetectignore
    homeDir, _ := os.UserHomeDir()
    globalFile := filepath.Join(homeDir, ".codetectignore")
    if content, err := os.ReadFile(globalFile); err == nil {
        patterns = append(patterns, parseIgnoreLines(string(content))...)
    }

    // 2. Load project .codetectignore
    projectFile := filepath.Join(repoRoot, ".codetectignore")
    if content, err := os.ReadFile(projectFile); err == nil {
        patterns = append(patterns, parseIgnoreLines(string(content))...)
    }

    // Compile all patterns together
    return ignore.CompileIgnoreLines(patterns...), nil
}
```

### 3.3 CLI Integration

**New flag:**

```bash
codetect index --ignore-file /path/to/.codetectignore
```

**Default behavior:**

```bash
# Automatically loads .codetectignore from repo root (if exists)
codetect index .

# Explicitly disable .codetectignore
codetect index --no-ignore .

# Use custom ignore file
codetect index --ignore-file custom-ignore.txt .
```

### 3.4 Configuration

**Add to `.codetect.yaml`:**

```yaml
indexing:
  ignore_file: .codetectignore  # Default
  use_global_ignore: true        # Load ~/.codetectignore
  respect_gitignore: false       # Independent of .gitignore
```

---

## 4. Common Use Cases

### 4.1 Exclude Generated Code

**Problem:** Generated code clutters search results

**.codetectignore:**
```gitignore
*.generated.ts
*.generated.go
*_pb.ts
*_pb.go
schema.graphql.ts
```

**Result:** Generated files never appear in search, improving signal-to-noise.

### 4.2 Exclude Minified/Bundled Code

**Problem:** Minified code is unreadable and unhelpful

**.codetectignore:**
```gitignore
*.min.js
*.min.css
*.bundle.js
*.map
dist/
build/
```

**Result:** Only source code is indexed, not compiled artifacts.

### 4.3 Exclude Test Fixtures

**Problem:** Test data files (JSON, CSV, etc.) aren't code

**.codetectignore:**
```gitignore
fixtures/
__snapshots__/
testdata/
*.fixture.json
```

**Result:** Focus on actual test code, not test data.

### 4.4 Exclude Vendor with Exceptions

**Problem:** Vendor code clutters results, but some vendor code is relevant

**.codetectignore:**
```gitignore
# Exclude all vendor/
vendor/

# Include specific critical libraries
!vendor/our-custom-lib/
!vendor/important-dependency/
```

**Result:** Most vendor code excluded, but critical libraries indexed.

### 4.5 Exclude Large Data Files

**Problem:** Large JSON/CSV/YAML files slow indexing

**.codetectignore:**
```gitignore
*.csv
*.json
*.xml

# Include config files
!config.json
!package.json
!tsconfig.json
```

**Result:** Data files excluded, but configuration files included.

---

## 5. Testing Strategy

### 5.1 Unit Tests

**Test pattern matching:**

```go
func TestCodetectIgnore(t *testing.T) {
    tests := []struct {
        name     string
        patterns []string
        path     string
        excluded bool
    }{
        {
            name:     "Exclude *.min.js",
            patterns: []string{"*.min.js"},
            path:     "dist/app.min.js",
            excluded: true,
        },
        {
            name:     "Include negated vendor",
            patterns: []string{"vendor/", "!vendor/important/"},
            path:     "vendor/important/lib.go",
            excluded: false,
        },
        {
            name:     "Exclude directory",
            patterns: []string{"dist/"},
            path:     "dist/app.js",
            excluded: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ignore := ignore.CompileIgnoreLines(tt.patterns...)
            assert.Equal(t, tt.excluded, ignore.MatchesPath(tt.path))
        })
    }
}
```

### 5.2 Integration Tests

**Test full indexing flow:**

```bash
# Create test repo
mkdir -p /tmp/test-ignore
cd /tmp/test-ignore
git init

# Create .codetectignore
cat > .codetectignore <<EOF
*.generated.go
dist/
EOF

# Create files
echo "package main" > main.go
echo "package generated" > generated.generated.go
mkdir dist
echo "console.log('minified')" > dist/app.min.js

# Index
codetect index .

# Verify: main.go indexed, generated.generated.go excluded
codetect search keyword "package" | grep main.go
codetect search keyword "package" | grep -q generated.generated.go && exit 1 || echo "OK: generated.go excluded"
```

### 5.3 Edge Cases

**Test edge cases:**

1. **Empty .codetectignore** → All files indexed
2. **No .codetectignore** → All files indexed
3. **Global ~/.codetectignore only** → Patterns applied to all projects
4. **Conflicting patterns** → Most specific wins
5. **Negation order** → Later negations override earlier exclusions
6. **Directory vs file** → `/dist` (root only) vs `dist` (anywhere)

---

## 6. Documentation

### 6.1 README.md Section

**Add to main README:**

```markdown
## Excluding Files from Indexing

Create a `.codetectignore` file in your repo root to exclude files from indexing:

```gitignore
# Exclude generated code
*.generated.ts
*_pb.go

# Exclude minified files
*.min.js
dist/

# Exclude test fixtures
fixtures/
```

Syntax is .gitignore-compatible. See [.codetectignore documentation](docs/codetectignore.md).
```

### 6.2 New docs/codetectignore.md

**Create comprehensive guide:**

```markdown
# .codetectignore Guide

## What is .codetectignore?

`.codetectignore` is a file that tells codetect which files to exclude from indexing and embedding.

## Syntax

Uses .gitignore syntax:

- `*.ext` - Exclude files by extension
- `dir/` - Exclude directory
- `!pattern` - Include exception
- `#comment` - Comment line

## Examples

[Include all use cases from section 4]

## FAQ

**Q: How is .codetectignore different from .gitignore?**
A: Independent. You can exclude tracked files or include gitignored files.

**Q: Where should I put .codetectignore?**
A: Repo root (`.codetectignore`) or home directory (`~/.codetectignore` for global).

**Q: Can I use multiple .codetectignore files?**
A: Yes, project + global patterns are combined.

[More FAQs...]
```

---

## 7. Success Criteria

**Phase 1d is complete when:**

- ✅ `.codetectignore` file format implemented (gitignore syntax)
- ✅ Patterns applied during indexing (file scanning)
- ✅ Patterns applied during embedding
- ✅ Hierarchical loading (project + global)
- ✅ Negation patterns work (`!vendor/important/`)
- ✅ CLI flag `--ignore-file` supported
- ✅ Documentation complete (README, docs/codetectignore.md)
- ✅ Unit + integration tests pass
- ✅ Common use cases validated (generated code, vendor, etc.)

---

## 8. Future Enhancements (Deferred)

**Not in Phase 1d scope:**

- **`.codetectignore` in subdirectories** - Only root-level for now
- **UI for managing exclusions** - CLI-only for Phase 1
- **Per-tool ignore** - Same patterns for indexing + embedding
- **Real-time reload** - Requires daemon restart after editing .codetectignore

These can be added in Phase 2 if user feedback requests them.

---

## 9. Risks & Mitigations

### Risk: Exclude too much by default

**Problem:** Users accidentally exclude important files

**Mitigation:**
- No default .codetectignore (explicit opt-in)
- Verbose mode shows excluded files
- Documentation has conservative examples

### Risk: Pattern matching performance

**Problem:** Checking patterns for every file is slow

**Mitigation:**
- Use compiled patterns (go-gitignore compiles once)
- Directory exclusions skip entire subtrees (fast)
- Benchmark with 10k+ file repos

### Risk: Confusing precedence (project vs global)

**Problem:** Users don't understand which patterns apply

**Mitigation:**
- Document merge strategy clearly
- `codetect index --dry-run` shows excluded files
- Verbose mode logs pattern source

---

## 10. Implementation Checklist

**Phase 1d tasks:**

- [ ] Add `github.com/sabhiram/go-gitignore` dependency
- [ ] Implement `LoadCodetectIgnore()` in `internal/indexer/ignore.go`
- [ ] Integrate with `scanFiles()` in `internal/indexer/indexer.go`
- [ ] Add `--ignore-file` and `--no-ignore` CLI flags
- [ ] Add `.codetectignore` support to config (`indexing.ignore_file`)
- [ ] Write unit tests for pattern matching
- [ ] Write integration tests for indexing flow
- [ ] Create `docs/codetectignore.md` guide
- [ ] Update README.md with .codetectignore section
- [ ] Test common use cases (generated code, vendor, fixtures)

---

## Conclusion

`.codetectignore` provides fine-grained control over what codetect indexes, using familiar .gitignore syntax. It's independent of Git, supports hierarchical patterns, and addresses common pain points (generated code, vendor bloat, test fixtures). Implementation is straightforward using an existing library (go-gitignore) and integrates cleanly with the existing file scanning logic.

**Next Steps:**
1. Review this specification
2. Implement Phase 1d (1 week)
3. Gather user feedback on common patterns
4. Document best practices for different project types
