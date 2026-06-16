<p align="center">
  <img src="https://raw.githubusercontent.com/xeme-os/xeme/main/.github/banner.png" alt="Xeme OS" width="600">
</p>

<h1 align="center">XEME OS</h1>
<p align="center">
  <strong>The AI-native GTM operating system.</strong><br>
  Self-hosted. MIT-licensed. 1-click install. Replaces $1,200-2,000/mo of commercial GTM tooling for $0-50/mo.
</p>

<p align="center">
  <a href="#install">Install</a> · <a href="#quickstart">Quickstart</a> · <a href="#commands">Commands</a> · <a href="#mcp">MCP</a> · <a href="#stack">Stack</a> · <a href="https://github.com/xeme-os/xeme/issues">Issues</a>
</p>

---

> **Xeme OS is a single Go binary that runs the entire outbound loop:**
> **discover → enrich → qualify → sequence → track — and now also monitors how AI search engines cite you (AEO/GEO).**

It bundles what took you 6+ paid tools to do:
- **Clay-style waterfall enrichment** (MoltSets + TheirStack + BuiltWith + local DNS)
- **Twenty-style CRM** (Xeme Ledger, GraphQL + SQLite)
- **Sequencer-style multichannel outreach** (email + LinkedIn + SMS)
- **YALC-style agent skills** (15 MCP tools for Claude Code)
- **Scrapling-style web scraper** (HN + job boards + signal scraping)
- **AEO/GEO tracking** (ChatGPT, Perplexity, You.com, Phind, Komo)

---

## Install

```bash
# 1-click: macOS / Linux (Apple Silicon + Intel + Linux)
curl -fsSL https://raw.githubusercontent.com/xeme-os/xeme/main/install/install.sh | bash

# Homebrew (macOS / Linux)
brew install xeme-os/tap/xeme

# Go
go install github.com/xeme-os/xeme/cmd/xeme@latest

# npm (Node wrapper)
npm install -g xeme-os
```

After install:
```bash
xeme init       # scaffold ~/.xeme/ (config + ledger)
xeme serve      # start local dashboard on http://127.0.0.1:4903
```

**That's it.** Xeme OS is running. From there, talk to it via:

- **CLI** — `xeme enrich --in leads.csv --out enriched.csv`
- **Web** — `http://127.0.0.1:4903` (local dashboard)
- **MCP** — `claude mcp add xeme --transport stdio -- xeme-mcp` (15 tools for Claude Code)
- **YALC / n8n / your own agent** — point at the MCP server or the REST API on :4903

---

## Quickstart

```bash
# 1. Enrich 100 leads through the waterfall (MoltSets + TheirStack + BuiltWith + local)
xeme enrich --in leads.csv --out enriched.csv

# 2. Score them with the 7-gate ICP rubric
xeme score --in enriched.csv --out scored.csv --min-score 60

# 3. Push Tier 1 leads to the Xeme Ledger
xeme crm sync --in scored.csv --dry-run
xeme crm sync --in scored.csv

# 4. Start a 7-step multichannel sequence
xeme sequence start --in tier1.csv --sequence default-7step

# 5. Track replies, opens, clicks
xeme sequence status

# 6. Monitor AEO/GEO: are you cited by ChatGPT/Perplexity?
xeme aeo status
xeme aeo check   # scores your brand across AI engines (0-100)

# 7. Or just ask Claude Code to do it:
claude> "Find me 50 VPs of Marketing at AI startups in SF, enrich them, and add to my CRM"
#         ↓
#         [MCP] → xeme_intel_search_companies → xeme_moltsets_enrich_email →
#                xeme_score_leads → xeme_crm_sync
```

---

## Commands

