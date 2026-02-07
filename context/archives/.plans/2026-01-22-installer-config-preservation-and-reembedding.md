# Plan: Installer Configuration Preservation and Re-embedding Support

**Date:** 2026-01-22
**Task:** Fix installer to preserve existing config and handle embedding dimension changes
**Type:** Bug Fix + Enhancement
**Complexity:** Medium (config merging logic + dimension detection)

---

## Context

The current installer has several issues when users re-run it after initial installation:

### Problems Identified

1. **Config Overwrite (Line 965)**
   - Uses `cat >` which completely overwrites existing config
   - Line 132 claims "Your existing configuration will be preserved" - **this is false**
   - Users lose custom settings (custom URLs, API keys, manual tweaks)

2. **No Dimension Mismatch Detection**
   - User switches from nomic-embed-text (768 dims) to bge-m3 (1024 dims)
   - Existing embeddings tables remain with 768-dim vectors
   - New config expects 1024-dim vectors
   - Next `repo-search embed` will **fail silently or with cryptic errors**

3. **No Re-embedding Guidance**
   - Installer doesn't detect existing embeddings
   - Doesn't warn about incompatibility
   - Doesn't offer to re-embed automatically
   - Users left to figure out they need `repo-search embed --force`

4. **Poor User Experience on Reinstall**
   - No clear indication of what changed
   - No summary of "before vs after" config
   - No option to keep existing settings

### User Impact

**Scenario 1: User upgrades to better model**
```bash
# Initial install: selected nomic-embed-text (768 dims)
./install.sh  # Created embeddings for 3 repos

# Later: wants to upgrade to bge-m3 (1024 dims)
./install.sh  # Runs again, selects bge-m3

# Result: Config updated, but embeddings still 768 dims
cd ~/my-project
repo-search embed  # FAILS: dimension mismatch error
```

**Scenario 2: User has custom config**
```bash
# Manually edited ~/.config/repo-search/config.env
export REPO_SEARCH_OLLAMA_URL="http://192.168.1.100:11434"  # Custom host
export REPO_SEARCH_DB_DSN="postgres://custom-db:5432/search"  # Custom DB

# Later: re-runs installer to update binaries
./install.sh

# Result: All custom settings LOST, config overwritten with defaults
```

---

## Objective

Make the installer safe and intelligent for re-installation scenarios:

1. **Preserve existing configuration** - Merge new settings with existing config
2. **Detect dimension mismatches** - Check if model change affects existing embeddings
3. **Offer re-embedding** - Guide users through fixing dimension mismatches
4. **Provide clear feedback** - Show what changed and what needs to be done

---

## Approach

### Phase 1: Config Preservation Logic

**Goal:** Don't lose user's existing settings

**Implementation:**

1. **Before config generation (line ~960)**, check if config already exists:
   ```bash
   if [[ -f "$CONFIG_FILE" ]]; then
       # Config exists - load existing values
       source "$CONFIG_FILE"
       EXISTING_CONFIG=true
   else
       EXISTING_CONFIG=false
   fi
   ```

2. **For each config variable**, use existing value if not changed by user:
   ```bash
   # If user selected same model, keep existing dimensions
   if [[ $EXISTING_CONFIG == true && -n "$REPO_SEARCH_VECTOR_DIMENSIONS" ]]; then
       # Only update if model changed
       if [[ "$EMBEDDING_MODEL" != "$REPO_SEARCH_EMBEDDING_MODEL" ]]; then
           # Model changed - use new dimensions
           # (but warn user - see Phase 2)
       else
           # Same model - keep existing dimensions
           VECTOR_DIMENSIONS="$REPO_SEARCH_VECTOR_DIMENSIONS"
       fi
   fi
   ```

3. **Show diff before writing**:
   ```bash
   echo ""
   warn "Updating existing configuration"
   echo ""
   echo "Changes:"
   if [[ "$OLD_EMBEDDING_MODEL" != "$EMBEDDING_MODEL" ]]; then
       echo "  Model: $OLD_EMBEDDING_MODEL → $EMBEDDING_MODEL"
   fi
   if [[ "$OLD_VECTOR_DIMENSIONS" != "$VECTOR_DIMENSIONS" ]]; then
       echo "  Dimensions: $OLD_VECTOR_DIMENSIONS → $VECTOR_DIMENSIONS"
   fi
   echo ""
   read -p "$(prompt "Apply these changes? [Y/n]")" APPLY_CHANGES
   ```

