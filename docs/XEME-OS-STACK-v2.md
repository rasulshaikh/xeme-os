# XEME OS — Full Open-Source GTM Stack Spec
## Agent-Ready Plugin & Integration Spec | v2.0 | 2026-06-05

---

## MISSION

Build a fully self-hosted, open-source, AI-native GTM Operating System that replaces:
- Clay ($5B platform) → for data enrichment
- Salesforge / Instantly → for email outreach
- Salesforce → for CRM
- Zapier / Make → for automation
- MoltSets → for agent-ready contact APIs
- Gong ($120/seat) → for meeting intelligence
- Intercom ($74/seat) → for chat/qualification
- Phantombuster ($99/mo) → for LinkedIn automation

**Core Principle:** Your data stays on your servers. You own everything. No vendor lock-in.

---

## THE COMPLETE STACK — LAYERS

```
╔══════════════════════════════════════════════════════╗
║               XEME OS v2 — FULL STACK            ║
╠══════════════════════════════════════════════════════╣
║  LAYER 7  │  AGENT LAYER        │ CrewAI + SalesGPT + YALC    ║
║  LAYER 6  │  CHAT / WIDGET     │ Chatwoot + Tiledesk        ║
║  LAYER 5  │  OUTREACH ENGINE   │ Billionmail + OpenOutreach   ║
║  LAYER 4  │  MEETING INTEL    │ Attendee                   ║
║  LAYER 3  │  DATA ENRICHMENT  │ Bricks + MoltSets + BuiltWith ║
║  LAYER 2  │  AUTOMATION       │ n8n + Semantic Router       ║
║  LAYER 1  │  CRM / DB         │ Twenty                     ║
║  LAYER 0  │  VECTOR / DATA    │ Qdrant + Airbyte + Multiwoven║
║            │  VISUAL BUILDER   │ Flowise                    ║
╚══════════════════════════════════════════════════════╝
```

---

## LAYER 1 — CRM (Replace Salesforce)

### Twenty — Self-Hosted CRM
- **GitHub:** github.com/twentyhq/twenty
- **Stars:** 44k+
- **License:** AGPL-3.0
- **Install:**
  ```bash
  git clone https://github.com/twentyhq/twenty.git
  cd twenty && cp .env.example .env
  docker compose up -d
  ```
- **What it replaces:** Salesforce, HubSpot CRM, Pipedrive
- **XEME OS Integration:** Webhook → n8n → Twenty contacts, Twenty API → enrich with Bricks data

---

## LAYER 2 — Automation (Replace Zapier/Make)

### n8n — Workflow Engine
- **URL:** n8n.io
- **License:** Sustainable open-source
- **Install:**
  ```bash
  docker run -d --name n8n -p 5678:5678 -v ~/.n8n:/home/node/.n8n n8nio/n8n
  ```
- **400+ integrations, AI nodes built-in, webhook triggers**

### Semantic Router — LLM Cost Optimizer
- **GitHub:** github.com/aurelio-labs/semantic-router
- **Stars:** 8k+
- **What it does:** Routes LLM requests to cheapest/fastest model — <1ms overhead
  ```python
  # Simple tasks → Haiku (cheap), Complex → Sonnet/Opus
  pip install semantic-router
  ```

---

## LAYER 3 — Data Enrichment (Replace Clay)

### Option A — Bricks (Fully Local)
- **GitHub:** github.com/BraaMohammed/bricks
- **Type:** Open-source Clay alternative, AI agents + web scraping + Puppeteer
- **Install:**
  ```bash
  git clone https://github.com/BraaMohammed/bricks.git && cd bricks && npm install && npm start
  ```

### Option B — Xeme OS Intel Engine (Already Built)
- `internal/intel/` — TheirStack + BuiltWith Free + Signal Scraper (HN + RemoteOK)
- **No key needed** for BuiltWith + Signal Scraper
- **TheirStack** requires `XEME_THEIRSTACK_API_KEY`