```
xeme init                  Scaffold ~/.xeme/ (config + ledger defaults)
xeme serve                 Start local dashboard on :4903
xeme status                Health check across all engines
xeme version               Print version
xeme config                Show resolved config

# Discovery + signal
xeme scrape                Scrape engagers from a public post URL
xeme intel search <query>  Search TheirStack for companies
xeme intel jobs <keywords> Search 202M job postings
xeme intel company <domain>  Get all signals for a domain

# Enrichment (4-tier waterfall)
xeme enrich --in <csv>     Local → Upstream → MoltSets → Demo
xeme moltsets status       Check MoltSets account balance
xeme moltsets search "moltsets.com"

# ICP scoring (7-gate)
xeme score --in <csv> --out <csv> [--min-score 60]

# CRM
xeme crm status            Probe Xeme Ledger
xeme crm sync --in <csv>   Sync CSV to Ledger
xeme crm graph             GraphQL playground

# Multichannel sequence
xeme sequence list         List sequences
xeme sequence start        Start a sequence for a CSV
xeme sequence status       Status of running sequences
xeme sequence pause        Pause a sequence
xeme sequence reply        Log a reply to a step

# End-to-end pipe
xeme pipe --in <csv>       scrape → enrich → score → sync

# AEO/GEO
xeme aeo status            Brand, prompts, competitors
xeme aeo check             Run all prompts × all engines
xeme aeo score             0-100 AEO score
xeme aeo optimize <url>    Analyze URL for AEO-readiness

# Self-update
xeme update                Pull latest + rebuild
xeme install <package>     Add a sub-component
```

---

## MCP — Claude Code Native

```bash
# Register Xeme OS as an MCP server in Claude Code
claude mcp add xeme --transport stdio -- xeme-mcp
```

You now have **20 tools** available in Claude Code:

| Tool | What it does |
|---|---|
| `xeme_status` | Engine health |
| `xeme_scrape_post` | Scrape engagers |
| `xeme_enrich_leads` | Waterfall enrich CSV |
| `xeme_score_leads` | 7-gate ICP scoring |
| `xeme_run_pipe` | End-to-end |
| `xeme_crm_sync` | Sync to Ledger |
| `xeme_moltsets_status` | MoltSets account |
| `xeme_moltsets_search_company` | Find companies |
| `xeme_moltsets_search_people` | Find people |
| `xeme_moltsets_enrich_email` | Get email from LinkedIn |
| `xeme_moltsets_enrich_phone` | Get mobile phone |
| `xeme_intel_status` | Intel providers |
| `xeme_intel_company_signals` | All signals for a domain |
| `xeme_intel_search_companies` | TheirStack company search |
| `xeme_intel_search_jobs` | TheirStack job search |
| `xeme_intel_tech_lookup` | BuiltWith free tech lookup |
| `xeme_intel_hn_hiring_signals` | HN "Who Is Hiring" |
| `xeme_aeo_status` | AEO config |
| `xeme_aeo_check_brand` | AI engine mentions |
| `xeme_aeo_score` | 0-100 AEO score |
| `xeme_aeo_optimize` | URL analysis |

Tell Claude: *"Find me 50 VPs of Marketing at Series A AI startups, enrich with emails + phones, score against our ICP, push Tier 1 to our CRM, and start a 5-step sequence."* — done.

---

## Stack — what it replaces

| Layer | Xeme OS | Commercial | $ saved |
|---|---|---|---|
| **Enrichment** | Local + MoltSets + TheirStack + BuiltWith | Clay | $300-1,000/mo |
| **CRM** | Xeme Ledger (SQLite + GraphQL) | Salesforce | $150/user/mo |
| **Sequencer** | Multichannel engine | Outreach/Salesloft | $100/user/mo |
| **Signals** | TheirStack + BuiltWith + HN scraper | ZoomInfo + Bombora | $500+/mo |
| **AEO/GEO** | Brand mention tracker | Profound / Scrunch | $500+/mo |
| **Scraping** | HN + job boards + adaptors | Apify + Phantombuster | $99+/mo |
| **Vector** | (use Qdrant or Weaviate alongside) | Pinecone | $70+/mo |
| **Automation** | (use n8n alongside) | Zapier | $50+/mo |
| **TOTAL** | **$0-50/mo** | **$1,200-2,000+/mo** | **$15-24k/yr per seat** |

