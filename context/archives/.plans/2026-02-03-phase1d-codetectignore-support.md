# Phase 1d Implementation Plan: .codetectignore Support

**Date:** 2026-02-03
**Branch:** `para/phase1-implementation-phase1d`
**Objective:** Implement purpose-built exclusion file for indexing control
**Type:** Sub-Plan (Phase 1d of Phase 1 Implementation)

---

## Objective

Implement `.codetectignore` file support to give users fine-grained control over what files are indexed and embedded, independent of `.gitignore`.

**Success Criteria:**
- ✅ .codetectignore works with standard .gitignore patterns
- ✅ Users can exclude paths independently of .gitignore
- ✅ Hierarchical loading (project + global `~/.codetectignore`)
- ✅ Documentation is clear and includes examples
- ✅ No performance regression on large repos (10k+ files)

**Timeline:** 1 week

---

## Implementation Steps

### Step 1: Add Dependencies & Core Infrastructure

**Create:** `internal/indexer/ignore.go`

**Tasks:**
- [ ] Add `github.com/sabhiram/go-gitignore` dependency
- [ ] Create `LoadCodetectIgnore(repoRoot string)` function
- [ ] Create `LoadCodetectIgnoreHierarchy(repoRoot string)` function
- [ ] Support loading from project root `.codetectignore`
- [ ] Support loading from global `~/.codetectignore`
- [ ] Merge patterns from both files (OR logic)

**Deliverable:** Pattern loading infrastructure

---

### Step 2: Integrate with File Scanning

**Modify:** `internal/indexer/indexer.go`

**Tasks:**
- [ ] Add `Ignore *ignore.GitIgnore` field to `IndexOptions` struct
- [ ] Update `scanFiles()` to check ignore patterns
- [ ] Skip entire directories when pattern matches (performance optimization)
- [ ] Skip individual files when pattern matches
- [ ] Ensure relative paths are used for pattern matching

**Example Integration:**

```go
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

**Deliverable:** File scanning respects .codetectignore patterns

---

### Step 3: Update CLI Commands

**Modify:** `cmd/codetect-index/main.go`

**Tasks:**
- [ ] Add `--ignore-file <path>` flag (custom .codetectignore path)
- [ ] Add `--no-ignore` flag (disable .codetectignore entirely)
- [ ] Load .codetectignore by default if it exists
- [ ] Pass ignore patterns to indexer options
- [ ] Add verbose logging to show excluded files (if --verbose)

**CLI Examples:**

```bash
# Default: automatically loads .codetectignore from repo root
codetect-index index .

# Explicitly disable .codetectignore
codetect-index index --no-ignore .

# Use custom ignore file
codetect-index index --ignore-file custom-ignore.txt .

# Verbose mode shows excluded files
codetect-index index --verbose .
# Output: "Excluded: dist/app.min.js (matched pattern: *.min.js)"
```

**Deliverable:** CLI supports .codetectignore flags

---

### Step 4: Add Configuration Support

**Modify:** `internal/config/index.go` (or create new config file)

**Tasks:**
- [ ] Add `Indexing` section to config struct
- [ ] Add `ignore_file` field (default: `.codetectignore`)
- [ ] Add `use_global_ignore` field (default: `true`)
- [ ] Add `respect_gitignore` field (default: `false` - independent)
- [ ] Load from `.codetect.yaml` if exists

**YAML Config Example:**

```yaml
indexing:
  ignore_file: .codetectignore  # Path to ignore file
  use_global_ignore: true        # Load ~/.codetectignore
  respect_gitignore: false       # Independent of .gitignore
