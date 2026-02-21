#!/bin/bash
#
# codetect - Global wrapper script for codetect MCP server
#
# Commands:
#   mcp            Start MCP server (used by .mcp.json)
#   index          Index symbols in current directory
#   embed          Generate embeddings for semantic search
#   init           Initialize codetect in current directory
#   doctor         Check installation and dependencies
#   reconfigure    Change database, embedding, and model settings
#   stats          Show index statistics
#   migrate        Discover existing indexes and register them
#   daemon         Manage background indexing daemon
#   registry       Manage project registry
#   version        Show installed version
#   help           Show this help message
#

set -e

# Installation paths
INSTALL_PREFIX="${CODETECT_PREFIX:-$HOME/.local}"
BIN_DIR="$INSTALL_PREFIX/bin"
SHARE_DIR="$INSTALL_PREFIX/share/codetect"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/codetect"
CONFIG_FILE="$CONFIG_DIR/config.env"
REGISTRY_FILE="$CONFIG_DIR/registry.json"
PID_FILE="$CONFIG_DIR/daemon.pid"
LOG_FILE="$CONFIG_DIR/daemon.log"
SOCKET_PATH="/tmp/codetect-$(id -u).sock"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

#
# Helper functions
#
success() {
    echo -e "${GREEN}✓${NC} $1"
}

warn() {
    echo -e "${YELLOW}!${NC} $1"
}

error() {
    echo -e "${RED}✗${NC} $1"
}

info() {
    echo -e "  $1"
}

prompt() {
    echo -e "${BLUE}?${NC} $1"
}

#
# Load global config if exists
#
load_config() {
    if [[ -f "$CONFIG_FILE" ]]; then
        source "$CONFIG_FILE"
    fi
}

#
# Generate config.env file with unset preamble
# Uses variables: cfg_db_type, cfg_db_dsn, cfg_embedding_provider,
#   cfg_ollama_url, cfg_embedding_model, cfg_vector_dimensions,
#   cfg_litellm_url, cfg_litellm_api_key
#
generate_config_file() {
    local target_file="$1"

    cat > "$target_file" << 'UNSET_BLOCK'
# codetect configuration
# Clear all codetect variables (prevents stale values from previous configs)
unset CODETECT_DB_TYPE
unset CODETECT_DB_DSN
unset CODETECT_DB_PATH
unset CODETECT_EMBEDDING_PROVIDER
unset CODETECT_OLLAMA_URL
unset CODETECT_LITELLM_URL
unset CODETECT_LITELLM_API_KEY
unset CODETECT_EMBEDDING_MODEL
unset CODETECT_VECTOR_DIMENSIONS
UNSET_BLOCK

    # Database backend
    echo "" >> "$target_file"
    echo "# Database backend: sqlite or postgres" >> "$target_file"
    echo "export CODETECT_DB_TYPE=\"$cfg_db_type\"" >> "$target_file"

    if [[ "$cfg_db_type" == "postgres" && -n "$cfg_db_dsn" ]]; then
        echo "" >> "$target_file"
        echo "# PostgreSQL configuration" >> "$target_file"
        echo "export CODETECT_DB_DSN=\"$cfg_db_dsn\"" >> "$target_file"
    fi

    # Embedding provider
    echo "" >> "$target_file"
    echo "# Embedding provider: ollama, litellm, or off" >> "$target_file"
    echo "export CODETECT_EMBEDDING_PROVIDER=\"$cfg_embedding_provider\"" >> "$target_file"

    if [[ "$cfg_embedding_provider" == "ollama" ]]; then
        echo "" >> "$target_file"
        echo "# Ollama configuration" >> "$target_file"
        echo "export CODETECT_OLLAMA_URL=\"$cfg_ollama_url\"" >> "$target_file"
        echo "export CODETECT_EMBEDDING_MODEL=\"$cfg_embedding_model\"" >> "$target_file"
        echo "export CODETECT_VECTOR_DIMENSIONS=\"$cfg_vector_dimensions\"" >> "$target_file"
    elif [[ "$cfg_embedding_provider" == "litellm" ]]; then
        echo "" >> "$target_file"
        echo "# LiteLLM configuration" >> "$target_file"
        echo "export CODETECT_LITELLM_URL=\"$cfg_litellm_url\"" >> "$target_file"
        echo "export CODETECT_LITELLM_API_KEY=\"$cfg_litellm_api_key\"" >> "$target_file"
        echo "export CODETECT_EMBEDDING_MODEL=\"$cfg_embedding_model\"" >> "$target_file"
    fi
}

#
# Commands
#

cmd_mcp() {
    load_config
    exec "$BIN_DIR/codetect-mcp" "$@"
}

cmd_index() {
    load_config
    local target_dir="."
    local extra_flags=()

    # Separate the target directory from flags
    for arg in "$@"; do
        case "$arg" in
            -*)
                extra_flags+=("$arg")
                ;;
            *)
                target_dir="$arg"
                ;;
        esac
    done

    echo -e "${CYAN}Indexing symbols in: ${target_dir}${NC}"
    "$BIN_DIR/codetect-index" index "$target_dir" "${extra_flags[@]}"
    success "Symbol indexing complete"
}

