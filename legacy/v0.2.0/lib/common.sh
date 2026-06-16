#!/usr/bin/env bash
# Xeme OS — Common functions
# Sourced by all xeme subcommands

set -euo pipefail

# Paths
XEME_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
XEME_CONFIG="$XEME_ROOT/config/xeme.yaml"
XEME_ENRICH_CONFIG="$XEME_ROOT/config/enrich.jsonc"
XEME_ICP_CONFIG="$XEME_ROOT/config/icp.yaml"

# Load config values (handles 1-2 level nested YAML)
# Usage: _cfg_get key [default]
# Examples:
#   _cfg_get deepline                # tools.deepline (searches whole file)
#   _cfg_get tools.deepline          # explicit path
#   _cfg_get icp.scoring.title_cmo   # nested
_cfg_get() {
  local key="$1"
  local default="${2:-}"
  local result
  result=$(awk -F': ' -v k="$key" '
    {
      gsub(/^[ \t]+/, "", $1)  # trim leading whitespace
      gsub(/^[ \t]+/, "", $2)  # trim leading whitespace on value
      if ($1 == k) { print $2; exit }
    }' "$XEME_CONFIG" 2>/dev/null | tr -d '"' | tr -d "'" | head -1)
  echo "${result:-$default}"
}

_cfg_get_list() {
  local prefix="$1"
  awk -F': ' -v p="$prefix" '$1 ~ "^"p {sub(/^[^[]*\[/, ""); sub(/\][[:space:]]*$/, ""); print}' "$XEME_CONFIG"
}

# Logging
_ts() { date "+%Y-%m-%d %H:%M:%S"; }

_log() {
  local level="$1"; shift
  local color=""
  case "$level" in
    info)  color="\033[0;36m" ;;
    warn)  color="\033[0;33m" ;;
    error) color="\033[0;31m" ;;
    ok)    color="\033[0;32m" ;;
  esac
  echo -e "${color}[$(_ts)] [$level]\033[0m $*" >&2
}

info()  { _log info "$@"; }
warn()  { _log warn "$@"; }
error() { _log error "$@"; }
ok()    { _log ok "$@"; }

# CSV utilities
csv_header() {
  head -1 "$1"
}

csv_count_rows() {
  local file="$1"
  if [[ ! -f "$file" ]]; then echo 0; return; fi
  local total
  total=$(wc -l < "$file" | tr -d ' ')
  echo $((total - 1))  # minus header
}

# Tool checks
require_tool() {
  local tool="$1"
  local path="${2:-}"
  if ! command -v "$tool" &>/dev/null; then
    error "Required tool '$tool' not found in PATH"
    [[ -n "$path" ]] && error "  Tried: $path"
    return 1
  fi
}

# Ensure workspace dir
ensure_workspace() {
  local ws="$XEME_ROOT/workspace"
  mkdir -p "$ws" "$XEME_ROOT/logs"
  echo "$ws"
}

# Generate a timestamped filename
ts_filename() {
  local prefix="${1:-xeme}"
  local ext="${2:-csv}"
  local ts
  ts=$(date "+%Y-%m-%d_%H%M%S")
  echo "${prefix}_${ts}.${ext}"
}

# Pretty section header
section() {
  echo ""
  echo -e "\033[1;34m═══════════════════════════════════════════════════════════════\033[0m"
  echo -e "\033[1;34m  $*\033[0m"
  echo -e "\033[1;34m═══════════════════════════════════════════════════════════════\033[0m"
  echo ""
}
