# Project Registry Guide

The codetect registry is a centralized tracking system for all indexed projects on your machine.

## What is the Registry?

The registry is a JSON file (`~/.config/codetect/registry.json`) that maintains a global catalog of all projects where you've run `codetect index`. It tracks:

- **Project paths** - Absolute paths to all indexed repositories
- **Index statistics** - Symbol counts, embedding counts, database size
- **Last indexed timestamp** - When each project was last indexed
- **Watch settings** - Whether auto-watch is enabled per project

## Why Use the Registry?

**Centralized Management:**
- View all indexed projects in one place
- Track index health across multiple repos
- Monitor storage usage across projects
- Coordinate multi-repo workflows

**Daemon Integration:**
- The background daemon uses the registry to track which projects to watch
- Auto-watch settings persist across daemon restarts
- Registry enables organization-scale monitoring

**Multi-Repo Support:**
- Essential for teams managing 10+ repositories
- Enables centralized PostgreSQL deployments
- Tracks dimension groups across repos

## Registry Commands

### `codetect registry list`

List all registered projects with their statistics.

**Usage:**
```bash
codetect registry list
```

**Output:**
```
Projects in Registry
====================

/Users/alice/projects/api
  Status:       ✅ Indexed
  Symbols:      12,345
  Embeddings:   8,456 (768-dim)
  Database:     47.2 MB
  Last indexed: 2 hours ago
  Watch:        enabled

/Users/alice/projects/frontend
  Status:       ⚠️  Not indexed
  Symbols:      0
  Embeddings:   0
  Database:     0 B
  Last indexed: never
  Watch:        disabled

Total: 2 projects (1 indexed, 1 pending)
```

**Filtering:**
```bash
# List only indexed projects
codetect registry list --indexed

# List only watched projects
codetect registry list --watched

# List projects with embeddings
codetect registry list --has-embeddings
```

### `codetect registry add`

Register the current directory in the registry.

**Usage:**
```bash
cd /path/to/project
codetect registry add
```

**When to use:**
- After running `codetect index` for the first time
- When setting up a new project for daemon monitoring
- To explicitly add a project without indexing

**Note:** `codetect index` automatically adds projects to the registry. Manual registration is optional.

**With options:**
```bash
# Add and enable auto-watch
codetect registry add --watch

# Add without indexing
codetect registry add --no-index
```

### `codetect registry remove`

Remove a project from the registry.

**Usage:**
```bash
# Remove current directory
cd /path/to/project
codetect registry remove

# Remove specific path
codetect registry remove /path/to/project
```

**What happens:**
- Project is removed from registry
- Centralized data directory is NOT deleted
- Project can be re-added later with `codetect registry add`

**Confirmation:**
```
Remove project from registry?
  Path: /Users/alice/projects/api
  This will not delete local indexes.
Continue? [y/N]: y
✓ Removed /Users/alice/projects/api from registry
```

### `codetect registry stats`

Show aggregate statistics across all registered projects.

**Usage:**
```bash
codetect registry stats
```

**Output:**
```
Registry Statistics
===================

Projects:     5 total (4 indexed, 1 pending)
Symbols:      45,678 total
Embeddings:   32,456 total (768-dim: 20,000 | 1024-dim: 12,456)
Storage:      234.5 MB total

Breakdown by Project:
  /Users/alice/projects/api         47.2 MB (12,345 symbols, 8,456 embeddings)
  /Users/alice/projects/frontend    28.1 MB (5,678 symbols, 4,321 embeddings)
  /Users/alice/projects/backend     89.3 MB (15,234 symbols, 12,000 embeddings)
  /Users/alice/projects/mobile      70.9 MB (12,421 symbols, 7,679 embeddings)
  /Users/alice/projects/legacy      0 B (not indexed)

Daemon Status: Running (watching 3 projects)
```

### `codetect migrate`

Discover existing indexes and register them.