### Option C — Fire Enrich
- **URL:** firecrawl.dev/blog/fire-enrich
- Open-source AI-powered data enrichment via web scraping

### Option D — DIY Waterfall
- **Stack:** n8n + Apollo API + Hunter.io + Apify scrapers
- Apollo: apollo.io (free tier: 50 credits/mo)

---

## LAYER 4 — Meeting Intelligence (Replace Gong)

### Attendee — Meeting Transcription + Analysis
- **GitHub:** github.com/attendeellabs/attendee
- **Stars:** 1.5k+
- **Install:**
  ```bash
  git clone https://github.com/attendee-labs/attendee.git && cd attendee && pip install -r requirements.txt
  ```
- **What it does:** Zoom/Meet → Transcribes → Scores MEDDIC/BANT → Logs to Twenty → Feeds Qdrant
- **Pipeline:**
  1. Call happens
  2. Attendee transcribes + extracts: pain points, budget, timeline, competitors
  3. n8n → Creates Twenty task + triggers follow-up
  4. Qdrant → Stores embedding for RAG
  5. CrewAI → Scores next outreach based on call

---

## LAYER 5 — Outreach (Replace Instantly/Salesforge)

### Billionmail — Cold Email (Open Source)
- **GitHub:** github.com/billionmail
- **Install:**
  ```bash
  git clone https://github.com/billionmail/billionmail.git && cd billionmail && docker compose up -d
  ```

### OpenOutreach — LinkedIn Automation (Replace Phantombuster)
- **GitHub:** github.com/eracle/OpenOutreach
- **Stars:** 2k+
- **Install:**
  ```bash
  git clone https://github.com/eracle/OpenOutreach.git && cd OpenOutreach && pip install -r requirements.txt
  cp .env.example .env && python main.py --config outreach_config.yaml
  ```
- **Pipeline:** Twenty "Ready for LinkedIn" → OpenOutreach visits profiles → Sends connection request → Logs to n8n → Updates Twenty

### MailWizz — Email Warmup
- **URL:** mailwizz.com
- Self-hosted, built-in warmup, ~$100-300 one-time

---

## LAYER 6 — Chat/Widget (Replace Intercom)

### Chatwoot — AI Chat Widget
- **GitHub:** github.com/chatwoot/chatwoot
- **Stars:** 23k+
- **Install:**
  ```bash
  git clone https://github.com/chatwoot/chatwoot.git && cd chatwoot && docker compose up -d
  ```
- **Pipeline:** Website visitor → Chatwoot widget → AI qualifies → Twenty contact → n8n trigger → Slack alert

### Tiledesk — Omnichannel Alternative (WhatsApp+)
- **GitHub:** github.com/Tiledesk/tiledesk
- **Install:**
  ```bash
  git clone https://github.com/Tiledesk/tiledesk-server.git && cd tiledesk-server && docker compose up -d
  ```

---

## LAYER 7 — AI Agent Layer (Replace Clay AI + Custom Agents)

### CrewAI — Multi-Agent Orchestration
- **GitHub:** github.com/crewaiinc/crewai
- **Stars:** 30k+
- **Install:**
  ```bash
  pip install crewai crewai-tools
  ```
- **Use:** Orchestrate multiple agents as a team (Researcher + Enricher + Writer + QC)
- **XEME OS Crew SDR Pipeline:**
  ```python
  from crewai import Agent, Crew, Task, Process
  researcher = Agent(role="Lead Researcher", goal="Find ICP-matching B2B leads", tools=[search_tool])
  enricher = Agent(role="Data Enricher", goal="Get verified emails + phones", tools=[moltsets_tool])
  writer = Agent(role="Email Writer", goal="Write personalized cold emails", tools=[])
  qc = Agent(role="Quality Checker", goal="Score email against ICP", tools=[])
  crew = Crew(agents=[researcher, enricher, writer, qc], process=Process.sequential)
  ```

