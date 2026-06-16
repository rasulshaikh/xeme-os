#!/bin/bash
# Xeme OS — 1-click installer
# Usage: curl -fsSL https://xeme.dev/install.sh | bash
#
# Installs the xeme CLI + all sub-binaries to ~/.xeme/bin/
# Sets up ~/.xeme/ config, ledger, and workspace.

set -e

# ── Config ────────────────────────────────────────────────
XEME_VERSION="${XEME_VERSION:-latest}"
XEME_HOME="${XEME_HOME:-$HOME/.xeme}"
XEME_BIN="$XEME_HOME/bin"
XEME_REPO="xeme-os/xeme"
GITHUB_BASE="https://github.com/$XEME_REPO/releases"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# ── Banner ────────────────────────────────────────────────
banner() {
  cat << 'EOF'
   ██╗  ██╗███████╗███╗   ███╗███████╗     ██████╗ ███████╗
   ╚██╗██╔╝██╔════╝████╗ ████║██╔════╝    ██╔═══██╗██╔════╝
    ╚███╔╝ █████╗  ██╔████╔██║█████╗      ██║   ██║███████╗
    ██╔██╗ ██╔══╝  ██║╚██╔╝██║██╔══╝      ██║   ██║╚════██║
   ██╔╝ ██╗███████╗██║ ╚═╝ ██║███████╗    ╚██████╔╝███████║
   ╚═╝  ╚═╝╚══════╝╚═╝     ╚═╝╚══════╝     ╚═════╝ ╚══════╝
                    AI-Native GTM Operating System
EOF
  echo ""
  echo -e "${CYAN}  The last GTM stack you'll ever pay for.${NC}"
  echo -e "${CYAN}  Self-hosted · MIT · 1-click install${NC}"
  echo ""
}

# ── Helpers ───────────────────────────────────────────────
log()   { echo -e "${BLUE}▸${NC} $1"; }
ok()    { echo -e "${GREEN}✓${NC} $1"; }
warn()  { echo -e "${YELLOW}!${NC} $1"; }
err()   { echo -e "${RED}✗${NC} $1" >&2; }
fatal() { err "$1"; exit 1; }

detect_platform() {
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  ARCH=$(uname -m)
  case "$OS" in
    linux)  OS=linux ;;
    darwin) OS=darwin ;;
    mingw*|cygwin*|msys*) OS=windows ;;
  esac
  case "$ARCH" in
    x86_64|amd64)  ARCH=amd64 ;;
    arm64|aarch64) ARCH=arm64 ;;
    *) fatal "Unsupported architecture: $ARCH" ;;
  esac
  PLATFORM="${OS}_${ARCH}"
  log "Detected platform: $PLATFORM"
}

check_dependencies() {
  for cmd in curl tar; do
    command -v "$cmd" >/dev/null 2>&1 || fatal "Required: $cmd"
  done
  ok "Dependencies: curl + tar found"
}

# ── Install paths ─────────────────────────────────────────
mkdir_paths() {
  mkdir -p "$XEME_BIN"
  mkdir -p "$XEME_HOME/config"
  mkdir -p "$XEME_HOME/ledger"
  mkdir -p "$XEME_HOME/workspace"
  mkdir -p "$XEME_HOME/logs"
  mkdir -p "$XEME_HOME/cache"
  ok "Created $XEME_HOME/"
}