**Optional paid upgrades** (BYOK — bring your own key):
- **MoltSets** ($27-497/mo) — 100%-verified emails + mobile phones
- **TheirStack** (~$49/mo) — 32k technologies, 202M job postings
- **Perplexity API** — best-in-class AI engine for AEO
- **OpenAI / Anthropic** — for paid AEO tracking

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                       XEME OS  (single Go binary)            │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│   ┌─────────────┐  ┌─────────────┐  ┌──────────────┐         │
│   │   Signal    │  │   Enrich    │  │    Intel     │         │
│   │  (scraping) │  │ (4-tier     │  │ (techno +    │         │
│   │  HN, jobs,  │  │  waterfall) │  │  hiring +    │         │
│   │  engagers)  │  │  local+API  │  │  intent)     │         │
│   └──────┬──────┘  └──────┬──────┘  └──────┬───────┘         │
│          └────────┬──────┴─────┬────────┘                    │
│                   │            │                            │
│   ┌───────────────▼──┐  ┌──────▼───────┐  ┌──────────────┐   │
│   │     Scorer       │  │   Ledger    │  │  AEO/GEO     │   │
│   │  (7-gate ICP)    │  │  (GraphQL + │  │  (AI-search  │   │
│   │                  │  │   SQLite)   │  │   tracker)   │   │
│   └─────────┬────────┘  └──────┬──────┘  └──────┬───────┘   │
│             └────────┬────────┘                 │           │
│                      │                          │           │
│   ┌──────────────────▼─────────────────────┐    │           │
│   │       Sequencer (multichannel)         │    │           │
│   │   email · linkedin · sms · whatsapp     │    │           │
│   └──────────────────┬─────────────────────┘    │           │
│                      │                          │           │
│   ┌──────────────────▼──────────────────────────▼──────┐    │
│   │        Orchestrator (Pipe / xeme run)             │    │
│   └──────────────────┬──────────────────────────┬──────┘    │
│                      │                          │           │
│   ┌──────────────────▼─────┐  ┌──────────────────▼────┐   │
│   │   xeme (CLI)            │  │   xeme-mcp (MCP)     │   │
│   │   xeme-os (Dashboard)   │  │   xeme-campaigns     │   │
│   │   xeme-workflows (n8n)  │  │   xeme-ledger-server │   │
│   └────────────────────────┘  └───────────────────────┘   │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

5 proprietary engines. 6 binaries. 20 MCP tools. 1 CLI.

---

## Why Xeme OS

**Self-hosted.** Your data never leaves your infra. No vendor lock-in. No "we changed our pricing."

**AI-native.** Built for the agent era. The CLI, the dashboard, the MCP server — every surface is designed for both humans and AI agents.

**Open core, premium plugins.** The whole stack is MIT-licensed. MoltSets, TheirStack, Perplexity — plug in your own keys, pay only for what you use.

**One binary.** The whole thing is a single 18MB Go binary. No Docker required (but works with it). No Node, no Python, no npm dependencies.

**Local-first.** Runs on your laptop. Runs on your homelab. Runs on a $5/mo VPS. The dashboard is at `http://localhost:4903`.

---

## Sub-components you can install

```bash
# After xeme init, install the open-source Clay alternative stack
xeme install deepline     # deepline.gtm data providers
xeme install clay         # open-source Clay alternative (Bricks-style)
xeme install twenty        # Twenty CRM
xeme install sequencer     # multichannel sequencer
xeme install yalc          # YALC skills library
xeme install scrapling     # Scrapling web scraper

# Or all of them
xeme install all
```

These are optional. Xeme OS works standalone.

---

## License

MIT — fork it, ship it, sell it.

## Credits

Built by [rasul](https://github.com/rasul) in San Francisco. Inspired by [Deepline](https://deepline.com), [Clay](https://clay.com), [Twenty](https://twenty.com), [Sequencer](https://sequencer.so), [YALC](https://github.com/Othmane-Khadri/YALC-the-GTM-operating-system), [Scrapling](https://github.com/d4vinci/Scrapling), [MoltSets](https://moltsets.com), [Billionmail](https://github.com/billionmail), [n8n](https://n8n.io), and everyone who said "self-host or die."