### SalesGPT — Context-Aware Sales Agent
- **GitHub:** github.com/filip-michalsky/SalesGPT
- **Stars:** 4k+
- **What it does:** Handles reply escalation, objection handling, natural multi-turn email conversations
- **Pipeline:** MoltSets → Enrich → SalesGPT writes email → Billionmail sends → SalesGPT handles replies → Escalates hot leads to human AE

### YALC — Claude Code Native GTM OS
- **GitHub:** github.com/Othmane-Khadri/YALC-the-GTM-operating-system
- **Skills Library:** library.moltsets.com (29 pre-built skills)
- **Install:** Clone into Claude Code, follow setup in library

---

## DATA LAYER (LAYER 0)

### Qdrant — Vector Database
- **GitHub:** github.com/qdrant/qdrant
- **Stars:** 13k+
- **License:** Apache-2.0
- **Install:**
  ```bash
  docker run -d --name qdrant -p 6333:6333 -p 6334:6334 -v $(pwd)/qdrant_storage:/qdrant/storage qdrant/qdrant
  ```
- **Use:** Semantic lead scoring, email memory, ICP matching, competitor detection

### Airbyte — Data Pipeline (300+ connectors)
- **GitHub:** github.com/airbytehq/airbyte
- **Stars:** 15k+
- **Install:**
  ```bash
  git clone https://github.com/airbytehq/airbyte && cd airbyte && docker compose up
  ```
- **Use:** Keep Qdrant fresh, sync Apollo → Twenty, feed Chatwoot data

### Multiwoven — Reverse ETL
- **GitHub:** github.com/Multiwoven/multiwoven
- **Stars:** 3k+
- **Install:**
  ```bash
  git clone https://github.com/Multiwoven/multiwoven && cd multiwoven && docker compose up
  ```
- **Use:** Sync Twenty → Meta/LinkedIn audiences, Qdrant scores → Twenty, enrichment data → CRM

### Flowise — Visual AI Agent Builder
- **GitHub:** github.com/FlowiseAI/Flowise
- **Stars:** 18k+
- **Install:**
  ```bash
  docker run -d --name flowise -p 3000:3000 flowiseai/flowise
  ```
- **Use:** No-code AI workflow prototyping before building in n8n

### Weaviate — Alternative Vector DB
- **GitHub:** github.com/weaviate/weaviate
- **Stars:** 11k+
- **Install:**
  ```bash
  docker run -d -p 8080:8080 semitechnologies/weaviate
  ```
- **Use:** Hybrid keyword + semantic search, multi-modal (text + images + audio)

---

## QUICK START — GET XEME OS LIVE IN 2 HOURS

```bash
# PHASE 1: Foundation (30 min)
git clone https://github.com/twentyhq/twenty.git && cd twenty && cp .env.example .env && docker compose up -d
docker run -d --name n8n -p 5678:5678 -v ~/.n8n:/home/node/.n8n n8nio/n8n
docker run -d --name qdrant -p 6333:6333 -p 6334:6334 -v $(pwd)/qdrant_storage:/qdrant/storage qdrant/qdrant

# PHASE 2: Data Layer (30 min)
git clone https://github.com/BraaMohammed/bricks.git && cd bricks && npm install
git clone https://github.com/airbytehq/airbyte && cd airbyte && docker compose up
git clone https://github.com/Multiwoven/multiwoven && cd multiwoven && docker compose up

# PHASE 3: AI Agents (30 min)
pip install crewai crewai-tools semantic-router
docker run -d --name flowise -p 3000:3000 flowiseai/flowise

# PHASE 4: Outreach (30 min)
git clone https://github.com/billionmail/billionmail && cd billionmail && docker compose up
git clone https://github.com/eracle/OpenOutreach && cd OpenOutreach && pip install -r requirements.txt
git clone https://github.com/chatwoot/chatwoot && cd chatwoot && docker compose up
git clone https://github.com/attendee-labs/attendee && cd attendee && pip install -r requirements.txt

# PHASE 5: Agent Layer
git clone https://github.com/Othmane-Khadri/YALC-the-GTM-operating-system.git
# Set MoltSets key (optional, $27/mo)
export XEME_THEIRSTACK_API_KEY="your_key_here"
export XEME_MOLTSETS_API_KEY="your_moltsets_key"
```

