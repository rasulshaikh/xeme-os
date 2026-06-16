#!/usr/bin/env bash
# Xeme OS — Demo script
# Runs the pipe against the sample CSV (no real LinkedIn scraping)

set -e
XEME_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$XEME_ROOT"

echo "═══════════════════════════════════════════════════════════"
echo "  Xeme OS — Demo Run"
echo "═══════════════════════════════════════════════════════════"
echo ""
echo "This demo runs the pipe against the sample CSV (5 leads)."
echo "No real LinkedIn scraping. Skips Twenty CRM sync."
echo ""

xeme pipe \
  --input examples/sample-leads.csv \
  --min-score 50 \
  --no-crm

echo ""
echo "═══════════════════════════════════════════════════════════"
echo "  Demo complete. Final files in workspace/"
echo "═══════════════════════════════════════════════════════════"
ls -la workspace/ | head -10
