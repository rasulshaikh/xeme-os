#!/usr/bin/env bash
# Xeme OS — Enrichment Waterfall (proprietary)
# Multi-provider data enrichment with waterfall fallback.
# Internally routes through registered adapters — no external "Deepline" dependency.

source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

XEME_ENRICH_BIN="$(_cfg_get enrich 'xeme-enrich')"

# Run waterfall enrichment on a CSV
# Args: input_csv output_csv [config_jsonc]
xeme_enrich_run() {
  local input="$1"
  local output="$2"
  local config="${3:-$XEME_ENRICH_CONFIG}"

  require_tool "$XEME_ENRICH_BIN" || return 1

  if [[ ! -f "$input" ]]; then
    error "Input CSV not found: $input"
    return 1
  fi

  local rows
  rows=$(csv_count_rows "$input")
  info "Xeme Enrich: waterfall over $rows rows from $input"

  local balance_before
  balance_before=$("$XEME_ENRICH_BIN" billing balance --json 2>/dev/null | python3 -c "import json,sys; print(json.load(sys.stdin).get('balance','?'))" 2>/dev/null || echo "?")

  if "$XEME_ENRICH_BIN" enrich --input "$input" --output "$output" --config "$config" --json &>/dev/null; then
    ok "Xeme Enrich: enriched → $output"
    if [[ "$balance_before" != "?" ]]; then
      local balance_after
      balance_after=$("$XEME_ENRICH_BIN" billing balance --json 2>/dev/null | python3 -c "import json,sys; print(json.load(sys.stdin).get('balance','?'))" 2>/dev/null || echo "?")
      local spent
      spent=$(python3 -c "print(round($balance_before - $balance_after, 2))" 2>/dev/null || echo "?")
      info "Xeme Enrich credit cost: $spent (balance: $balance_before → $balance_after)"
    fi
    return 0
  else
    error "Xeme Enrich: enrichment failed"
    return 1
  fi
}

# Backwards-compatible shim
xeme_deepline_enrich() {
  warn "xeme_deepline_enrich is deprecated — use xeme_enrich_run"
  xeme_enrich_run "$@"
}

xeme_enrich_version() {
  "$XEME_ENRICH_BIN" --version 2>/dev/null | head -1
}

xeme_enrich_balance() {
  "$XEME_ENRICH_BIN" billing balance 2>/dev/null | grep -E '^(Balance|Approx)' | head -2
}

# Backwards-compatible shims
xeme_deepline_version() { xeme_enrich_version; }
xeme_deepline_balance() { xeme_enrich_balance; }