4. **Preserve custom settings** - Don't prompt for values that already exist:
   ```bash
   # Only ask for Ollama URL if not in existing config
   if [[ -z "$REPO_SEARCH_OLLAMA_URL" ]]; then
       read -p "$(prompt "Ollama URL [http://localhost:11434]")" OLLAMA_URL
   else
       # Keep existing
       OLLAMA_URL="$REPO_SEARCH_OLLAMA_URL"
       info "Using existing Ollama URL: $OLLAMA_URL"
   fi
   ```

### Phase 2: Dimension Mismatch Detection

**Goal:** Detect when model change will break existing embeddings

**Implementation:**

1. **After model selection**, check if dimensions changed:
   ```bash
   DIMENSION_MISMATCH=false

   if [[ $EXISTING_CONFIG == true ]]; then
       OLD_DIMENSIONS="${REPO_SEARCH_VECTOR_DIMENSIONS:-768}"
       NEW_DIMENSIONS="$VECTOR_DIMENSIONS"

       if [[ "$OLD_DIMENSIONS" != "$NEW_DIMENSIONS" ]]; then
           DIMENSION_MISMATCH=true
       fi
   fi
   ```

2. **Check for existing embeddings** (SQLite and PostgreSQL):
   ```bash
   if [[ $DIMENSION_MISMATCH == true ]]; then
       # Check for existing embeddings
       EXISTING_EMBEDDINGS=false

       if [[ $DB_TYPE == "sqlite" ]]; then
           # Check common SQLite locations
           if find ~ -name "symbols.db" -o -name ".repo_search" -type d 2>/dev/null | head -1; then
               EXISTING_EMBEDDINGS=true
           fi
       elif [[ $DB_TYPE == "postgres" && -n "$POSTGRES_DSN" ]]; then
           # Check PostgreSQL for embeddings table
           if psql "$POSTGRES_DSN" -c "SELECT 1 FROM embeddings LIMIT 1" &>/dev/null; then
               EXISTING_EMBEDDINGS=true
           fi
       fi
   fi
   ```

3. **Show dimension mismatch warning**:
   ```bash
   if [[ $DIMENSION_MISMATCH == true && $EXISTING_EMBEDDINGS == true ]]; then
       print_box "$YELLOW" \
           "⚠️  DIMENSION MISMATCH DETECTED" \
           "" \
           "Your embedding model change will cause dimension mismatch:" \
           "  • Old model: $OLD_MODEL ($OLD_DIMENSIONS dimensions)" \
           "  • New model: $EMBEDDING_MODEL ($NEW_DIMENSIONS dimensions)" \
           "" \
           "Existing embeddings were created with $OLD_DIMENSIONS dimensions." \
           "The new model produces $NEW_DIMENSIONS-dimensional vectors." \
           "" \
           "${BOLD}This will break semantic search in existing repositories!${NC}" \
           "" \
           "You must re-embed all repositories after installation."
   fi
   ```

### Phase 3: Re-embedding Support

**Goal:** Guide users through fixing their embeddings

**Implementation:**

1. **Detect indexed repositories** (from registry or common locations):
   ```bash
   # Load repository registry if it exists
   REGISTRY_FILE="$HOME/.config/repo-search/registry.json"
   INDEXED_REPOS=()

   if [[ -f "$REGISTRY_FILE" ]]; then
       # Parse JSON to get list of indexed repos
       # (Use jq if available, or grep/sed fallback)
       INDEXED_REPOS=($(jq -r '.repositories[].path' "$REGISTRY_FILE" 2>/dev/null || echo ""))
   fi
   ```

2. **Offer batch re-embedding**:
   ```bash
   if [[ $DIMENSION_MISMATCH == true && ${#INDEXED_REPOS[@]} -gt 0 ]]; then
       echo ""
       info "Found ${#INDEXED_REPOS[@]} indexed repositories:"
       for repo in "${INDEXED_REPOS[@]}"; do
           echo "  • $repo"
       done
       echo ""
       read -p "$(prompt "Re-embed all repositories now? [Y/n]")" REEMBED_NOW
       REEMBED_NOW=${REEMBED_NOW:-Y}

       if [[ $REEMBED_NOW =~ ^[Yy] ]]; then
           REEMBED_AFTER_INSTALL=true
       else
           REEMBED_AFTER_INSTALL=false
           warn "You MUST re-embed manually before using semantic search"
           info "For each repository, run:"
           echo "    cd /path/to/repo"
           echo "    repo-search embed --force"
       fi
   fi
   ```

