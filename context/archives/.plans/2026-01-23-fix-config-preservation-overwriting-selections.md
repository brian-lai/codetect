# Fix Config Preservation Overwriting User Selections

**Date:** 2026-01-23
**Status:** Planning

## Objective

Fix the installer config preservation logic that overwrites user selections with old config values. Currently, when a user reinstalls and selects a new embedding model (e.g., `bge-m3`), the installer preserves the old value (`nomic-embed-text`) from the existing config file, ignoring the user's new selection.

## Problem Analysis

**Current behavior:**
1. User runs installer and selects `bge-m3` (option 1, recommended model)
2. Installer sets `EMBEDDING_MODEL="bge-m3"` (line 382)
3. Later, installer loads existing config (line 1128)
4. Config preservation overwrites selection: `EMBEDDING_MODEL="${CODETECT_EMBEDDING_MODEL:-$EMBEDDING_MODEL}"` (line 1138)
5. Old value `nomic-embed-text` replaces user's new selection `bge-m3`
6. User ends up with wrong model despite selecting the right one

**Root cause:**
The config preservation logic on line 1138 runs AFTER user selections, unconditionally preferring old values over new selections. This was intended to preserve custom settings like API keys and URLs, but it also prevents intentional model changes.

## Approach

### Solution: Detect Interactive Changes

The fix should distinguish between:
- **Values user intentionally changed** during prompts → Use NEW value
- **Values user didn't change** (kept defaults) → Preserve OLD value

**Strategy:**
1. Move config loading BEFORE interactive prompts
2. Use old values as DEFAULTS in prompts (not overrides after)
3. If user selects a DIFFERENT value than the default, honor their choice
4. If user keeps the default, preserve their existing config