cmd_embed() {
    load_config

    if [[ "$CODETECT_EMBEDDING_PROVIDER" == "off" ]]; then
        warn "Embedding provider is disabled"
        info "Set CODETECT_EMBEDDING_PROVIDER in $CONFIG_FILE to enable"
        return 0
    fi

    echo -e "${CYAN}Generating embeddings...${NC}"
    "$BIN_DIR/codetect-index" embed "$@"
    success "Embedding complete"
}

cmd_init() {
    local force=false

    if [[ "$1" == "-f" || "$1" == "--force" ]]; then
        force=true
    fi

    # If --force, overwrite completely (original behavior)
    if [[ "$force" == "true" ]]; then
        warn "Force flag set - overwriting .mcp.json"
        cat > .mcp.json << 'EOF'
{
  "mcpServers": {
    "codetect": {
      "command": "codetect",
      "args": ["mcp"]
    }
  }
}
EOF
        success "Created .mcp.json (overwrote existing)"

    # If file doesn't exist, create it
    elif [[ ! -f ".mcp.json" ]]; then
        cat > .mcp.json << 'EOF'
{
  "mcpServers": {
    "codetect": {
      "command": "codetect",
      "args": ["mcp"]
    }
  }
}
EOF
        success "Created .mcp.json"

    # File exists - merge codetect entry
    else
        info ".mcp.json exists - merging codetect server entry"

        # Check for Python3
        if ! command -v python3 &> /dev/null; then
            error "python3 not found - cannot merge .mcp.json"
            info "Use 'codetect init --force' to overwrite instead"
            return 1
        fi

        # Use Python to merge JSON
        local python_output
        python_output=$(python3 << 'PYTHON_SCRIPT' 2>&1
import json
import sys

try:
    # Read existing file
    with open('.mcp.json', 'r') as f:
        data = json.load(f)

    # Ensure mcpServers key exists
    if 'mcpServers' not in data:
        data['mcpServers'] = {}

    # Check if codetect already exists
    codetect_exists = 'codetect' in data['mcpServers']

    # Add or update codetect entry
    data['mcpServers']['codetect'] = {
        'command': 'codetect',
        'args': ['mcp']
    }

    # Write back with formatting
    with open('.mcp.json', 'w') as f:
        json.dump(data, f, indent=2)
        f.write('\n')  # Add trailing newline

    # Print status for shell script
    if codetect_exists:
        print("UPDATED", end='')
    else:
        print("ADDED", end='')

except json.JSONDecodeError as e:
    print(f"ERROR: Invalid JSON - {e}", file=sys.stderr)
    sys.exit(1)
except Exception as e:
    print(f"ERROR: {e}", file=sys.stderr)
    sys.exit(1)
PYTHON_SCRIPT
)
        local python_exit=$?

        if [[ $python_exit -ne 0 ]]; then
            error "Failed to merge .mcp.json"
            error "$python_output"
            info "Your .mcp.json may have invalid JSON syntax"
            info "Use 'codetect init --force' to overwrite, or fix the file manually"
            return 1
        fi

        if [[ "$python_output" == "UPDATED" ]]; then
            success "Updated codetect entry in .mcp.json"
        else
            success "Added codetect to .mcp.json (preserved existing servers)"
        fi
    fi

    # Register in central registry
    registry_add "$(pwd)" 2>/dev/null || true

    info "Run 'codetect index' to index this codebase"
}