3. **Execute re-embedding after installation** (at end of install.sh):
   ```bash
   if [[ $REEMBED_AFTER_INSTALL == true ]]; then
       print_header "Re-embedding Repositories"

       for repo in "${INDEXED_REPOS[@]}"; do
           echo ""
           info "Re-embedding: $repo"

           if cd "$repo" && repo-search embed --force; then
               success "✓ $repo re-embedded successfully"
           else
               error "✗ Failed to re-embed $repo"
               warn "You'll need to re-embed manually: cd $repo && repo-search embed --force"
           fi
       done
   fi
   ```

4. **PostgreSQL special handling** - Drop/recreate embeddings table:
   ```bash
   if [[ $DB_TYPE == "postgres" && $DIMENSION_MISMATCH == true ]]; then
       echo ""
       warn "PostgreSQL embeddings table must be recreated"
       info "This will drop the 'embeddings' table and recreate with new dimensions"
       echo ""
       read -p "$(prompt "Drop and recreate embeddings table? [Y/n]")" DROP_TABLE
       DROP_TABLE=${DROP_TABLE:-Y}

       if [[ $DROP_TABLE =~ ^[Yy] ]]; then
           info "Dropping embeddings table..."
           if psql "$POSTGRES_DSN" -c "DROP TABLE IF EXISTS embeddings CASCADE;"; then
               success "Embeddings table dropped"
           else
               error "Failed to drop table"
               warn "You may need to drop manually: psql -c 'DROP TABLE embeddings CASCADE;'"
           fi
       fi
   fi
   ```

### Phase 4: User Experience Improvements

**Goal:** Make reinstallation process clear and confidence-inspiring

**Implementation:**

1. **Show config comparison** before and after:
   ```bash
   print_section "Configuration Summary"

   echo "Current configuration:"
   echo "  Database: $OLD_DB_TYPE"
   echo "  Model: $OLD_EMBEDDING_MODEL ($OLD_VECTOR_DIMENSIONS dims)"
   echo "  Provider: $OLD_EMBEDDING_PROVIDER"
   echo ""
   echo "New configuration:"
   echo "  Database: $DB_TYPE"
   echo "  Model: $EMBEDDING_MODEL ($VECTOR_DIMENSIONS dims)"
   echo "  Provider: $EMBEDDING_PROVIDER"
   echo ""

   if [[ "$OLD_EMBEDDING_MODEL" == "$EMBEDDING_MODEL" ]]; then
       success "No model change - existing embeddings will continue to work"
   else
       warn "Model changed - re-embedding required"
   fi
   ```

2. **Generate migration script** for manual re-embedding:
   ```bash
   MIGRATION_SCRIPT="/tmp/reembed-repos.sh"
   cat > "$MIGRATION_SCRIPT" << EOF
   #!/bin/bash
   # Auto-generated migration script
   # Re-embeds all indexed repositories with new model

   echo "Re-embedding all repositories..."
   echo "Old model: $OLD_EMBEDDING_MODEL ($OLD_VECTOR_DIMENSIONS dims)"
   echo "New model: $EMBEDDING_MODEL ($VECTOR_DIMENSIONS dims)"
   echo ""

   # Source new config
   source "$CONFIG_FILE"

   # Re-embed each repository
   ${REEMBED_COMMANDS[@]}

   echo ""
   echo "Re-embedding complete!"
   EOF

   chmod +x "$MIGRATION_SCRIPT"
   info "Migration script saved to: $MIGRATION_SCRIPT"
   ```

3. **Add dry-run mode**:
   ```bash
   # At the start of install.sh
   DRY_RUN=false
   if [[ "$1" == "--dry-run" ]]; then
       DRY_RUN=true
       info "DRY RUN MODE - No changes will be made"
   fi

   # Before making changes
   if [[ $DRY_RUN == true ]]; then
       echo ""
       echo "Changes that would be made:"
       echo "  • Config: $CONFIG_FILE"
       echo "  • Model: $EMBEDDING_MODEL ($VECTOR_DIMENSIONS dims)"
       if [[ $DIMENSION_MISMATCH == true ]]; then
           echo "  • Re-embed required: Yes"
       fi
       exit 0
   fi
   ```

---