# ── Download + extract ─────────────────────────────────────
download_binaries() {
  if [ "$XEME_VERSION" = "latest" ]; then
    DOWNLOAD_URL="$GITHUB_BASE/latest/download/xeme_${PLATFORM}.tar.gz"
  else
    DOWNLOAD_URL="$GITHUB_BASE/download/v${XEME_VERSION}/xeme_${PLATFORM}.tar.gz"
  fi

  TMPDIR=$(mktemp -d)
  trap "rm -rf $TMPDIR" EXIT

  log "Downloading $DOWNLOAD_URL"
  if ! curl -fsSL --retry 3 -o "$TMPDIR/xeme.tar.gz" "$DOWNLOAD_URL" 2>/dev/null; then
    # Fallback: build from source (only if Go is installed)
    if command -v go >/dev/null 2>&1; then
      warn "No release for $PLATFORM. Building from source..."
      build_from_source
      return
    fi
    fatal "Could not download release and Go is not installed. Try: brew install go && retry"
  fi
  ok "Downloaded release"

  log "Extracting to $XEME_BIN/"
  tar -xzf "$TMPDIR/xeme.tar.gz" -C "$XEME_BIN"
  chmod +x "$XEME_BIN"/*
  ok "Installed: $(ls $XEME_BIN | tr '\n' ' ')"
}

build_from_source() {
  if ! command -v git >/dev/null 2>&1; then
    fatal "git is required to build from source"
  fi
  TMPDIR=$(mktemp -d)
  trap "rm -rf $TMPDIR" EXIT
  log "Cloning xeme-os/xeme"
  git clone --depth 1 https://github.com/$XEME_REPO.git "$TMPDIR/xeme"
  cd "$TMPDIR/xeme"
  log "Building all binaries (this may take 2-3 min)..."
  mkdir -p "$XEME_BIN"
  for cmd in xeme xeme-os xeme-mcp xeme-campaigns xeme-ledger-server xeme-workflows; do
    go build -o "$XEME_BIN/$cmd" "./cmd/$(echo $cmd | sed 's/^xeme-//; s/^xeme$/xeme/')" 2>/dev/null \
      || go build -o "$XEME_BIN/$cmd" "./cmd/${cmd/xeme-/}"
  done
  chmod +x "$XEME_BIN"/*
  ok "Built from source"
}

# ── Shell PATH ───────────────────────────────────────────
setup_path() {
  PROFILE=""
  case "$SHELL" in
    */zsh)  PROFILE="$HOME/.zshrc" ;;
    */bash) PROFILE="$HOME/.bashrc" ;;
    */fish) PROFILE="$HOME/.config/fish/config.fish" ;;
    *)      PROFILE="$HOME/.profile" ;;
  esac

  EXPORT_LINE="export PATH=\"$XEME_BIN:\$PATH\""

  if [ -n "$PROFILE" ] && [ -f "$PROFILE" ] && ! grep -q "$XEME_BIN" "$PROFILE"; then
    echo "" >> "$PROFILE"
    echo "# Xeme OS" >> "$PROFILE"
    echo "$EXPORT_LINE" >> "$PROFILE"
    ok "Added $XEME_BIN to PATH in $PROFILE"
    warn "Run: source $PROFILE  (or open a new shell)"
  fi
}

# ── Default config ───────────────────────────────────────
write_default_config() {
  cat > "$XEME_HOME/config/xeme.yaml" << 'YAML'
# Xeme OS — default config
# Edit to add your API keys for premium providers.

# Premium enrichment providers (optional)
enrich:
  providers:
    - local       # DNS/MX pattern matching (always free)
    - upstream    # Xeme upstream (free tier)
    # - moltsets  # requires XEME_MOLTSETS_API_KEY, $27/mo
    # - theirstack # requires XEME_THEIRSTACK_API_KEY, $49/mo

# BuiltWith (free, no key)
intel:
  builtwith:
    enabled: true

# Signal scraper (free, no key)
scraper:
  enabled: true
  sources: [hackernews, remoteok]

# AEO/GEO tracking
aeo:
  enabled: false  # set XEME_AEO_BRAND to enable
YAML
  ok "Wrote default config: $XEME_HOME/config/xeme.yaml"
}

# ── Init message ─────────────────────────────────────────
init_message() {
  cat << 'INIT'

  ┌────────────────────────────────────────────────────────────┐
  │                                                            │
  │   ✓ Xeme OS installed successfully                        │
  │                                                            │
  │   Next steps:                                              │
  │     1. Open a new shell (or source your profile)          │
  │     2. Run: xeme init                                     │
  │     3. Run: xeme serve  (starts local dashboard)         │
  │                                                            │
  │   Quick commands:                                          │
  │     xeme --help        # see all commands                 │
  │     xeme version       # check version                    │
  │     xeme status        # health check                     │
  │     xeme enrich        # enrich a CSV                     │
  │     xeme sequence      # multichannel outreach            │
  │     xeme install deepline clay twenty yalc scrapling      │
  │                                                            │
  │   Optional premium providers:                              │
  │     export XEME_MOLTSETS_API_KEY="ms_..."                │
  │     export XEME_THEIRSTACK_API_KEY="..."                  │
  │                                                            │
  │   Docs:    https://github.com/xeme-os/xeme               │
  │   Discord: https://discord.gg/xeme                        │
  │                                                            │
  └────────────────────────────────────────────────────────────┘
INIT
}

# ── Main ──────────────────────────────────────────────────
main() {
  banner
  detect_platform
  check_dependencies
  mkdir_paths
  download_binaries
  setup_path
  write_default_config
  init_message
}

main "$@"