cmd_doctor() {
    load_config

    echo -e "${CYAN}codetect Installation Check${NC}"
    echo ""

    # Check binaries
    echo "Binaries:"
    if [[ -x "$BIN_DIR/codetect-mcp" ]]; then
        success "codetect-mcp: $BIN_DIR/codetect-mcp"
    else
        error "codetect-mcp not found at $BIN_DIR/codetect-mcp"
    fi

    if [[ -x "$BIN_DIR/codetect-index" ]]; then
        success "codetect-index: $BIN_DIR/codetect-index"
    else
        error "codetect-index not found at $BIN_DIR/codetect-index"
    fi
    echo ""

    # Check dependencies
    echo "Dependencies:"
    if command -v rg &> /dev/null; then
        RG_VERSION=$(rg --version | head -1)
        success "ripgrep: $RG_VERSION"
    else
        error "ripgrep (rg) not found"
    fi

    if command -v sg &> /dev/null; then
        ASTGREP_VERSION=$(sg --version 2>/dev/null | head -1)
        success "ast-grep: $ASTGREP_VERSION (primary)"
    else
        warn "ast-grep (sg) not found — install for best symbol indexing"
        info "  brew install ast-grep"
    fi

    if command -v ctags &> /dev/null && ctags --version 2>&1 | grep -q "Universal Ctags"; then
        CTAGS_VERSION=$(ctags --version | head -1 | cut -d',' -f1)
        success "ctags: $CTAGS_VERSION (fallback)"
    else
        warn "universal-ctags not found (fallback for unsupported languages)"
    fi
    echo ""

    # Check embedding provider
    echo "Embedding Provider:"
    local provider="${CODETECT_EMBEDDING_PROVIDER:-ollama}"

    case "$provider" in
        ollama)
            echo -e "  Provider: ${GREEN}Ollama${NC}"
            local ollama_url="${CODETECT_OLLAMA_URL:-http://localhost:11434}"
            if curl -s "$ollama_url/api/tags" &> /dev/null; then
                success "Ollama is running at $ollama_url"
                local model="${CODETECT_EMBEDDING_MODEL:-nomic-embed-text}"
                if curl -s "$ollama_url/api/tags" | grep -q "$model"; then
                    success "Model '$model' is available"
                else
                    warn "Model '$model' not found"
                fi
            else
                warn "Ollama not running at $ollama_url"
            fi
            ;;
        litellm)
            echo -e "  Provider: ${GREEN}LiteLLM${NC}"
            local litellm_url="${CODETECT_LITELLM_URL:-http://localhost:4000}"
            if curl -s "$litellm_url/health" &> /dev/null; then
                success "LiteLLM is running at $litellm_url"
            else
                warn "LiteLLM not running at $litellm_url"
            fi
            ;;
        off)
            echo -e "  Provider: ${YELLOW}Disabled${NC}"
            info "Semantic search is disabled"
            ;;
        *)
            error "Unknown provider: $provider"
            ;;
    esac
    echo ""

    # Check config
    echo "Configuration:"
    if [[ -f "$CONFIG_FILE" ]]; then
        success "Config file: $CONFIG_FILE"
    else
        info "No global config (using defaults)"
        info "Create with: mkdir -p $CONFIG_DIR && touch $CONFIG_FILE"
    fi
    echo ""

    # Show active configuration values
    echo "Configuration Values:"
    local config_vars=(
        "CODETECT_DB_TYPE"
        "CODETECT_DB_DSN"
        "CODETECT_DB_PATH"
        "CODETECT_EMBEDDING_PROVIDER"
        "CODETECT_EMBEDDING_MODEL"
        "CODETECT_VECTOR_DIMENSIONS"
        "CODETECT_OLLAMA_URL"
        "CODETECT_LITELLM_URL"
        "CODETECT_LITELLM_API_KEY"
    )
    local has_config=false
    for var in "${config_vars[@]}"; do
        local val="${!var}"
        if [[ -n "$val" ]]; then
            has_config=true
            # Mask API keys
            if [[ "$var" == *"API_KEY"* ]]; then
                info "$var=****${val: -4}"
            # Mask password in DSN
            elif [[ "$var" == *"DSN"* && "$val" == *"@"* ]]; then
                local masked=$(echo "$val" | sed 's|://[^:]*:[^@]*@|://***:***@|')
                info "$var=$masked"
            else
                info "$var=$val"
            fi
        fi
    done
    if [[ "$has_config" == "false" ]]; then
        info "(no codetect variables set)"
    fi
    echo ""

    # Check current directory
    echo "Current Directory:"
    if [[ -f ".mcp.json" ]]; then
        success ".mcp.json exists"
    else
        info "No .mcp.json (run 'codetect init' to create)"
    fi

    # Check for centralized data directory
    local data_dir="$HOME/.codetect/projects"
    if [[ -d "$data_dir" ]]; then
        local project_count=$(ls -1d "$data_dir"/*/ 2>/dev/null | wc -l | tr -d ' ')
        success "Centralized storage: $data_dir ($project_count projects)"
    else
        info "No centralized storage (run 'codetect index' to create)"
    fi

    # Check if current project has an index
    local project_name=$(basename "$(pwd)")
    local found_index=false
    if [[ -d "$data_dir" ]]; then
        for dir in "$data_dir"/"${project_name}"-*/; do
            if [[ -d "$dir" ]]; then
                found_index=true
                if [[ -f "${dir}index.db" ]]; then
                    local size=$(du -h "${dir}index.db" | cut -f1)
                    success "Index found: $(basename "$dir") (${size})"
                else
                    success "Data dir found: $(basename "$dir")"
                fi
                break
            fi
        done
    fi
    if [[ "$found_index" == "false" ]]; then
        info "No index for current project (run 'codetect index' to create)"
    fi
    echo ""

    # Check ignore files
    echo "Ignore Files:"
    local xdg_ignore="$CONFIG_DIR/ignore"
    local legacy_ignore="$HOME/.codetectignore"
    local project_ignore=".codetectignore"

    if [[ -f "$xdg_ignore" ]]; then
        success "Global:  $xdg_ignore"
    else
        info "○ Global:  $xdg_ignore (not set)"
    fi

    if [[ -f "$legacy_ignore" ]]; then
        warn "Legacy:  $legacy_ignore [deprecated → move to $xdg_ignore]"
    fi

    if [[ -f "$project_ignore" ]]; then
        success "Project: $(pwd)/$project_ignore"
    else
        info "○ Project: .codetectignore (not set)"
    fi
}

cmd_stats() {
    load_config

    # Use codetect-index stats (handles centralized path resolution)
    "$BIN_DIR/codetect-index" stats "$@"
}

#
# Daemon commands
#
cmd_daemon() {
    local subcmd="${1:-status}"
    shift || true

    case "$subcmd" in
        start)
            daemon_start "$@"
            ;;
        stop)
            daemon_stop
            ;;
        status)
            daemon_status
            ;;
        logs)
            daemon_logs "$@"
            ;;
        help|--help|-h)
            daemon_help
            ;;
        *)
            error "Unknown daemon command: $subcmd"
            daemon_help
            exit 1
            ;;
    esac
}

