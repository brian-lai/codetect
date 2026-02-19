# .codetectignore Guide

## Overview

`.codetectignore` lets you control which files codetect indexes and embeds, independent of `.gitignore`.

**Use it to:**
- Exclude generated code
- Skip minified/bundled files
- Ignore test fixtures
- Filter out large data files

## File Locations

Patterns are loaded from three locations and merged (OR logic — a file is excluded if it matches any pattern from any source):

| Priority | Path | Notes |
|---|---|---|
| 1 (highest) | `<repo>/.codetectignore` | Project-specific; commit to VCS |
| 2 | `~/.config/codetect/ignore` | Global (XDG config dir) |
| 3 | `~/.codetectignore` | Legacy global — loaded with a deprecation warning |

### Global Ignore

Create `~/.config/codetect/ignore` to apply patterns across all your projects:

```bash
mkdir -p ~/.config/codetect
cat >> ~/.config/codetect/ignore << 'EOF'
# Global patterns for all projects
*.min.js
*.bundle.js
vendor/
EOF
```

**Migrating from `~/.codetectignore`:** If you have the legacy `~/.codetectignore`, move it:

```bash
mkdir -p ~/.config/codetect
mv ~/.codetectignore ~/.config/codetect/ignore
```

The legacy path still works but prints a deprecation warning on every `codetect index` run.

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
codetect index --force
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

## FAQ

**Q: Do I need .codetectignore?**
No, it's optional. By default, codetect uses reasonable ignore patterns.

**Q: Can I exclude files that are tracked in Git?**
Yes! `.codetectignore` is independent of `.gitignore`.

**Q: Can I include files that are gitignored?**
Yes, just don't add them to `.codetectignore`.

**Q: How do I see what files were excluded?**
Run with verbose mode: `codetect index --verbose`

**Q: Does .codetectignore affect search?**
Yes, excluded files won't appear in any search results.

**Q: Which ignore file takes precedence?**
Project `.codetectignore` patterns always apply. All three sources are merged — a file
is excluded if it matches any pattern from any file.

## Best Practices

1. **Start small** - Don't over-exclude. Add patterns as needed.
2. **Test first** - Use `--verbose` to see what gets excluded.
3. **Be specific** - Prefer specific patterns (`*.generated.go`) over broad ones (`src/`).
4. **Document** - Add comments to explain why patterns exist.
5. **Use global for personal preferences** - Put patterns that apply to all your projects
   (e.g., editor temp files) in `~/.config/codetect/ignore`.

## Troubleshooting

**Problem:** Files are still being indexed despite .codetectignore

**Solutions:**
- Check pattern syntax (use `.gitignore` format)
- Run `codetect index --force` to force reindex
- Use `--verbose` flag to see what's being excluded
- Verify .codetectignore is in repository root
- Run `codetect doctor` to see which ignore files are active

**Problem:** Too many files are excluded

**Solutions:**
- Review your patterns
- Remove or comment out broad patterns
- Use more specific exclusions

**Problem:** Seeing "deprecated" warning about ~/.codetectignore

**Solution:** Move the file to the XDG config location:
```bash
mkdir -p ~/.config/codetect
mv ~/.codetectignore ~/.config/codetect/ignore
```

## See Also

- [Installation Guide](installation.md)
- [Architecture](architecture.md)
- [.gitignore syntax](https://git-scm.com/docs/gitignore)