**Usage:**
```bash
# Scan home directory
codetect migrate

# Scan specific directory
codetect migrate /path/to/search
```

**When to use:**
- After upgrading from v1 (which didn't have registry)
- When setting up registry for existing indexed projects

**What it does:**
1. Scans `~/.codetect/projects/index.json` for indexed projects
2. Checks if each project is already registered
3. Prompts to add unregistered projects
4. Updates statistics for existing projects

**Output:**
```
Scanning /Users/alice/projects for codetect indexes...

Found 5 indexed projects:
  ✓ /Users/alice/projects/api (already registered)
  ✓ /Users/alice/projects/frontend (already registered)
  + /Users/alice/projects/backend (new, will add)
  + /Users/alice/projects/mobile (new, will add)
  ✓ /Users/alice/projects/legacy (already registered)

Add 2 new projects to registry? [y/N]: y
✓ Added 2 projects to registry
```

## Registry File Format

The registry is stored at `~/.config/codetect/registry.json`:

```json
{
  "version": 1,
  "projects": {
    "/Users/alice/projects/api": {
      "path": "/Users/alice/projects/api",
      "symbols": 12345,
      "embeddings": 8456,
      "dimensions": 768,
      "db_size": 49500160,
      "last_indexed": "2026-02-01T15:30:00Z",
      "watch_enabled": true
    },
    "/Users/alice/projects/frontend": {
      "path": "/Users/alice/projects/frontend",
      "symbols": 0,
      "embeddings": 0,
      "dimensions": 0,
      "db_size": 0,
      "last_indexed": null,
      "watch_enabled": false
    }
  },
  "settings": {
    "auto_watch": true,
    "debounce_ms": 500
  }
}
```

**Manual editing:**
You can manually edit the registry file, but use caution:
- ✅ Safe: Change `watch_enabled` flags
- ✅ Safe: Update `settings`
- ⚠️ Risky: Change project paths (may break daemon)
- ⚠️ Risky: Modify statistics (will be overwritten on next index)

**Backup:**
```bash
cp ~/.config/codetect/registry.json ~/.config/codetect/registry.json.backup
```

## Registry and Daemon Integration

The registry is tightly integrated with the background daemon:

### Auto-Watch Settings

**Global auto-watch:**
```bash
# Enable auto-watch for all new projects
codetect registry config --auto-watch=true

# Check current setting
codetect registry config --show
```

**Per-project watch:**
```bash
# Enable watch for current project
cd /path/to/project
codetect registry watch --enable

# Disable watch
codetect registry watch --disable

# Check watch status
codetect registry watch --status
```

### Daemon Workflow

1. **Daemon starts** → Reads registry for watched projects
2. **File changes** → Daemon detects changes in watched projects
3. **Re-indexing** → Daemon runs `codetect index` automatically
4. **Registry update** → Daemon updates statistics after indexing

**View daemon status:**
```bash
codetect daemon status
```

**Output:**
```
Daemon Status: Running (PID: 12345)
Started: 3 hours ago

Watching 3 projects:
  /Users/alice/projects/api (last indexed: 5 min ago)
  /Users/alice/projects/frontend (last indexed: 1 hour ago)
  /Users/alice/projects/backend (last indexed: 3 hours ago)

Recent activity:
  15:30:00 - Detected change in api/handlers.go
  15:30:01 - Re-indexing /Users/alice/projects/api
  15:30:03 - Index complete (47.2 MB, 12,345 symbols)
  15:30:03 - Registry updated
```

## Multi-Repo Workflows

### Scenario 1: Organization with 10+ Repos

**Setup:**
```bash
# Use PostgreSQL for centralized storage
export CODETECT_DB_TYPE=postgres
export CODETECT_DB_DSN="postgres://codetect:codetect@db.company.com/codetect?sslmode=disable"

# Index all repos
for repo in ~/projects/*; do
  cd "$repo"
  codetect index
  codetect embed -j 10
done

# View aggregate stats
codetect registry stats
```

**Benefits:**
- Single command shows all project health
- Centralized PostgreSQL database
- Dimension-grouped tables isolate repos with different models

### Scenario 2: Selective Monitoring

**Enable watch for active projects only:**
```bash
# Enable watch for frequently edited repos
cd ~/projects/api && codetect registry watch --enable
cd ~/projects/frontend && codetect registry watch --enable

# Leave inactive repos unwatched
cd ~/projects/legacy && codetect registry watch --disable
```

### Scenario 3: CI/CD Integration

**Build script:**
```bash
#!/bin/bash
# .github/workflows/index.sh

# Install codetect
curl -sSL https://codetect.sh/install.sh | bash

# Index and update registry
codetect index --force
codetect embed --force -j 20

# Export stats to artifacts
codetect registry stats --json > index-stats.json
```

## Troubleshooting

### Registry Not Updating

**Symptom:**
```bash
codetect index
# Index succeeds, but registry shows zeros
```

**Solution:**
```bash
# Manual registry update
codetect registry update

# Check registry file exists
ls -la ~/.config/codetect/registry.json

# Verify permissions
chmod 644 ~/.config/codetect/registry.json
```

### Stale Projects in Registry

**Symptom:**
Registry lists projects that no longer exist.

**Solution:**
```bash
# Remove stale entries
codetect registry clean

# Or manually remove specific project
codetect registry remove /path/to/deleted/project
```

### Registry Corruption

**Symptom:**
```
ERROR: failed to parse registry: invalid JSON
```

**Solution:**
```bash
# Restore from backup
cp ~/.config/codetect/registry.json.backup ~/.config/codetect/registry.json

# Or rebuild registry
rm ~/.config/codetect/registry.json
codetect migrate ~/projects
```

### Multiple Users on Same Machine

Each user has their own registry:
```
/home/alice/.config/codetect/registry.json
/home/bob/.config/codetect/registry.json
```

**Shared projects:**
If multiple users index the same repo, each has their own centralized data directory:
```
~alice/.codetect/projects/shared-project-a1b2c3d4/
~bob/.codetect/projects/shared-project-a1b2c3d4/
```

**Best practice:**
Use a centralized PostgreSQL database for shared repos.

## Best Practices

### For Individual Developers

1. **Run `codetect migrate` after upgrade** - Discovers existing indexes
2. **Enable auto-watch for active projects** - Keeps indexes fresh
3. **Periodic `codetect registry stats`** - Monitor storage usage
4. **Disable watch for large monorepos** - Avoid excessive re-indexing

### For Teams

1. **Use PostgreSQL + registry** - Centralized tracking and storage
2. **Document watch policies** - Which repos should be watched
3. **Monitor aggregate stats** - Track org-wide index health
4. **Schedule periodic re-indexing** - Keep embeddings fresh

### For CI/CD

1. **Force full re-index** - Use `--force` flag in CI
2. **Export registry stats** - Track index size over time
3. **Cache embeddings** - Use content-addressed cache for speedup

## Advanced Usage

### Custom Registry Location

```bash
export CODETECT_REGISTRY_PATH=/custom/path/registry.json
codetect index
```

### JSON Output for Scripting

```bash
# Machine-readable output
codetect registry list --json
codetect registry stats --json

# Example: Count indexed projects
codetect registry list --json | jq '.projects | length'
```

### Batch Operations

```bash
# Enable watch for all projects
codetect registry watch --enable --all

# Re-index all registered projects
codetect registry reindex --all

# Clean and rebuild registry
codetect registry clean --rebuild
```

## See Also

- [Daemon Guide](daemon.md) - Background indexing setup
- [Installation Guide](installation.md) - Initial setup
- [Architecture](architecture.md) - Registry internals
- [PostgreSQL Setup](postgres-setup.md) - Multi-repo database

---

**Document Version:** 1.0
**Last Updated:** 2026-02-01
**codetect Version:** 2.0.0+
