#!/usr/bin/env bash
# Xeme OS — The Pipe (proprietary end-to-end pipeline)
# Flow: signal ingest → filter → enrich → score → CRM sync → learn
# Every step uses Xeme-owned modules. No external orchestration.

source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
source "$(dirname "${BASH_SOURCE[0]}")/signal.sh"
source "$(dirname "${BASH_SOURCE[0]}")/enrich.sh"
source "$(dirname "${BASH_SOURCE[0]}")/crm.sh"

# Score a single lead against the Xeme ICP rubric
# Args: title signal email
xeme_pipe_score() {
  local title="$1"
  local signal="$2"
  local email="$3"
  local title_lower signal_lower
  title_lower=$(echo "$title" | tr '[:upper:]' '[:lower:]')
  signal_lower=$(echo "$signal" | tr '[:upper:]' '[:lower:]')

  local score=0

  # Title scoring (Xeme ICP rubric)
  if [[ "$title_lower" == *"cmo"* ]] || [[ "$title_lower" == *"chief marketing"* ]]; then
    score=$((score + $(_cfg_get icp.scoring.title_cmo 50)))
  elif [[ "$title_lower" == *"vp"* ]] && [[ "$title_lower" == *"marketing"* || "$title_lower" == *"demand"* ]]; then
    score=$((score + $(_cfg_get icp.scoring.title_vp_marketing 40)))
  else
    score=$((score + $(_cfg_get icp.scoring.title_other 20)))
  fi

  # Signal scoring
  if [[ "$signal_lower" == *"evaluating"* ]] || [[ "$signal_lower" == *"job change"* ]] || [[ "$signal_lower" == *"g2"* ]] || [[ "$signal_lower" == *"capterra"* ]]; then
    score=$((score + $(_cfg_get icp.scoring.signal_evaluating 25)))
  elif [[ "$signal_lower" == *"commented"* ]] || [[ "$signal_lower" == *"posted"* ]] || [[ "$signal_lower" == *"engaged"* ]]; then
    score=$((score + $(_cfg_get icp.scoring.signal_commenting 20)))
  else
    score=$((score + $(_cfg_get icp.scoring.signal_following 15)))
  fi

  # Email verified bonus
  if [[ -n "$email" && "$email" == *"@"* ]]; then
    score=$((score + $(_cfg_get icp.scoring.email_verified 5)))
  fi

  if [[ $score -gt 100 ]]; then score=100; fi
  echo "$score"
}

# Tier from score
xeme_pipe_tier() {
  local score="$1"
  local t1 t2
  t1=$(_cfg_get icp.scoring.tier1_threshold 70)
  t2=$(_cfg_get icp.scoring.tier2_threshold 50)
  if [[ $score -ge $t1 ]]; then echo "Tier 1 - Hot"
  elif [[ $score -ge $t2 ]]; then echo "Tier 2 - Warm"
  else echo "Tier 3 - Nurture"
  fi
}

# Main pipe runner
# Args: --signal-url URL | --input CSV [--roles ROLES] [--min-score N] [--dry-run] [--no-crm]
xeme_pipe_run() {
  local signal_url="" roles="" min_score=0 dry_run=false no_crm=false
  local input_csv=""

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --signal-url|--scrape-url) signal_url="$2"; shift 2 ;;
      --input)                   input_csv="$2"; shift 2 ;;
      --roles)                   roles="$2"; shift 2 ;;
      --min-score)               min_score="$2"; shift 2 ;;
      --dry-run)                 dry_run=true; shift ;;
      --no-crm)                  no_crm=true; shift ;;
      *)                         warn "Unknown arg: $1"; shift ;;
    esac
  done

  local ws
  ws=$(ensure_workspace)
  local stamp
  stamp=$(date "+%Y%m%d_%H%M%S")
  local signals_csv="$ws/signals_${stamp}.csv"
  local enriched_csv="$ws/enriched_${stamp}.csv"
  local final_csv="$ws/final_${stamp}.csv"

  section "Xeme OS Pipe — Run $stamp"

  # Step 1: Ingest signals
  if [[ -n "$signal_url" ]]; then
    info "[1/5] Xeme Signal: scraping $signal_url"
    if ! xeme_signal_scrape_post "$signal_url" "$signals_csv"; then
      error "Xeme Signal ingest failed. Aborting."
      return 1
    fi
  elif [[ -n "$input_csv" ]]; then
    info "[1/5] Using existing input: $input_csv"
    cp "$input_csv" "$signals_csv"
  else
    error "Provide --signal-url or --input"
    return 1
  fi

  local signal_rows
  signal_rows=$(csv_count_rows "$signals_csv")
  info "Signals: $signal_rows rows"
  if [[ $signal_rows -eq 0 ]]; then
    warn "No signals to process. Exiting."
    return 0
  fi

  # Step 2: Role filter
  if [[ -n "$roles" ]]; then
    info "[2/5] Filtering to roles: $roles"
    python3 <<PYEOF
import csv
roles = [r.strip().lower() for r in "$roles".split(",")]
with open("$signals_csv") as f:
    reader = csv.DictReader(f)
    rows = list(reader)
    headers = reader.fieldnames or []
def title_matches(title):
    if not title: return False
    t = title.lower()
    for r in roles:
        if r in t: return True
    return False