## Detailed Implementation Steps

### Step 1: Refactor Config Generation Section (lines 960-1005)

**Current:**
```bash
cat > "$CONFIG_FILE" << EOF
# Overwrites everything
EOF
```

**New:**
```bash
# Load existing config if present
OLD_CONFIG_BACKUP="$CONFIG_FILE.backup.$(date +%s)"
if [[ -f "$CONFIG_FILE" ]]; then
    cp "$CONFIG_FILE" "$OLD_CONFIG_BACKUP"
    info "Backed up existing config to: $OLD_CONFIG_BACKUP"

    # Source existing values
    source "$CONFIG_FILE"
    EXISTING_CONFIG=true

    # Store old values for comparison
    OLD_DB_TYPE="$REPO_SEARCH_DB_TYPE"
    OLD_EMBEDDING_MODEL="$REPO_SEARCH_EMBEDDING_MODEL"
    OLD_VECTOR_DIMENSIONS="${REPO_SEARCH_VECTOR_DIMENSIONS:-768}"
    OLD_EMBEDDING_PROVIDER="$REPO_SEARCH_EMBEDDING_PROVIDER"
else
    EXISTING_CONFIG=false
fi

# Generate new config (same as before)
cat > "$CONFIG_FILE" << EOF
# ... new config ...
EOF
```

### Step 2: Add Dimension Check After Model Selection (after line 419)

**Insert after model selection:**
```bash
success "Selected: $EMBEDDING_MODEL (dimensions: $VECTOR_DIMENSIONS)"
echo ""

# Check for dimension mismatch with existing config
if [[ $EXISTING_CONFIG == true ]]; then
    OLD_DIMENSIONS="${REPO_SEARCH_VECTOR_DIMENSIONS:-768}"

    if [[ "$OLD_DIMENSIONS" != "$VECTOR_DIMENSIONS" ]]; then
        DIMENSION_MISMATCH=true

        print_box "$YELLOW" \
            "⚠️  DIMENSION CHANGE DETECTED" \
            "" \
            "Previous model: $OLD_EMBEDDING_MODEL ($OLD_DIMENSIONS dims)" \
            "New model: $EMBEDDING_MODEL ($VECTOR_DIMENSIONS dims)" \
            "" \
            "This change requires re-embedding all indexed repositories." \
            "The installer will help you with this after installation."

        echo ""
        read -p "$(prompt "Continue with model change? [Y/n]")" CONTINUE_CHANGE
        CONTINUE_CHANGE=${CONTINUE_CHANGE:-Y}

        if [[ ! $CONTINUE_CHANGE =~ ^[Yy] ]]; then
            info "Keeping existing model: $OLD_EMBEDDING_MODEL"
            EMBEDDING_MODEL="$OLD_EMBEDDING_MODEL"
            VECTOR_DIMENSIONS="$OLD_DIMENSIONS"
            DIMENSION_MISMATCH=false
        fi
    else
        info "No dimension change - existing embeddings will continue to work"
    fi
fi
```

### Step 3: Add Repository Detection (after Step 5: Build and Install)

