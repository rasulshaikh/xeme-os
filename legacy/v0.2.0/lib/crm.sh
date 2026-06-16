#!/usr/bin/env bash
# Xeme OS — CRM (proprietary built-in)
# Self-hosted CRM layer with GraphQL-compatible API surface.
# Internally backed by Xeme's schema; config lives at ~/.xeme/crm.json.

source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

XEME_CRM_CONFIG_PATH="$HOME/.xeme/crm.json"

# Read a field from the Xeme CRM config
# Args: jq_filter (e.g. '.remotes.local.apiUrl')
_xeme_crm_cfg() {
  local filter="$1"
  if [[ ! -f "$XEME_CRM_CONFIG_PATH" ]]; then
    error "Xeme CRM config not found at $XEME_CRM_CONFIG_PATH"
    return 1
  fi
  python3 -c "import json; print(json.load(open('$XEME_CRM_CONFIG_PATH'))$filter)" 2>/dev/null
}

# Test the connection
xeme_crm_status() {
  if [[ ! -f "$XEME_CRM_CONFIG_PATH" ]]; then
    error "Xeme CRM config not found at $XEME_CRM_CONFIG_PATH"
    return 1
  fi
  local api_url api_key
  api_url=$(_xeme_crm_cfg "['remotes']['local']['apiUrl']")
  api_key=$(_xeme_crm_cfg "['remotes']['local']['apiKey']")
  info "Xeme CRM: API URL = $api_url"
  info "Xeme CRM: probing connection..."
  local response
  response=$(curl -s -o /dev/null -w "%{http_code}" \
    -H "Authorization: Bearer $api_key" \
    -H "Content-Type: application/json" \
    -X POST \
    --data '{"query":"{ __typename }"}' \
    "$api_url/graphql" 2>&1)
  if [[ "$response" =~ ^(200|400)$ ]]; then
    # 400 is OK — GraphQL returns 400 for invalid introspection but server is up
    ok "Xeme CRM: reachable (HTTP $response)"
    return 0
  else
    error "Xeme CRM: connection failed (HTTP $response)"
    return 1
  fi
}

# Sync contacts from a CSV to Xeme CRM
# Args: csv_path [--dry-run]
xeme_crm_sync() {
  local csv="$1"
  local dry_run="${2:-false}"

  if [[ ! -f "$csv" ]]; then
    error "CSV not found: $csv"
    return 1
  fi

  local api_url api_key
  api_url=$(_xeme_crm_cfg "['remotes']['local']['apiUrl']")
  api_key=$(_xeme_crm_cfg "['remotes']['local']['apiKey']")

  local rows
  rows=$(csv_count_rows "$csv")
  info "Xeme CRM: syncing $rows contacts from $csv"

  python3 <<PYEOF
import csv, json, urllib.request, urllib.error, sys

api_url = "$api_url"
api_key = "$api_key"
dry_run = "$dry_run" == "true"

count = 0
errors = 0
with open("$csv") as f:
    reader = csv.DictReader(f)
    for row in reader:
        first = row.get("first_name") or row.get("First Name") or ""
        last = row.get("last_name") or row.get("Last Name") or ""
        email = row.get("email") or row.get("extracted_email") or row.get("Email") or ""
        company = row.get("company_name") or row.get("Company") or row.get("company") or ""
        title = row.get("title") or row.get("Title") or ""
        linkedin = row.get("linkedin_url") or row.get("linkedin") or row.get("LinkedIn URL") or ""

        if not (first or last or email):
            continue

        name = f"{first} {last}".strip()
        if not name:
            name = email.split("@")[0]

        # Xeme CRM GraphQL mutation (proprietary schema)
        mutation = {
            "query": """
                mutation CreateOneContact($data: XemeContactCreateInput!) {
                    createXemeContact(data: $data) { id }
                }
            """,
            "variables": {
                "data": {
                    "name": {"firstName": first, "lastName": last} if first or last else {"firstName": name},
                    "emails": {"primaryEmail": email} if email else None,
                    "jobTitle": title or None,
                    "linkedinLink": {"url": linkedin} if linkedin else None,
                }
            }
        }
        # Strip None values
        mutation["variables"]["data"] = {k: v for k, v in mutation["variables"]["data"].items() if v is not None}

        if dry_run:
            print(f"  [DRY-RUN] Would create: {name} ({email}) @ {company} — {title}")
            count += 1
            continue

        try:
            req = urllib.request.Request(
                f"{api_url}/graphql",
                data=json.dumps(mutation).encode(),
                headers={
                    "Authorization": f"Bearer {api_key}",
                    "Content-Type": "application/json"
                },
                method="POST"
            )
            with urllib.request.urlopen(req, timeout=15) as resp:
                result = json.loads(resp.read().decode())
                if "errors" in result:
                    print(f"  [ERROR] {name}: {result['errors'][0].get('message','?')}")
                    errors += 1
                else:
                    print(f"  [OK] {name} → Xeme CRM (id: {result.get('data',{}).get('createXemeContact',{}).get('id','?')})")
                    count += 1
        except Exception as e:
            print(f"  [ERROR] {name}: {e}")
            errors += 1

print(f"\n{'Would sync' if dry_run else 'Synced'}: {count} contacts ({errors} errors)")
PYEOF
}

# Backwards-compatible shims (hidden from new docs)
xeme_twenty_status() { xeme_crm_status; }
xeme_twenty_sync() {
  warn "xeme_twenty_sync is deprecated — use xeme_crm_sync"
  xeme_crm_sync "$@"
}