This way:
- API keys, URLs, custom settings → preserved (user doesn't change them)
- Model selection → updated when user explicitly picks a different model
- Database type → updated when user explicitly changes it

### Implementation Steps

1. **Reorganize installer flow** (`install.sh`):
   - Move `load_existing_config()` call from line 1128 to BEFORE interactive prompts (around line 290)
   - Store loaded values as variables like `OLD_EMBEDDING_MODEL`, `OLD_DB_TYPE`, etc.

2. **Update prompt defaults**:
   - For model selection (line 376): Use `OLD_EMBEDDING_MODEL` to pre-select matching option
   - For database type: Use `OLD_DB_TYPE` as default
   - For URLs: Use old values as defaults

3. **Remove overwrite logic** (lines 1134-1140):
   - Delete the post-prompt overwrite logic that replaces user selections
   - Keep only the backup creation logic

4. **Preserve non-interactive values**:
   - API keys (no prompt) → preserve from old config
   - URLs (has prompt, user rarely changes) → use old as default, honor new if changed
   - Model (has prompt, user MAY change) → use old as default, honor new if changed

5. **Test scenarios**:
   - Fresh install → works as before
   - Reinstall keeping same model → preserves old config
   - Reinstall selecting new model → uses new selection
   - Reinstall with custom API keys → preserves keys

## Files to Modify

| File | Lines | Changes |
|------|-------|---------|
| `install.sh` | 290-420 | Move config loading before prompts, use old values as defaults |
| `install.sh` | 1110-1145 | Remove post-prompt overwrite logic |
| `install.sh` | 376-419 | Update model selection to detect and use old model as default |

## Risks

### Risk 1: Breaking Existing Behavior
- **Issue:** Users who rely on config preservation for other values
- **Mitigation:** Only change the timing of when config is loaded, not the preservation of non-interactive values (API keys, custom URLs)

### Risk 2: Default Detection
- **Issue:** How to detect if user kept the default vs. explicitly chose it?
- **Mitigation:** For model selection, we can detect by checking if the choice matches the old model's option number. If it does, preserve old value. If different option selected, use new value.

### Risk 3: Multiple Config Values
- **Issue:** Many config values to handle (DB type, model, URLs, keys, dimensions)
- **Mitigation:** Use consistent pattern: load old → use as default → honor explicit changes

## Success Criteria

1. **Fresh install works**: No existing config, prompts work normally
2. **Reinstall preserving works**: User keeps same model, config unchanged
3. **Reinstall updating works**: User selects new model (bge-m3), config updates to new model
4. **API keys preserved**: Custom API keys and URLs remain intact during reinstall
5. **Dimension mismatch detection works**: Changing from 768-dim to 1024-dim model shows warning

## Testing Plan

1. **Test 1: Fresh install**
   - Remove `~/.config/codetect/config.env`
   - Run installer, select bge-m3
   - Verify config has `CODETECT_EMBEDDING_MODEL="bge-m3"`

2. **Test 2: Reinstall with same model**
   - Set config to `nomic-embed-text`
   - Run installer, select option 4 (nomic-embed-text)
   - Verify config still has `nomic-embed-text`

3. **Test 3: Reinstall with new model** ⭐ KEY TEST
   - Set config to `nomic-embed-text`
   - Run installer, select option 1 (bge-m3)
   - Verify config updated to `bge-m3`
   - Verify dimension updated to `1024`

4. **Test 4: Custom API key preservation**
   - Set config with custom LiteLLM API key
   - Run installer (Ollama mode)
   - Verify API key still in config

5. **Test 5: Dimension mismatch warning**
   - Set config to `nomic-embed-text` (768 dims)
   - Run installer, select bge-m3 (1024 dims)
   - Verify warning appears about dimension mismatch

## Implementation Notes

**Key insight:** The problem is ORDER, not LOGIC.

Current flow:
```
1. Prompt user → EMBEDDING_MODEL="bge-m3"
2. Load config → CODETECT_EMBEDDING_MODEL="nomic-embed-text"
3. Overwrite → EMBEDDING_MODEL="nomic-embed-text" ❌
```

Fixed flow:
```
1. Load config → OLD_EMBEDDING_MODEL="nomic-embed-text"
2. Prompt user (default=OLD) → User selects bge-m3
3. Use selection → EMBEDDING_MODEL="bge-m3" ✅
```

**Specific changes:**

1. Move line 1128 (`if load_existing_config; then`) to before line 297 (start of interactive prompts)

2. Change model prompt to detect old model and pre-select it:
```bash
# Detect which option corresponds to old model
DEFAULT_CHOICE=1
if [[ "$OLD_EMBEDDING_MODEL" == "bge-m3" ]]; then DEFAULT_CHOICE=1; fi
if [[ "$OLD_EMBEDDING_MODEL" == "snowflake-arctic-embed" ]]; then DEFAULT_CHOICE=2; fi
if [[ "$OLD_EMBEDDING_MODEL" == "jina-embeddings-v3" ]]; then DEFAULT_CHOICE=3; fi
if [[ "$OLD_EMBEDDING_MODEL" == "nomic-embed-text" ]]; then DEFAULT_CHOICE=4; fi

read -p "$(prompt "Your choice [$DEFAULT_CHOICE]")" MODEL_CHOICE
MODEL_CHOICE=${MODEL_CHOICE:-$DEFAULT_CHOICE}
```

3. Remove lines 1134-1140 (the overwrite logic)

4. For API keys (no prompt), still preserve:
```bash
# These have no prompts, so always preserve
LITELLM_API_KEY="${LITELLM_API_KEY:-$OLD_LITELLM_API_KEY}"
```

## User Impact

**Immediate fix for user:**
Until this is fixed, user can manually update config:
```bash
# Edit config
vi ~/.config/codetect/config.env

# Change:
export CODETECT_EMBEDDING_MODEL="bge-m3"
export CODETECT_VECTOR_DIMENSIONS="1024"

# Re-embed with new model
cd /path/to/project
codetect embed --force
```

**After fix:**
- Reinstalling and selecting new model will work correctly
- Config preservation will still work for API keys and URLs
- Users can confidently update their embedding model via installer