**Insert before final summary (around line 1020):**
```bash
#
# Step 6: Handle Embedding Migration (if needed)
#
if [[ $DIMENSION_MISMATCH == true ]]; then
    print_header "Step 6/6: Embedding Migration"

    # Detect indexed repositories
    REGISTRY_FILE="$HOME/.config/repo-search/registry.json"
    INDEXED_REPOS=()

    if [[ -f "$REGISTRY_FILE" ]]; then
        # Try to parse with jq, fallback to grep
        if command -v jq &> /dev/null; then
            INDEXED_REPOS=($(jq -r '.repositories[]?.path // empty' "$REGISTRY_FILE" 2>/dev/null))
        else
            # Fallback: grep for paths (crude but works)
            INDEXED_REPOS=($(grep -oP '"path":\s*"\K[^"]+' "$REGISTRY_FILE" 2>/dev/null))
        fi
    fi

    # Also check for .repo_search directories (repositories with local embeddings)
    if [[ $DB_TYPE == "sqlite" ]]; then
        # Find directories with .repo_search subdirectories
        while IFS= read -r repo_dir; do
            INDEXED_REPOS+=("$(dirname "$repo_dir")")
        done < <(find ~ -type d -name ".repo_search" 2>/dev/null)
    fi

    # Remove duplicates
    INDEXED_REPOS=($(printf '%s\n' "${INDEXED_REPOS[@]}" | sort -u))

    if [[ ${#INDEXED_REPOS[@]} -gt 0 ]]; then
        echo ""
        warn "Found ${#INDEXED_REPOS[@]} indexed repositories that need re-embedding:"
        for repo in "${INDEXED_REPOS[@]}"; do
            echo "  • $repo"
        done
        echo ""

        read -p "$(prompt "Re-embed all repositories now? [Y/n]")" REEMBED_NOW
        REEMBED_NOW=${REEMBED_NOW:-Y}

        if [[ $REEMBED_NOW =~ ^[Yy] ]]; then
            echo ""
            info "Re-embedding repositories (this may take several minutes)..."

            # Source new config to use new model
            source "$CONFIG_FILE"

            SUCCESS_COUNT=0
            FAIL_COUNT=0

            for repo in "${INDEXED_REPOS[@]}"; do
                echo ""
                info "Re-embedding: $repo"

                if [[ -d "$repo" ]]; then
                    if (cd "$repo" && repo-search embed --force &> /tmp/reembed-$$.log); then
                        success "✓ $repo"
                        ((SUCCESS_COUNT++))
                    else
                        error "✗ $repo (see /tmp/reembed-$$.log)"
                        ((FAIL_COUNT++))
                    fi
                else
                    warn "✗ $repo (directory not found)"
                    ((FAIL_COUNT++))
                fi
            done

            echo ""
            if [[ $FAIL_COUNT -eq 0 ]]; then
                success "All $SUCCESS_COUNT repositories re-embedded successfully!"
            else
                warn "$SUCCESS_COUNT succeeded, $FAIL_COUNT failed"
                info "Failed repositories must be re-embedded manually:"
                echo "    cd /path/to/repo"
                echo "    repo-search embed --force"
            fi
        else
            # Generate migration script
            MIGRATION_SCRIPT="$HOME/reembed-repos.sh"
            cat > "$MIGRATION_SCRIPT" << EOF
#!/bin/bash
# Auto-generated migration script for re-embedding repositories
# Generated: $(date)

set -e

echo "Re-embedding all repositories with new model..."
echo "Model: $EMBEDDING_MODEL ($VECTOR_DIMENSIONS dimensions)"
echo ""

# Source new configuration
source "$CONFIG_FILE"

# Re-embed each repository
EOF

            for repo in "${INDEXED_REPOS[@]}"; do
                cat >> "$MIGRATION_SCRIPT" << EOF

echo "Re-embedding: $repo"
cd "$repo" && repo-search embed --force
EOF
            done

            cat >> "$MIGRATION_SCRIPT" << EOF

echo ""
echo "✓ All repositories re-embedded successfully!"
EOF

            chmod +x "$MIGRATION_SCRIPT"

            echo ""
            warn "Re-embedding skipped"
            success "Migration script saved to: $MIGRATION_SCRIPT"
            info "Run it when you're ready: bash $MIGRATION_SCRIPT"
        fi
    else
        info "No indexed repositories found - you're all set!"
    fi
fi
```

### Step 4: Update Final Summary (lines 1022-1088)

**Add to summary:**
```bash
print_box "$MAGENTA" \
    "${BOLD}Configuration Changes${NC}" \
    "  Old model: $OLD_EMBEDDING_MODEL ($OLD_VECTOR_DIMENSIONS dims)" \
    "  New model: $EMBEDDING_MODEL ($VECTOR_DIMENSIONS dims)" \
    "  $(if [[ $DIMENSION_MISMATCH == true ]]; then echo "${YELLOW}⚠️  Re-embedding required${NC}"; else echo "${GREEN}✓ No re-embedding needed${NC}"; fi)"
```

---

## Edge Cases & Error Handling

### 1. Config File Parsing Failures

**Issue:** Existing config may have syntax errors or be corrupted

**Solution:**
```bash
if [[ -f "$CONFIG_FILE" ]]; then
    # Try to source it safely
    if source "$CONFIG_FILE" 2>/dev/null; then
        EXISTING_CONFIG=true
    else
        warn "Existing config is corrupted or has syntax errors"
        read -p "$(prompt "Overwrite with new config? [Y/n]")" OVERWRITE
        if [[ ! $OVERWRITE =~ ^[Yy] ]]; then
            error "Cannot proceed with corrupted config"
            exit 1
        fi
        EXISTING_CONFIG=false
    fi
fi
```

