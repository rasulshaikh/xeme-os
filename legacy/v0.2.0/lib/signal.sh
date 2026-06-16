#!/usr/bin/env bash
# Xeme OS — LinkedIn Signal Engine (proprietary)
# Detects buyer-intent signals from LinkedIn post engagers, job changes,
# funding events, and tech-stack changes. Owned by Xeme OS.

source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

XEME_SIGNAL_BIN="$(_cfg_get signal 'xeme-signal')"

# Scrape engagers from a LinkedIn post
# Args: post_url output [account] [max_pages]
xeme_signal_scrape_post() {
  local url="$1"
  local out="$2"
  local account="${3:-}"
  local max_pages="${4:-10}"

  require_tool "$XEME_SIGNAL_BIN" || return 1

  if [[ -z "$url" ]]; then
    error "Provide a LinkedIn post URL"
    return 1
  fi

  info "Xeme Signal: scraping $url"
  local account_flag=()
  [[ -n "$account" ]] && account_flag=(--account "$account")

  if "$XEME_SIGNAL_BIN" scrape post \
      --url "$url" \
      --out "$out" \
      --max-pages "$max_pages" \
      "${account_flag[@]}" &>/dev/null; then
    ok "Xeme Signal: scraped → $out"
    return 0
  else
    error "Xeme Signal: scrape failed"
    return 1
  fi
}

xeme_signal_version() {
  "$XEME_SIGNAL_BIN" --version 2>/dev/null | head -1
}

# Backwards-compatible shims (hidden from new docs)
xeme_yalc_scrape_post() {
  warn "xeme_yalc_scrape_post is deprecated — use xeme_signal_scrape_post"
  xeme_signal_scrape_post "$@"
}

xeme_yalc_version() { xeme_signal_version; }