daemon_start() {
    # Check if already running
    if [[ -S "$SOCKET_PATH" ]]; then
        if daemon_is_running; then
            warn "Daemon is already running"
            return 0
        else
            # Stale socket
            rm -f "$SOCKET_PATH"
        fi
    fi

    # Ensure config directory exists
    mkdir -p "$CONFIG_DIR"

    # Check if daemon binary exists
    if [[ ! -x "$BIN_DIR/codetect-daemon" ]]; then
        error "codetect-daemon not found at $BIN_DIR/codetect-daemon"
        info "Run 'make install' from the codetect directory"
        return 1
    fi

    echo -e "${CYAN}Starting daemon...${NC}"

    # Start daemon in background
    nohup "$BIN_DIR/codetect-daemon" start --foreground >> "$LOG_FILE" 2>&1 &
    local pid=$!

    # Wait for socket to be ready
    local tries=0
    while [[ ! -S "$SOCKET_PATH" && $tries -lt 20 ]]; do
        sleep 0.1
        ((tries++))
    done

    if [[ -S "$SOCKET_PATH" ]]; then
        success "Daemon started (PID: $pid)"
        info "Log file: $LOG_FILE"
    else
        error "Daemon failed to start"
        info "Check logs: codetect daemon logs"
        return 1
    fi
}

daemon_stop() {
    if ! daemon_is_running; then
        warn "Daemon is not running"
        return 0
    fi

    echo -e "${CYAN}Stopping daemon...${NC}"

    # Send stop command via socket
    echo '{"action":"stop"}' | nc -U "$SOCKET_PATH" 2>/dev/null || true

    # Wait for socket to be removed
    local tries=0
    while [[ -S "$SOCKET_PATH" && $tries -lt 20 ]]; do
        sleep 0.1
        ((tries++))
    done

    if [[ ! -S "$SOCKET_PATH" ]]; then
        success "Daemon stopped"
    else
        # Force kill if still running
        if [[ -f "$PID_FILE" ]]; then
            local pid=$(cat "$PID_FILE")
            kill -9 "$pid" 2>/dev/null || true
            rm -f "$PID_FILE" "$SOCKET_PATH"
        fi
        success "Daemon stopped (forced)"
    fi
}

daemon_status() {
    if daemon_is_running; then
        echo -e "${GREEN}Daemon is running${NC}"
        echo ""

        # Get status from daemon
        local response=$(echo '{"action":"status"}' | nc -U "$SOCKET_PATH" 2>/dev/null)
        if [[ -n "$response" ]]; then
            echo "$response" | python3 -m json.tool 2>/dev/null || echo "$response"
        fi
    else
        echo -e "${YELLOW}Daemon is not running${NC}"
        info "Start with: codetect daemon start"
    fi
}

daemon_logs() {
    local lines="${1:-50}"

    if [[ ! -f "$LOG_FILE" ]]; then
        info "No log file found"
        return 0
    fi

    echo -e "${CYAN}Daemon logs (last $lines lines):${NC}"
    echo ""
    tail -n "$lines" "$LOG_FILE"
}

daemon_is_running() {
    [[ -S "$SOCKET_PATH" ]] && echo '{"action":"status"}' | nc -U "$SOCKET_PATH" &>/dev/null
}

daemon_help() {
    echo "Usage: codetect daemon <command>"
    echo ""
    echo "Commands:"
    echo "  start       Start the background daemon"
    echo "  stop        Stop the daemon"
    echo "  status      Show daemon status"
    echo "  logs [n]    Show last n lines of logs (default: 50)"
    echo "  help        Show this help"
}

#
# Registry commands
#
cmd_registry() {
    local subcmd="${1:-list}"
    shift || true

    case "$subcmd" in
        list)
            registry_list
            ;;
        add)
            registry_add "$@"
            ;;
        remove)
            registry_remove "$@"
            ;;
        stats)
            registry_stats
            ;;
        help|--help|-h)
            registry_help
            ;;
        *)
            error "Unknown registry command: $subcmd"
            registry_help
            exit 1
            ;;
    esac
}

registry_list() {
    if [[ ! -f "$REGISTRY_FILE" ]]; then
        info "No projects registered"
        info "Run 'codetect init' in a project to register it"
        return 0
    fi

    echo -e "${CYAN}Registered Projects${NC}"
    echo ""

    # Parse JSON and display projects
    python3 -c "
import json
import sys
from datetime import datetime

try:
    with open('$REGISTRY_FILE') as f:
        data = json.load(f)

    projects = data.get('projects', [])
    if not projects:
        print('  No projects registered')
        sys.exit(0)

    for p in projects:
        path = p.get('path', 'unknown')
        name = p.get('name', 'unknown')
        watch = '✓' if p.get('watch_enabled') else '○'
        stats = p.get('index_stats', {})
        symbols = stats.get('symbols', 0)
        embeddings = stats.get('embeddings', 0)

        last_indexed = p.get('last_indexed')
        if last_indexed:
            dt = datetime.fromisoformat(last_indexed.replace('Z', '+00:00'))
            last_indexed = dt.strftime('%Y-%m-%d %H:%M')
        else:
            last_indexed = 'never'

        print(f'  {watch} {name}')
        print(f'    Path: {path}')
        print(f'    Symbols: {symbols}, Embeddings: {embeddings}')
        print(f'    Last indexed: {last_indexed}')
        print()
except FileNotFoundError:
    print('  No projects registered')
except Exception as e:
    print(f'  Error reading registry: {e}')
"
}