### 2. Repository Detection Failures

**Issue:** Registry file missing or JSON parsing fails

**Solution:**
```bash
# Multiple fallback methods:
# 1. Try jq (most reliable)
# 2. Try grep/sed (crude but works)
# 3. Find .repo_search directories (last resort)
# 4. If all fail, prompt user manually
```

### 3. PostgreSQL Connection Failures

**Issue:** Can't check for embeddings table if DB is down

**Solution:**
```bash
if ! psql "$POSTGRES_DSN" -c "SELECT 1" &>/dev/null; then
    warn "Cannot connect to PostgreSQL"
    info "Unable to check for existing embeddings"
    info "If you have existing embeddings, you'll need to re-embed manually"

    read -p "$(prompt "Continue anyway? [Y/n]")" CONTINUE
    if [[ ! $CONTINUE =~ ^[Yy] ]]; then
        exit 1
    fi
fi
```

### 4. Re-embedding Failures

**Issue:** `repo-search embed --force` fails for some repos

**Solution:**
```bash
# Log failures to file
FAILED_REPOS=()

for repo in "${INDEXED_REPOS[@]}"; do
    if ! (cd "$repo" && repo-search embed --force); then
        FAILED_REPOS+=("$repo")
    fi
done

if [[ ${#FAILED_REPOS[@]} -gt 0 ]]; then
    warn "Failed to re-embed ${#FAILED_REPOS[@]} repositories:"
    for repo in "${FAILED_REPOS[@]}"; do
        echo "  • $repo"
    done

    # Save failed repos to file for retry
    RETRY_SCRIPT="$HOME/reembed-failed-repos.sh"
    # ... generate retry script ...
fi
```

### 5. Dimension Detection Ambiguity

**Issue:** Can't determine dimensions from existing config (old config format)

**Solution:**
```bash
if [[ -z "$REPO_SEARCH_VECTOR_DIMENSIONS" ]]; then
    # Guess based on model
    case "$REPO_SEARCH_EMBEDDING_MODEL" in
        nomic-embed-text|nomic-embed-text:1.5)
            OLD_VECTOR_DIMENSIONS="768"
            ;;
        bge-m3|snowflake-arctic-embed|jina-embeddings-v3)
            OLD_VECTOR_DIMENSIONS="1024"
            ;;
        *)
            # Unknown - ask user
            read -p "$(prompt "Enter dimensions of existing embeddings [768/1024]")" OLD_VECTOR_DIMENSIONS
            OLD_VECTOR_DIMENSIONS=${OLD_VECTOR_DIMENSIONS:-768}
            ;;
    esac

    warn "REPO_SEARCH_VECTOR_DIMENSIONS not set in config"
    info "Inferred dimensions: $OLD_VECTOR_DIMENSIONS (based on model $REPO_SEARCH_EMBEDDING_MODEL)"
fi
```

### 6. User Changes Mind Mid-Process

**Issue:** User realizes they don't want to change models

**Solution:**
```bash
# Add confirmation checkpoints
read -p "$(prompt "Continue with model change? [Y/n]")" CONTINUE_CHANGE

# Allow rollback
if [[ -f "$OLD_CONFIG_BACKUP" ]]; then
    warn "Installation cancelled - restoring backup"
    cp "$OLD_CONFIG_BACKUP" "$CONFIG_FILE"
fi
```

---

## Testing Strategy

### Manual Testing Scenarios

#### Test 1: Fresh Install (Baseline)
```bash
# No existing config
./install.sh
# Select bge-m3
# Verify: Config created with VECTOR_DIMENSIONS=1024
```

#### Test 2: Reinstall Same Model
```bash
# Existing config: nomic-embed-text (768)
./install.sh
# Select nomic-embed-text again
# Expected: No dimension mismatch warning, no re-embedding needed
```

#### Test 3: Upgrade Model (Dimension Change)
```bash
# Existing config: nomic-embed-text (768)
# Create some embeddings first
mkdir test-repo && cd test-repo && git init
repo-search embed .

# Now upgrade
cd ~/codetect
./install.sh
# Select bge-m3 (1024)
# Expected:
# - Dimension mismatch warning
# - Offer to re-embed
# - Re-embedding succeeds
```

#### Test 4: Downgrade Model (Rare but possible)
```bash
# Existing config: bge-m3 (1024)
./install.sh
# Select nomic-embed-text (768)
# Expected: Same dimension mismatch handling
```

