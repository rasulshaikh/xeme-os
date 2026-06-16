# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| v1.3.x  | ✅ Active          |
| < v1.3  | ❌ Not maintained  |

## Reporting a Vulnerability

If you find a security issue, **please don't open a public issue.**

Email: **security@xeme.dev** (or open a private security advisory on GitHub:
https://github.com/rasulshaikh/xeme-os/security/advisories/new)

Include:
- Description of the issue
- Reproduction steps
- Impact (data leak, RCE, etc.)
- Your handle (optional, for credit in the fix)

We aim to respond within **72 hours** and ship a fix within **14 days** for
critical issues.

## What Xeme OS does (and doesn't) handle

**Xeme OS sends your data to third-party APIs you configure.** If you set
`XEME_MOLTSETS_API_KEY`, your data is sent to `api.moltsets.com`. Same for
TheirStack (`api.theirstack.com`), BuiltWith (`api.builtwith.com`), and any
AI engine you query via AEO/GEO.

- ✅ The local dashboard listens on `127.0.0.1` by default. If you bind to
  `0.0.0.0`, **anyone on your network can access it** with no auth. Use a
  reverse proxy (Caddy, nginx) with auth if you do this.
- ✅ The MCP server uses stdio. It cannot be reached over the network.
- ✅ All API keys live in env vars or `~/.xeme/config/xeme.yaml`. They are
  never logged.
- ❌ Xeme OS does **not** encrypt the local SQLite ledger. If you store
  PII, full-disk encryption (FileVault on macOS, LUKS on Linux) is your
  friend.
- ❌ Xeme OS does **not** authenticate API requests to the local dashboard.
  Anyone with localhost access can run pipelines.

## Best practices

- **Never commit API keys.** `.gitignore` excludes `.env`. The install script
  prompts for keys; they live in `~/.xeme/config/xeme.yaml` (chmod 600).
- **Run behind a firewall.** The local dashboard at `:4903` is for dev.
  For production, deploy the binaries on a VPS and put Caddy/nginx in front.
- **Use BYOK.** Don't put your own MoltSets/TheirStack keys in shared
  workspaces. Each user should bring their own.
- **Audit enrichment data.** Scraped emails can be wrong, outdated, or
  malicious. Validate before sending.