registry_add() {
    local path="${1:-.}"
    path=$(cd "$path" && pwd)

    if [[ ! -d "$path" ]]; then
        error "Directory not found: $path"
        return 1
    fi

    # Ensure config directory exists
    mkdir -p "$CONFIG_DIR"

    # Add to registry using Python (simple JSON manipulation)
    python3 -c "
import json
import os
from datetime import datetime

registry_file = '$REGISTRY_FILE'
project_path = '$path'
project_name = os.path.basename(project_path)

# Load or create registry
try:
    with open(registry_file) as f:
        data = json.load(f)
except FileNotFoundError:
    data = {'version': 1, 'projects': [], 'settings': {'auto_watch': True, 'debounce_ms': 500, 'max_projects': 50}}

# Check if already registered
for p in data['projects']:
    if p['path'] == project_path:
        print(f'Project already registered: {project_name}')
        exit(0)

# Add project
data['projects'].append({
    'path': project_path,
    'name': project_name,
    'added_at': datetime.utcnow().isoformat() + 'Z',
    'last_indexed': None,
    'index_stats': {'symbols': 0, 'embeddings': 0, 'db_size_bytes': 0},
    'watch_enabled': data['settings']['auto_watch']
})

with open(registry_file, 'w') as f:
    json.dump(data, f, indent=2)

print(f'Added project: {project_name}')
"
    success "Project registered"

    # If daemon is running, tell it to watch this project
    if daemon_is_running; then
        echo "{\"action\":\"add\",\"path\":\"$path\"}" | nc -U "$SOCKET_PATH" &>/dev/null || true
        info "Daemon will watch this project"
    fi
}

registry_remove() {
    local path="${1:-.}"
    path=$(cd "$path" 2>/dev/null && pwd) || path="$1"

    if [[ ! -f "$REGISTRY_FILE" ]]; then
        error "No registry found"
        return 1
    fi

    # Remove from registry using Python
    python3 -c "
import json
import os

registry_file = '$REGISTRY_FILE'
project_path = '$path'

with open(registry_file) as f:
    data = json.load(f)

original_len = len(data['projects'])
data['projects'] = [p for p in data['projects'] if p['path'] != project_path]

if len(data['projects']) == original_len:
    print(f'Project not found in registry: {project_path}')
    exit(1)

with open(registry_file, 'w') as f:
    json.dump(data, f, indent=2)

print(f'Removed project: {os.path.basename(project_path)}')
"
    success "Project removed"

    # If daemon is running, tell it to stop watching
    if daemon_is_running; then
        echo "{\"action\":\"remove\",\"path\":\"$path\"}" | nc -U "$SOCKET_PATH" &>/dev/null || true
    fi
}

registry_stats() {
    if [[ ! -f "$REGISTRY_FILE" ]]; then
        info "No projects registered"
        return 0
    fi

    echo -e "${CYAN}Registry Statistics${NC}"
    echo ""

    python3 -c "
import json

with open('$REGISTRY_FILE') as f:
    data = json.load(f)

projects = data.get('projects', [])
total_symbols = sum(p.get('index_stats', {}).get('symbols', 0) for p in projects)
total_embeddings = sum(p.get('index_stats', {}).get('embeddings', 0) for p in projects)
total_size = sum(p.get('index_stats', {}).get('db_size_bytes', 0) for p in projects)
watched = sum(1 for p in projects if p.get('watch_enabled'))

print(f'Total projects: {len(projects)}')
print(f'Watched projects: {watched}')
print(f'Total symbols: {total_symbols}')
print(f'Total embeddings: {total_embeddings}')
print(f'Total index size: {total_size / 1024 / 1024:.2f} MB')
"
}

registry_help() {
    echo "Usage: codetect registry <command>"
    echo ""
    echo "Commands:"
    echo "  list        List all registered projects"
    echo "  add [path]  Add project to registry (default: current directory)"
    echo "  remove <path>  Remove project from registry"
    echo "  stats       Show aggregate statistics"
    echo "  help        Show this help"
}