#### Test 5: Custom Settings Preserved
```bash
# Edit config manually
echo 'export REPO_SEARCH_OLLAMA_URL="http://custom:11434"' >> ~/.config/repo-search/config.env

./install.sh
# Reinstall
# Expected: Custom URL preserved (not overwritten)
```

#### Test 6: PostgreSQL Dimension Change
```bash
# Start with SQLite/nomic
./install.sh  # Select SQLite, nomic-embed-text

# Change to PostgreSQL/bge-m3
./install.sh  # Select PostgreSQL, bge-m3
# Expected: Offer to drop embeddings table
```

#### Test 7: Registry Detection
```bash
# Index several repositories
repo-search index ~/project1
repo-search index ~/project2
repo-search index ~/project3

# Change model
./install.sh  # Select different model
# Expected: Detects all 3 repos, offers batch re-embed
```

#### Test 8: Failed Re-embedding
```bash
# Create scenario where one repo fails to embed
chmod 000 ~/project2  # Make unreadable

./install.sh
# Change model, opt for re-embedding
# Expected:
# - Embeds project1: success
# - Embeds project2: fails gracefully
# - Embeds project3: success
# - Shows failure summary
# - Generates retry script
```

#### Test 9: Corrupted Config
```bash
# Corrupt existing config
echo "INVALID SYNTAX @#$%" >> ~/.config/repo-search/config.env

./install.sh
# Expected: Detects corruption, offers to overwrite, doesn't crash
```

#### Test 10: Ollama Not Running During Reinstall
```bash
# Stop Ollama
killall ollama

./install.sh
# Change model
# Expected: Can't detect existing models, graceful handling
```

### Automated Testing (Optional)

Create a test harness script:

```bash
#!/bin/bash
# test-installer-reinstall.sh

run_test() {
    local test_name=$1
    echo "Running: $test_name"

    # Setup
    setup_test_env

    # Execute
    ./install.sh < test-input.txt

    # Verify
    verify_expectations

    # Cleanup
    cleanup_test_env
}

# Run all tests
run_test "fresh_install"
run_test "same_model_reinstall"
run_test "model_upgrade"
# ... etc
```

---

## Risks & Mitigations

### Risk 1: Config Backup Failures

**Impact:** High - Could lose user's custom config
**Likelihood:** Low
**Mitigation:**
- Always backup before modifying
- Use atomic writes (write to temp, then mv)
- Fail loudly if backup fails
```bash
if ! cp "$CONFIG_FILE" "$BACKUP_FILE"; then
    error "Failed to backup config - ABORTING"
    exit 1
fi
```

### Risk 2: Registry Parsing Errors

**Impact:** Medium - Might miss repositories to re-embed
**Likelihood:** Medium (JSON format changes, corruption)
**Mitigation:**
- Multiple fallback methods (jq, grep, find)
- Allow user to manually specify repos
- Generate retry script even if detection fails

### Risk 3: Batch Re-embedding Takes Too Long

**Impact:** Low - User impatience
**Likelihood:** High (large repos, many repos)
**Mitigation:**
- Show progress per-repo
- Allow skipping with script generation
- Add time estimates
```bash
info "Estimated time: ~3 minutes per repository"
```

### Risk 4: PostgreSQL Table Drop is Destructive

**Impact:** High - Data loss
**Likelihood:** Medium (user clicks through warnings)
**Mitigation:**
- Clear warnings with bold text
- Require explicit confirmation
- Offer backup/export before drop
```bash
read -p "$(prompt "Type 'YES' to confirm dropping embeddings table")" CONFIRM
if [[ "$CONFIRM" != "YES" ]]; then
    info "Aborted"
    exit 1
fi
```

### Risk 5: Detection Misses Edge Cases

**Impact:** Medium - User has to manually fix
**Likelihood:** High (unusual setups)
**Mitigation:**
- Provide comprehensive manual instructions
- Generate migration scripts as fallback
- Document troubleshooting steps

### Risk 6: Re-embedding Fails Silently

**Impact:** High - User thinks it worked but it didn't
**Likelihood:** Medium
**Mitigation:**
- Check exit codes
- Verify embeddings exist after re-embedding
- Show clear success/failure per repo
```bash
if repo-search embed --force && check_embeddings_exist; then
    success "✓ Re-embedded successfully"
else
    error "✗ Re-embedding failed"
fi
```