```

**Deliverable:** YAML configuration support

---

### Step 5: Testing

**Create:**
- `internal/indexer/ignore_test.go`
- `internal/indexer/integration_test.go`

**Unit Tests:**
- [ ] Test pattern matching (*.min.js, dist/, vendor/)
- [ ] Test negation patterns (!vendor/important/)
- [ ] Test directory vs file patterns (dist/ vs dist)
- [ ] Test wildcard behavior (`*` vs `**`)
- [ ] Test empty .codetectignore
- [ ] Test no .codetectignore file

**Integration Tests:**
- [ ] Create test repo with .codetectignore
- [ ] Add various file types (code, generated, minified)
- [ ] Run indexing
- [ ] Verify excluded files don't appear in index
- [ ] Verify included files do appear in index

**Edge Cases:**
- [ ] Conflicting patterns (later patterns win)
- [ ] Global + project .codetectignore merge
- [ ] Negation order (later negations override earlier exclusions)
- [ ] Root-level vs anywhere (`/dist` vs `dist`)

**Deliverable:** Comprehensive test coverage

---

### Step 6: Documentation

**Create:** `docs/codetectignore.md`

**Tasks:**
- [ ] Write comprehensive .codetectignore guide
- [ ] Document syntax (same as .gitignore)
- [ ] Include common use cases (5-6 examples)
- [ ] Document hierarchical loading
- [ ] Add FAQ section
- [ ] Explain relationship with .gitignore

**Update:** `README.md`

**Tasks:**
- [ ] Add "Excluding Files from Indexing" section
- [ ] Show quick example `.codetectignore`
- [ ] Link to full docs/codetectignore.md guide

**Update:** `docs/installation.md`

**Tasks:**
- [ ] Add .codetectignore setup instructions
- [ ] Recommend patterns for common project types (JS, Go, Python)

**Deliverable:** Complete documentation

---

### Step 7: Validate Common Use Cases

**Test Scenarios:**

1. **Exclude Generated Code**
   ```gitignore
   *.generated.ts
   *_pb.go
   ```

2. **Exclude Minified Files**
   ```gitignore
   *.min.js
   dist/
   ```

3. **Exclude Test Fixtures**
   ```gitignore
   fixtures/
   testdata/
   ```

4. **Exclude Vendor with Exceptions**
   ```gitignore
   vendor/
   !vendor/critical-lib/
   ```

5. **Exclude Large Data Files**
   ```gitignore
   *.csv
   !config.json
   ```

**Tasks:**
- [ ] Create test repos for each scenario
- [ ] Verify exclusions work correctly
- [ ] Measure indexing performance (should be faster with exclusions)
- [ ] Document best practices for each use case

**Deliverable:** Validated use cases + best practices

---

## Files to Create

| File | Purpose |
|------|---------|
| `internal/indexer/ignore.go` | Pattern loading and matching |
| `internal/indexer/ignore_test.go` | Unit tests for pattern matching |
| `internal/indexer/integration_test.go` | Integration tests for indexing |
| `docs/codetectignore.md` | Comprehensive user guide |

## Files to Modify

| File | Changes |
|------|---------|
| `go.mod` | Add `github.com/sabhiram/go-gitignore` dependency |
| `internal/indexer/indexer.go` | Add ignore pattern checking to `scanFiles()` |
| `internal/config/index.go` | Add indexing configuration section |
| `cmd/codetect-index/main.go` | Add `--ignore-file` and `--no-ignore` flags |
| `README.md` | Add .codetectignore section |
| `docs/installation.md` | Add .codetectignore setup instructions |

---

## Success Metrics

### Functional Metrics
- ✅ .codetectignore syntax matches .gitignore 100%
- ✅ All common use cases work correctly
- ✅ Hierarchical loading (project + global) works
- ✅ Negation patterns work correctly

### Performance Metrics
- ✅ No regression on small repos (<1k files)
- ✅ Faster indexing on large repos with exclusions (10k+ files)
- ✅ Pattern matching adds <5% overhead

### User Experience Metrics
- ✅ Documentation is clear and comprehensive
- ✅ CLI flags are intuitive
- ✅ Error messages are helpful
- ✅ Verbose mode shows excluded files for debugging

---

## Risks & Mitigations

### Risk: Accidental Over-Exclusion

**Problem:** User excludes too much and breaks search

**Mitigation:**
- No default .codetectignore (explicit opt-in)
- Verbose mode shows excluded files
- `--dry-run` flag to preview exclusions (future enhancement)

### Risk: Pattern Matching Performance

**Problem:** Checking patterns for every file is slow

**Mitigation:**
- Use compiled patterns (go-gitignore compiles once)
- Directory exclusions skip entire subtrees
- Benchmark with 10k+ file repos before shipping

### Risk: Confusion with .gitignore

**Problem:** Users expect .codetectignore to behave like .gitignore

**Mitigation:**
- Clear documentation on independence
- Examples showing all 4 scenarios (tracked/ignored vs indexed/excluded)
- FAQ section addressing common confusion

---

## Dependencies

**External Libraries:**
- `github.com/sabhiram/go-gitignore` (MIT license, mature, well-tested)

**Internal Dependencies:**
- `internal/indexer/indexer.go` (file scanning logic)
- `internal/config/` (configuration system)

**No Breaking Changes:**
- .codetectignore is optional (default: all files indexed)
- Existing behavior unchanged if .codetectignore doesn't exist

---

## Testing Strategy

### Unit Tests
- Pattern matching (20+ test cases)
- Pattern loading (project, global, hierarchy)
- Edge cases (empty file, no file, conflicting patterns)

### Integration Tests
- Full indexing flow with .codetectignore
- Verify excluded files don't appear in search results
- Verify included files do appear in search results
- Test all common use cases

### Performance Tests
- Benchmark indexing with/without .codetectignore
- Test with 10k+ file repos
- Measure pattern matching overhead

---

## Rollout Plan

### Phase 1: Implement Core
1. Add dependency
2. Implement pattern loading
3. Integrate with file scanning
4. Add CLI flags

### Phase 2: Test & Validate
1. Write unit tests
2. Write integration tests
3. Test common use cases
4. Benchmark performance

### Phase 3: Document & Ship
1. Write docs/codetectignore.md
2. Update README.md
3. Create example .codetectignore files
4. Ship in next release

---

## Future Enhancements (Deferred)

**Not in Phase 1d scope:**

- `.codetectignore` in subdirectories (only root-level for now)
- `--dry-run` flag to preview exclusions
- UI for managing exclusions
- Per-tool ignore (different patterns for indexing vs embedding)
- Real-time reload (requires daemon restart for now)

These can be added in Phase 2 if user feedback requests them.

---

## Reference Documentation

**Specification:** `context/data/2026-02-03-codetectignore-spec.md`
**Master Plan:** `context/plans/2026-02-02-phase1-implementation-roadmap.md`

**External References:**
- [.gitignore syntax](https://git-scm.com/docs/gitignore)
- [go-gitignore library](https://github.com/sabhiram/go-gitignore)

---

## Review Checklist

Before starting implementation:
- [x] Specification reviewed and approved
- [x] Library choice validated (go-gitignore)
- [x] Integration points identified
- [x] Test strategy defined
- [x] Documentation plan complete
- [x] Timeline realistic (1 week)

---

**End of Plan**