#
# Migrate command - discover existing indexes and register them
#
cmd_migrate() {
    local dry_run=false
    local search_paths=()

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --dry-run|-n)
                dry_run=true
                shift
                ;;
            --help|-h)
                migrate_help
                return 0
                ;;
            *)
                search_paths+=("$1")
                shift
                ;;
        esac
    done

    # Default search paths if none provided
    if [[ ${#search_paths[@]} -eq 0 ]]; then
        search_paths=(
            "$HOME/dev"
            "$HOME/projects"
            "$HOME/code"
            "$HOME/src"
            "$HOME/repos"
            "$HOME/workspace"
            "$HOME/work"
            "$HOME/Documents/dev"
            "$HOME/Documents/projects"
            "$HOME/Documents/code"
        )
    fi

    echo -e "${CYAN}Scanning for existing codetect indexes...${NC}"
    echo ""

    local found=()
    local already_registered=()
    local new_projects=()

    # Load existing registry paths for comparison
    local registered_paths=""
    if [[ -f "$REGISTRY_FILE" ]]; then
        registered_paths=$(python3 -c "
import json
try:
    with open('$REGISTRY_FILE') as f:
        data = json.load(f)
    for p in data.get('projects', []):
        print(p.get('path', ''))
except:
    pass
" 2>/dev/null)
    fi

    # Scan each search path
    for base_path in "${search_paths[@]}"; do
        if [[ ! -d "$base_path" ]]; then
            continue
        fi

        # Find all .codetect directories (max depth 3 to avoid deep recursion)
        while IFS= read -r index_dir; do
            if [[ -n "$index_dir" ]]; then
                # Get project directory (parent of .codetect)
                local project_dir=$(dirname "$index_dir")
                found+=("$project_dir")

                # Check if already registered
                if echo "$registered_paths" | grep -qF "$project_dir"; then
                    already_registered+=("$project_dir")
                else
                    new_projects+=("$project_dir")
                fi
            fi
        done < <(find "$base_path" -maxdepth 4 -type d -name ".codetect" 2>/dev/null)
    done

    # Report findings
    echo "Search paths scanned:"
    for path in "${search_paths[@]}"; do
        if [[ -d "$path" ]]; then
            info "$path"
        else
            info "$path ${YELLOW}(not found)${NC}"
        fi
    done
    echo ""

    if [[ ${#found[@]} -eq 0 ]]; then
        info "No existing indexes found"
        echo ""
        info "To index a project manually:"
        info "  cd /path/to/project"
        info "  codetect init && codetect index"
        return 0
    fi

    echo "Found ${#found[@]} project(s) with existing indexes:"
    echo ""

    # Show already registered
    if [[ ${#already_registered[@]} -gt 0 ]]; then
        echo -e "${GREEN}Already registered (${#already_registered[@]}):${NC}"
        for project in "${already_registered[@]}"; do
            info "✓ $(basename "$project") - $project"
        done
        echo ""
    fi

    # Show new projects to migrate
    if [[ ${#new_projects[@]} -gt 0 ]]; then
        echo -e "${YELLOW}New projects to register (${#new_projects[@]}):${NC}"
        for project in "${new_projects[@]}"; do
            info "○ $(basename "$project") - $project"
        done
        echo ""

        if [[ "$dry_run" == "true" ]]; then
            info "Dry run - no changes made"
            info "Run without --dry-run to register these projects"
        else
            echo -e "${CYAN}Registering new projects...${NC}"
            echo ""

            local success_count=0
            local fail_count=0

            for project in "${new_projects[@]}"; do
                if registry_add "$project" 2>/dev/null; then
                    success "Registered: $(basename "$project")"
                    ((success_count++))
                else
                    error "Failed to register: $(basename "$project")"
                    ((fail_count++))
                fi
            done

            echo ""
            success "Migration complete: $success_count registered, $fail_count failed"

            # Suggest starting daemon
            if [[ $success_count -gt 0 ]] && ! daemon_is_running; then
                echo ""
                info "Start the daemon to auto-reindex on file changes:"
                info "  codetect daemon start"
            fi
        fi
    else
        success "All found projects are already registered"
    fi
}

migrate_help() {
    echo "Usage: codetect migrate [options] [paths...]"
    echo ""
    echo "Scan for existing .codetect indexes and register them in the central registry."
    echo ""
    echo "Options:"
    echo "  --dry-run, -n   Show what would be registered without making changes"
    echo "  --help, -h      Show this help"
    echo ""
    echo "Arguments:"
    echo "  paths           Directories to scan (default: ~/dev, ~/projects, ~/code, etc.)"
    echo ""
    echo "Examples:"
    echo "  codetect migrate                    # Scan default locations"
    echo "  codetect migrate --dry-run         # Preview without registering"
    echo "  codetect migrate ~/work ~/personal # Scan specific directories"
}

#
# Reconfigure command
#
cmd_reconfigure() {
    load_config

    echo -e "${CYAN}codetect Configuration${NC}"
    echo "======================"
    echo ""

    # Read current values (with defaults)
    local old_db_type="${CODETECT_DB_TYPE:-sqlite}"
    local old_db_dsn="${CODETECT_DB_DSN:-}"
    local old_embedding_provider="${CODETECT_EMBEDDING_PROVIDER:-ollama}"
    local old_ollama_url="${CODETECT_OLLAMA_URL:-http://localhost:11434}"
    local old_embedding_model="${CODETECT_EMBEDDING_MODEL:-nomic-embed-text}"
    local old_vector_dimensions="${CODETECT_VECTOR_DIMENSIONS:-768}"
    local old_litellm_url="${CODETECT_LITELLM_URL:-http://localhost:4000}"
    local old_litellm_api_key="${CODETECT_LITELLM_API_KEY:-}"

    # --- Database backend ---
    echo -e "${CYAN}Database Backend${NC}"
    echo ""

    local db_default=1
    [[ "$old_db_type" == "postgres" ]] && db_default=2

    if [[ "$old_db_type" == "sqlite" ]]; then
        echo -e "  ${GREEN}1)${NC} SQLite (current)"
        echo -e "  ${GREEN}2)${NC} PostgreSQL"
    else
        echo -e "  ${GREEN}1)${NC} SQLite"
        echo -e "  ${GREEN}2)${NC} PostgreSQL (current)"
    fi
    echo ""
    read -p "$(prompt "Your choice [$db_default]: ")" db_choice
    db_choice=${db_choice:-$db_default}

    cfg_db_type="sqlite"
    cfg_db_dsn=""
    if [[ "$db_choice" == "2" ]]; then
        cfg_db_type="postgres"
        echo ""
        read -p "$(prompt "PostgreSQL DSN [$old_db_dsn]: ")" cfg_db_dsn
        cfg_db_dsn=${cfg_db_dsn:-$old_db_dsn}
    fi
    echo ""

    # --- Embedding provider ---
    echo -e "${CYAN}Embedding Provider${NC}"
    echo ""

    local emb_default=1
    [[ "$old_embedding_provider" == "litellm" ]] && emb_default=2
    [[ "$old_embedding_provider" == "off" ]] && emb_default=3

    if [[ "$old_embedding_provider" == "ollama" ]]; then
        echo -e "  ${GREEN}1)${NC} Ollama (current)"
    else
        echo -e "  ${GREEN}1)${NC} Ollama"
    fi
    if [[ "$old_embedding_provider" == "litellm" ]]; then
        echo -e "  ${GREEN}2)${NC} LiteLLM (current)"
    else
        echo -e "  ${GREEN}2)${NC} LiteLLM"
    fi
    if [[ "$old_embedding_provider" == "off" ]]; then
        echo -e "  ${GREEN}3)${NC} Off (current)"
    else
        echo -e "  ${GREEN}3)${NC} Off (disable semantic search)"
    fi
    echo ""
    read -p "$(prompt "Your choice [$emb_default]: ")" emb_choice
    emb_choice=${emb_choice:-$emb_default}

    cfg_embedding_provider="ollama"
    cfg_ollama_url="$old_ollama_url"
    cfg_embedding_model="$old_embedding_model"
    cfg_vector_dimensions="$old_vector_dimensions"
    cfg_litellm_url="$old_litellm_url"
    cfg_litellm_api_key="$old_litellm_api_key"

    case "$emb_choice" in
        1)
            cfg_embedding_provider="ollama"
            echo ""

            # Model selection menu
            echo -e "${CYAN}Embedding Model${NC}"
            echo ""

            local model_default=1
            case "$old_embedding_model" in
                bge-m3) model_default=1 ;;
                snowflake-arctic-embed*) model_default=2 ;;
                jina-embeddings-v3) model_default=3 ;;
                nomic-embed-text) model_default=4 ;;
                *) model_default=5 ;;
            esac

            echo -e "  ${GREEN}1)${NC} bge-m3 (recommended, 1024 dims)$([ "$old_embedding_model" = "bge-m3" ] && echo " (current)")"
            echo -e "  ${GREEN}2)${NC} snowflake-arctic-embed-l-v2.0 (1024 dims)$([ "$old_embedding_model" = "snowflake-arctic-embed" ] && echo " (current)")"
            echo -e "  ${GREEN}3)${NC} jina-embeddings-v3 (1024 dims)$([ "$old_embedding_model" = "jina-embeddings-v3" ] && echo " (current)")"
            echo -e "  ${GREEN}4)${NC} nomic-embed-text (768 dims)$([ "$old_embedding_model" = "nomic-embed-text" ] && echo " (current)")"
            echo -e "  ${GREEN}5)${NC} Custom"
            echo ""
            read -p "$(prompt "Your choice [$model_default]: ")" model_choice
            model_choice=${model_choice:-$model_default}

            case "$model_choice" in
                1) cfg_embedding_model="bge-m3"; cfg_vector_dimensions="1024" ;;
                2) cfg_embedding_model="snowflake-arctic-embed"; cfg_vector_dimensions="1024" ;;
                3) cfg_embedding_model="jina-embeddings-v3"; cfg_vector_dimensions="1024" ;;
                4) cfg_embedding_model="nomic-embed-text"; cfg_vector_dimensions="768" ;;
                5)
                    read -p "$(prompt "Model name [$old_embedding_model]: ")" cfg_embedding_model
                    cfg_embedding_model=${cfg_embedding_model:-$old_embedding_model}
                    read -p "$(prompt "Vector dimensions [$old_vector_dimensions]: ")" cfg_vector_dimensions
                    cfg_vector_dimensions=${cfg_vector_dimensions:-$old_vector_dimensions}
                    ;;
            esac

            echo ""
            read -p "$(prompt "Ollama URL [$old_ollama_url]: ")" cfg_ollama_url
            cfg_ollama_url=${cfg_ollama_url:-$old_ollama_url}
            ;;

        2)
            cfg_embedding_provider="litellm"
            echo ""
            read -p "$(prompt "LiteLLM URL [$old_litellm_url]: ")" cfg_litellm_url
            cfg_litellm_url=${cfg_litellm_url:-$old_litellm_url}
            read -p "$(prompt "API Key [$old_litellm_api_key]: ")" cfg_litellm_api_key
            cfg_litellm_api_key=${cfg_litellm_api_key:-$old_litellm_api_key}
            read -p "$(prompt "Embedding model [$old_embedding_model]: ")" cfg_embedding_model
            cfg_embedding_model=${cfg_embedding_model:-$old_embedding_model}
            ;;

        3)
            cfg_embedding_provider="off"
            ;;
    esac

    echo ""

    # Detect changes
    local changes=()
    [[ "$old_db_type" != "$cfg_db_type" ]] && changes+=("Database: $old_db_type -> $cfg_db_type")
    [[ "$old_db_dsn" != "$cfg_db_dsn" && "$cfg_db_type" == "postgres" ]] && changes+=("DSN: updated")
    [[ "$old_embedding_provider" != "$cfg_embedding_provider" ]] && changes+=("Provider: $old_embedding_provider -> $cfg_embedding_provider")
    [[ "$old_embedding_model" != "$cfg_embedding_model" ]] && changes+=("Model: $old_embedding_model -> $cfg_embedding_model")
    [[ "$old_vector_dimensions" != "$cfg_vector_dimensions" ]] && changes+=("Dimensions: $old_vector_dimensions -> $cfg_vector_dimensions")
    [[ "$old_ollama_url" != "$cfg_ollama_url" && "$cfg_embedding_provider" == "ollama" ]] && changes+=("Ollama URL: $old_ollama_url -> $cfg_ollama_url")

    if [[ ${#changes[@]} -eq 0 ]]; then
        success "No changes — configuration unchanged"
        return 0
    fi

    # Create backup
    mkdir -p "$CONFIG_DIR"
    local backup_file="$CONFIG_FILE.backup.$(date +%Y%m%d-%H%M%S)"
    if [[ -f "$CONFIG_FILE" ]]; then
        cp "$CONFIG_FILE" "$backup_file"
    fi

    # Generate new config
    generate_config_file "$CONFIG_FILE"

    success "Configuration updated"
    if [[ -f "$backup_file" ]]; then
        info "Backup: $backup_file"
    fi
    echo ""

    info "Changes:"
    for change in "${changes[@]}"; do
        info "  $change"
    done
    echo ""

    # Warn on dimension change
    if [[ "$old_vector_dimensions" != "$cfg_vector_dimensions" ]]; then
        warn "Dimension change detected — re-embed your projects:"
        info "  codetect embed --force"
        echo ""
    fi

    info "Run 'source $CONFIG_FILE' to apply in current shell"
}

cmd_version() {
    local source_dir="${CODETECT_SOURCE:-$HOME/dev/codetect}"

    # Try git tag from source repo first (most accurate after `codetect update`)
    if [[ -d "$source_dir/.git" ]]; then
        local tag
        tag=$(git -C "$source_dir" describe --tags --exact-match 2>/dev/null) || true
        if [[ -n "$tag" ]]; then
            echo "codetect $tag"
            return 0
        fi
        # Fallback: short commit hash
        local commit
        commit=$(git -C "$source_dir" rev-parse --short HEAD 2>/dev/null) || true
        if [[ -n "$commit" ]]; then
            echo "codetect dev ($commit)"
            return 0
        fi
    fi

    # Fallback: ask codetect-index for its version
    if [[ -x "$BIN_DIR/codetect-index" ]]; then
        "$BIN_DIR/codetect-index" version
        return 0
    fi

    echo "codetect (unknown version)"
}

cmd_update() {
    local source_dir="${CODETECT_SOURCE:-$HOME/dev/codetect}"

    if [[ ! -f "$source_dir/scripts/update.sh" ]]; then
        error "Update script not found"
        info "Set CODETECT_SOURCE to the location of your codetect clone"
        info "Default: $source_dir"
        return 1
    fi

    exec "$source_dir/scripts/update.sh" "$@"
}

cmd_help() {
    echo -e "${CYAN}codetect${NC} - MCP server for codebase search & navigation"
    echo ""
    echo "Usage: codetect <command> [options]"
    echo ""
    echo "Commands:"
    echo "  mcp             Start MCP server (used by .mcp.json)"
    echo "  index [path]    Index symbols (default: current directory)"
    echo "  embed [path]    Generate embeddings for semantic search"
    echo "  init [-f]       Create .mcp.json in current directory"
    echo "  doctor          Check installation and dependencies"
    echo "  reconfigure     Change database, embedding, and model settings"
    echo "  stats           Show index statistics"
    echo "  migrate         Discover existing indexes and register them"
    echo "  daemon <cmd>    Manage background indexing daemon"
    echo "  registry <cmd>  Manage project registry"
    echo "  update          Update to latest version from GitHub"
    echo "  version         Show installed version"
    echo "  help            Show this help message"
    echo ""
    echo "Daemon Commands:"
    echo "  daemon start    Start background daemon"
    echo "  daemon stop     Stop daemon"
    echo "  daemon status   Show daemon status"
    echo "  daemon logs     View daemon logs"
    echo ""
    echo "Registry Commands:"
    echo "  registry list     List registered projects"
    echo "  registry add      Add current project"
    echo "  registry remove   Remove a project"
    echo "  registry stats    Show aggregate stats"
    echo ""
    echo "Configuration:"
    echo "  Global config: $CONFIG_FILE"
    echo "  Registry:      $REGISTRY_FILE"
    echo "  Per-project:   .mcp.json (created by 'init')"
    echo ""
    echo "Quick Start:"
    echo "  codetect init          # Initialize project"
    echo "  codetect index         # Index symbols"
    echo "  codetect migrate       # Register existing indexes"
    echo "  codetect daemon start  # Start background daemon"
    echo "  claude                    # Start Claude Code"
}

#
# Main
#
main() {
    local cmd="${1:-help}"
    shift || true

    case "$cmd" in
        mcp)
            cmd_mcp "$@"
            ;;
        index)
            cmd_index "$@"
            ;;
        embed)
            cmd_embed "$@"
            ;;
        init)
            cmd_init "$@"
            ;;
        doctor)
            cmd_doctor "$@"
            ;;
        reconfigure)
            cmd_reconfigure "$@"
            ;;
        stats)
            cmd_stats "$@"
            ;;
        migrate)
            cmd_migrate "$@"
            ;;
        daemon)
            cmd_daemon "$@"
            ;;
        registry)
            cmd_registry "$@"
            ;;
        update)
            cmd_update "$@"
            ;;
        version|--version|-V)
            cmd_version
            ;;
        help|--help|-h)
            cmd_help
            ;;
        *)
            error "Unknown command: $cmd"
            echo ""
            cmd_help
            exit 1
            ;;
    esac
}

main "$@"