---

## COST COMPARISON

| Commercial Tool | Monthly Cost | XEME OS Cost |
|---|---|---|
| Clay | $300-1000/mo | Free (Bricks + n8n) |
| Gong | $120/seat/mo | Free (Attendee) |
| Phantombuster | $99/mo | Free (OpenOutreach) |
| Intercom | $74/seat/mo | Free (Chatwoot) |
| Census/Hightouch | $500+/mo | Free (Multiwoven) |
| Vector DB (Pinecone) | $70+/mo | Free (Qdrant) |
| CrewAI SaaS | $200+/mo | Free (self-hosted) |
| Flowise SaaS | $49/mo | Free (self-hosted) |
| **TOTAL** | **~$1,200-2,000/mo** | **~$0-50/mo** |

---

## FULL PIPELINE — AGENT WORKFLOW

```
[INPUT: CSV of 100 target companies]
    ↓
[Airbyte] Cleans & normalizes data → Qdrant staging
    ↓
[Qdrant + Semantic Router] ICP embedding match
    Route: "score lead" → Haiku (cheap)
    Route: "write email" → Sonnet (capable)
    Route: "scrape data" → Bricks (local)
    ↓
[CrewAI SDR Crew]
    Researcher → Finds prospects
    Enricher → Calls MoltSets / Bricks
    Writer → Personalized emails
    QC → Scores before send
    ↓
[Multiwoven] Sync scores to Twenty → Push to Meta/LinkedIn audiences → Trigger n8n
    ↓
[n8n] Hot Lead → Slack alert + email to AE | Reply → SalesGPT handles objection
    ↓
[Attendee] Post-call: transcribes → extracts insights → scores MEDDIC → logs to Twenty
    ↓
[Qdrant] Stores embedding for RAG recall
    ↓
[Chatwoot] Website visitors → AI qualifies → Creates Twenty contact → n8n trigger
    ↓
[OUTPUT: 100 companies → 47 qualified → 12 demos → 3 deals, all logged in Twenty]
```

---

## AGENT COMMANDS — COPY-PASTE FOR CLAUDE CODE

```
"Build the full XEME OS enrichment pipeline:
1. Use Bricks to scrape company data from my CSV of 100 companies
2. Use MoltSets to enrich with business emails + mobile phones
3. Use BuiltWith free API for tech stack detection
4. Use HackerNews signal scraper for hiring signals
5. Write enriched results to Twenty CRM
6. Trigger Billionmail outreach sequence via n8n webhook"

"Set up the CrewAI SDR crew with roles: Researcher, Enricher, Writer, QC.
Each agent should use XEME OS engines as tools."

"Build the Qdrant vector store for ICP matching:
Embed my top 50 customers → Store in Qdrant → Score new leads by cosine similarity"
```

---

## KNOWN GAPS & WORKAROUNDS

| Issue | Workaround |
|---|---|
| B2B contact data isn't free | Apollo free tier + MoltSets paid for critical mass |
| Email warmup takes 4-6 weeks | Start MailWizz warmup immediately |
| Self-hosting needs maintenance | Use Portainer for Docker management |
| LinkedIn anti-bot detection | OpenOutreach with randomized delays + user agent rotation |
| DNS setup complex | Cloudflare wizard + MailWizz docs |
| CrewAI needs prompt tuning | Start with example SDR crew, iterate on prompts |
| Airbyte connectors break on API changes | Pin versions, test monthly |

---

## AGENT CHECKLIST