filtered = [r for r in rows if title_matches(r.get("title") or r.get("Title") or "")]
with open("$signals_csv","w",newline="") as f:
    writer = csv.DictWriter(f, fieldnames=headers)
    writer.writeheader()
    writer.writerows(filtered)
print(f"  Filtered: {len(rows)} → {len(filtered)} rows match roles")
PYEOF
  else
    info "[2/5] No role filter (using all signals)"
  fi

  # Step 3: Xeme Enrich waterfall
  info "[3/5] Xeme Enrich: waterfall enrichment"
  if ! xeme_enrich_run "$signals_csv" "$enriched_csv"; then
    error "Xeme Enrich failed. Aborting."
    return 1
  fi

  # Step 4: Score + tier
  info "[4/5] Scoring + tiering"
  python3 <<PYEOF
import csv, json

def extract_contact(cell):
    if not cell: return None
    try: data = json.loads(cell) if isinstance(cell, str) else cell
    except: return None
    result = data.get('result') if isinstance(data, dict) else None
    if isinstance(result, list) and result and isinstance(result[0], dict):
        return result[0]
    return None

def extract_email(cell, hint=''):
    if hint and '@' in hint: return hint
    if not cell: return None
    try: data = json.loads(cell) if isinstance(cell, str) else cell
    except: return None
    result = data.get('result') if isinstance(data, dict) else None
    if isinstance(result, str) and '@' in result: return result
    if isinstance(result, list) and result and isinstance(result[0], dict):
        return result[0].get('email')
    if isinstance(result, dict) and 'email' in result: return result['email']
    return None

with open("$enriched_csv") as f:
    reader = csv.DictReader(f)
    rows = list(reader)
    headers = list(reader.fieldnames or [])

# Build the final output
final_headers = ['first_name','last_name','title','company_name','domain','email','linkedin_url','score','tier','signal_source']
new_rows = []
for r in rows:
    contact = extract_contact(r.get('contact',''))
    email = extract_email(r.get('email_waterfall',''), contact.get('email') if contact else None)
    if contact:
        full = (contact.get('full_name') or contact.get('name') or '').strip()
        parts = full.split()
        first = parts[0].title() if parts else (r.get('first_name') or '')
        last = ' '.join(parts[1:]).title() if len(parts) > 1 else (r.get('last_name') or '')
        title = contact.get('title') or r.get('title','')
        linkedin = (contact.get('linkedin') or '').replace('http://www.linkedin.com','').replace('https://www.linkedin.com','').replace('http://linkedin.com','')
    else:
        first = r.get('first_name','')
        last = r.get('last_name','')
        title = r.get('title','')
        linkedin = r.get('linkedin_url','')

    if not email and contact:
        email = contact.get('email')

    signal = r.get('signal_source') or r.get('signal') or ''
    title_lower = (title or '').lower()
    signal_lower = (signal or '').lower()
    score = 0
    if 'cmo' in title_lower or 'chief marketing' in title_lower: score += 50
    elif 'vp' in title_lower and ('marketing' in title_lower or 'demand' in title_lower): score += 40
    else: score += 20
    if 'evaluating' in signal_lower or 'job change' in signal_lower or 'g2' in signal_lower or 'capterra' in signal_lower: score += 25
    elif 'commented' in signal_lower or 'posted' in signal_lower or 'engaged' in signal_lower: score += 20
    else: score += 15
    if email and '@' in email: score += 5
    if score > 100: score = 100
    if score >= 70: tier = 'Tier 1 - Hot'
    elif score >= 50: tier = 'Tier 2 - Warm'
    else: tier = 'Tier 3 - Nurture'

    new_rows.append({
        'first_name': first, 'last_name': last, 'title': title,
        'company_name': r.get('company_name') or r.get('company',''),
        'domain': r.get('domain',''),
        'email': email or '',
        'linkedin_url': linkedin or '',
        'score': score, 'tier': tier,
        'signal_source': signal,
    })

if $min_score > 0:
    before = len(new_rows)
    new_rows = [r for r in new_rows if r['score'] >= $min_score]
    print(f"  Filtered to min-score {$min_score}: {before} → {len(new_rows)} rows")

with open("$final_csv", 'w', newline='') as f:
    writer = csv.DictWriter(f, fieldnames=final_headers)
    writer.writeheader()
    writer.writerows(new_rows)

t1 = sum(1 for r in new_rows if r['tier'] == 'Tier 1 - Hot')
t2 = sum(1 for r in new_rows if r['tier'] == 'Tier 2 - Warm')
t3 = sum(1 for r in new_rows if r['tier'] == 'Tier 3 - Nurture')
emails = sum(1 for r in new_rows if r['email'])
print(f"  Total: {len(new_rows)} | Tier 1: {t1} | Tier 2: {t2} | Tier 3: {t3} | Emails: {emails}")
PYEOF

  ok "Final output: $final_csv"

  # Step 5: Xeme CRM sync
  if [[ "$no_crm" == "true" ]]; then
    info "[5/5] Skipping CRM sync (--no-crm)"
  elif [[ "$dry_run" == "true" ]]; then
    info "[5/5] DRY RUN: would sync to Xeme CRM"
  else
    info "[5/5] Xeme CRM: syncing contacts"
    xeme_crm_sync "$final_csv" false
  fi

  section "Done"
  ok "Final file: $final_csv"
  ok "Rows: $(csv_count_rows "$final_csv")"
}
