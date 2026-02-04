# .codetectignore Guide

## Overview

`.codetectignore` lets you control which files codetect indexes and embeds, independent of `.gitignore`.

**Use it to:**
- Exclude generated code
- Skip minified/bundled files
- Ignore test fixtures
- Filter out large data files

## Quick Start

Create `.codetectignore` in your repository root:

```gitignore
# Exclude generated code
*.generated.ts
*.generated.go
*_pb.ts
*_pb.go

# Exclude minified/compiled files
*.min.js
*.bundle.js
dist/
build/

# Exclude test fixtures
fixtures/
testdata/
__snapshots__/
```

Then reindex:

```bash
cd /path/to/your/project
codetect-index index --force .
```

## Syntax

Uses standard `.gitignore` syntax:

| Pattern | Matches | Example |
|---------|---------|---------|
| `*.ext` | Files with extension | `*.min.js` → `app.min.js` |
| `dir/` | Directory | `dist/` → all files in `dist/` |
| `path/to/file` | Specific path | `src/generated.go` |
| `**/pattern` | Any directory level | `**/fixtures/*.json` |
| `#comment` | Comment line | `# This is ignored` |

## Examples

### Exclude Generated Code

```gitignore
*.generated.ts
*.generated.go
*_pb.go
*_pb.ts
schema.graphql.ts
```

### Exclude Build Artifacts

```gitignore
dist/
build/
out/
target/
*.min.js
*.bundle.js
*.map
```

### Exclude Test Fixtures

```gitignore
fixtures/
testdata/
__snapshots__/
*.fixture.json
```

### Exclude Large Data Files

```gitignore
*.csv
*.xml
*.json

# But include config files
!config.json
!package.json
!tsconfig.json
```

**Note:** Negation patterns (`!pattern`) have limited support.

## How It Works

### Independence from .gitignore

`.codetectignore` is **independent** of `.gitignore`:

| File Status | .gitignore | .codetectignore | Indexed? |
|-------------|------------|-----------------|----------|
| Tracked | No | No | ✅ Yes |
| Tracked | No | Yes | ❌ No |
| Ignored | Yes | No | ✅ Yes |
| Ignored | Yes | Yes | ❌ No |

**Example:** You can index `vendor/` even if it's gitignored, or exclude `*.generated.go` even if it's tracked.

### Hierarchical Loading

Patterns are loaded from:
1. `.codetectignore` in repository root (project-level)
2. `~/.codetectignore` in home directory (global)

Patterns from both files are combined (OR logic).

## FAQ

**Q: Do I need .codetectignore?**
No, it's optional. By default, codetect uses reasonable ignore patterns.

**Q: Can I exclude files that are tracked in Git?**
Yes! `.codetectignore` is independent of `.gitignore`.

**Q: Can I include files that are gitignored?**
Yes, just don't add them to `.codetectignore`.

**Q: How do I see what files were excluded?**
Run with verbose mode: `codetect-index index --verbose .`

**Q: Does .codetectignore affect search?**
Yes, excluded files won't appear in any search results.

## Best Practices

1. **Start small** - Don't over-exclude. Add patterns as needed.
2. **Test first** - Use `--verbose` to see what gets excluded.
3. **Be specific** - Prefer specific patterns (`*.generated.go`) over broad ones (`src/`).
4. **Document** - Add comments to explain why patterns exist.

## Troubleshooting

**Problem:** Files are still being indexed despite .codetectignore

**Solutions:**
- Check pattern syntax (use `.gitignore` format)
- Run `codetect-index index --force .` to force reindex
- Use `--verbose` flag to see what's being excluded
- Verify .codetectignore is in repository root

**Problem:** Too many files are excluded

**Solutions:**
- Review your patterns
- Remove or comment out broad patterns
- Use more specific exclusions

## See Also

- [Installation Guide](installation.md)
- [Architecture](architecture.md)
- [.gitignore syntax](https://git-scm.com/docs/gitignore)