- [ ] Install Twenty (CRM) — Docker compose
- [ ] Install n8n (Automation) — Docker
- [ ] Install Qdrant (Vector DB) — Docker
- [ ] Install Bricks (Local Enrichment) — npm
- [ ] Clone Airbyte (Data Pipeline) — Docker
- [ ] Install Multiwoven (Reverse ETL) — Docker
- [ ] Install CrewAI (Agent Framework) — pip
- [ ] Install Semantic Router — pip
- [ ] Install Flowise (Visual Builder) — Docker
- [ ] Install Billionmail (Email Outreach) — Docker
- [ ] Install OpenOutreach (LinkedIn) — pip
- [ ] Install Chatwoot (Chat Widget) — Docker
- [ ] Install Attendee (Meeting Intel) — pip
- [ ] Clone YALC into Claude Code
- [ ] Set XEME_THEIRSTACK_API_KEY (optional, $27/mo for TheirStack)
- [ ] Configure DNS (SPF/DKIM/DMARC via Cloudflare)
- [ ] Build n8n enrichment workflow: Bricks → MoltSets → Twenty
- [ ] Build n8n outreach workflow: Twenty → Billionmail
- [ ] Build Qdrant ICP scoring pipeline
- [ ] Build Chatwoot → Twenty → n8n lead qualification flow
- [ ] Build Attendee → Twenty call logging flow
- [ ] Test full pipeline end-to-end
- [ ] Set up Portainer for Docker management
- [ ] Set up Uptime Kuma for health monitoring
- [ ] Document all API keys in .env (never hardcode)
- [ ] Write runbook: how to restart each service

---

**XEME OS — The last GTM stack you'll ever pay for.**
**Built with: Rust · Go · Python · TypeScript · Docker**
**License: MIT + AGPL-3.0 (per-component)**
**Version: 2.0 | Updated: 2026-06-05**


---

## LAYER 8 — AEO / GEO Engine (Native to XEME OS)

### Xeme AEO/GEO Engine — `internal/aeo/`
**This is the 5th Xeme engine, built natively into the Go binary.**

**What it does:** Tracks how your brand appears across AI search engines (ChatGPT, Claude, Perplexity, You.com, Phind, Komo) and tells you what to write to get cited more.

**4-pillar AEO strategy:**

1. **Prompt mining** — what customers actually ask AI engines
2. **Mention tracking** — which engines mention your brand (You.com, Phind, Komo work free)
3. **Citation source analysis** — which sources engines cite (so you know what to write)
4. **Content gap detection** — queries where competitors appear but you don't

**Setup:**
```bash
export XEME_AEO_BRAND="Xeme"
export XEME_AEO_DOMAIN="xeme.app"
export XEME_AEO_PROMPTS="best AI GTM tool,open source Clay alternative,self-hosted Salesforce alternative"
export XEME_AEO_COMPETITORS="Clay,Apollo,Instantly,Salesforce"
# Optional: paid API keys for richer coverage
export PERPLEXITY_API_KEY="pplx-..."
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."
```

**API Endpoints:**
- `GET /api/aeo/status` — config + engine list
- `GET /api/aeo/check` — run all prompts across all engines
- `GET /api/aeo/score` — 0-100 AEO score with per-engine breakdown
- `GET /api/aeo/optimize?url=...` — content analysis with issues + recommendations

**MCP Tools (5 new):**
- `xeme_aeo_status` — show config
- `xeme_aeo_check_brand` — run full check
- `xeme_aeo_score` — compute 0-100 score
- `xeme_aeo_optimize` — analyze URL for AEO-readiness
- `xeme_aeo_check_single` — test one prompt/one engine

**Why this matters:**
- Gartner predicts 25% drop in traditional search volume by 2026
- AI engines cite specific sources — if you're not in their training/retrieval corpus, you don't exist
- The brands that get cited by ChatGPT, Perplexity, Google AI Overviews now will own the next decade of GTM visibility