---

## Success Criteria

### Functional Requirements

✅ **Config Preservation:**
1. Existing config is backed up before modification
2. Custom settings (URLs, API keys) are not overwritten
3. User is shown diff of what will change
4. Config changes can be aborted

✅ **Dimension Mismatch Detection:**
1. Detects when model change affects dimensions
2. Checks for existing embeddings (SQLite + PostgreSQL)
3. Warns user with clear explanation
4. Prevents proceeding without acknowledgment

✅ **Re-embedding Support:**
1. Detects indexed repositories (registry + file search)
2. Offers batch re-embedding
3. Shows progress during re-embedding
4. Reports success/failure per repository
5. Generates retry script if user skips

✅ **User Experience:**
1. Clear messaging at each step
2. No silent failures
3. Provides actionable instructions
4. Can complete without manual intervention

### Non-Functional Requirements

✅ **Safety:**
- No data loss from config overwrite
- No breaking existing working setups
- Backups created before destructive operations

✅ **Robustness:**
- Handles missing dependencies (jq, psql)
- Graceful fallback for detection failures
- Works with corrupted configs

✅ **Performance:**
- Re-embedding doesn't block unnecessarily
- Can skip and defer to later
- Progress indication for long operations

---

## Implementation Order

### Priority 1: Config Preservation (Most Critical)
- Fix line 132's misleading message
- Implement backup and merge logic
- Show diff before writing
- **Time estimate:** 2 hours

### Priority 2: Dimension Detection
- Check existing config dimensions
- Compare with selected model
- Show clear warning
- **Time estimate:** 1.5 hours

### Priority 3: Repository Detection
- Parse registry.json
- Find .repo_search directories
- Handle edge cases
- **Time estimate:** 2 hours

### Priority 4: Re-embedding Flow
- Offer batch re-embed
- Execute with progress
- Generate retry script
- **Time estimate:** 2 hours

### Priority 5: UX Polish
- Config diff display
- Better messages
- Error handling
- **Time estimate:** 1 hour

**Total estimate:** ~8.5 hours (1 full dev day)

---

## Related Documentation

### Files to Update

1. **`install.sh`**
   - Lines 125-142: Enhance reinstall detection
   - Lines 960-1005: Refactor config generation
   - Lines 1020+: Add embedding migration step

2. **`docs/installation.md`**
   - Add section on reinstallation
   - Document what happens during upgrade
   - Show how to manually re-embed

3. **`README.md`**
   - Add note about re-embedding after model change
   - Link to migration guide

### New Documentation

1. **`docs/upgrading.md`** (new file)
   - Guide for upgrading between models
   - Troubleshooting dimension mismatches
   - Manual re-embedding instructions

2. **`docs/troubleshooting.md`** (new file)
   - Common reinstallation issues
   - Config corruption recovery
   - Embedding verification

---

## Questions to Resolve

1. **Should we backup the entire embeddings database before re-embedding?**
   - Pro: Safety net if something goes wrong
   - Con: Could be gigabytes of data, slow
   - **Recommendation:** Only for PostgreSQL (can pg_dump embeddings table)

2. **Should we auto-detect and stop if Ollama isn't running during re-embed?**
   - Pro: Prevents waste of time
   - Con: User might want to start Ollama during process
   - **Recommendation:** Check before starting batch, prompt to start Ollama

3. **Should we support rolling back to previous model if re-embedding fails?**
   - Pro: Safety feature
   - Con: Complex state management
   - **Recommendation:** Keep backup config, allow restore, but don't auto-rollback

4. **Should we rate-limit or throttle batch re-embedding?**
   - Pro: Prevents overwhelming Ollama
   - Con: Makes it slower
   - **Recommendation:** No throttling, but show ETA

5. **Should we verify embeddings after re-embedding (spot check)?**
   - Pro: Catches silent failures
   - Con: Adds time
   - **Recommendation:** Quick check: query for count of embeddings

---

## Notes

- This fix addresses a **major UX gap** that could frustrate users upgrading models
- Config preservation is especially important for users with custom PostgreSQL setups
- The dimension mismatch issue would cause cryptic errors without this fix
- Re-embedding support makes model upgrades painless
- Consider this a **high-priority bug fix** despite being an enhancement

---

**Ready to Execute:** After review
**Blockers:** None
**Dependencies:** None (self-contained to install.sh)
