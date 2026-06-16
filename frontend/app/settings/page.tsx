import Link from 'next/link';

export default function SettingsPage() {
  return (
    <div>
      <div className="flex items-baseline justify-between mb-6">
        <h1 className="text-3xl font-semibold tracking-tight">Settings</h1>
        <Link href="/" className="text-sm text-[var(--fg-dim)]">← Overview</Link>
      </div>
      <section className="card mb-4">
        <h2 className="text-lg font-semibold mb-3">Theme</h2>
        <p className="text-sm text-[var(--fg-dim)] mb-4">Switch between the default minimal style and the xeme.co brand.</p>
        <p className="text-sm">Use the toggle in the top-right of the navigation.</p>
      </section>
      <section className="card mb-4">
        <h2 className="text-lg font-semibold mb-3">Engine endpoints</h2>
        <table className="t">
          <thead><tr><th>Engine</th><th>Binary</th><th>Port</th></tr></thead>
          <tbody>
            <tr><td>Xeme CLI</td><td className="font-mono text-xs">~/Projects/xeme-os/xeme</td><td>—</td></tr>
            <tr><td>Xeme Ledger</td><td className="font-mono text-xs">~/Projects/xeme-os/xeme-ledger-server</td><td>:8088</td></tr>
            <tr><td>Xeme MCP</td><td className="font-mono text-xs">~/Projects/xeme-os/xeme-mcp</td><td>stdio</td></tr>
            <tr><td>Xeme Workflows</td><td className="font-mono text-xs">~/Projects/xeme-os/xeme-workflows</td><td>—</td></tr>
            <tr><td>Xeme Campaigns</td><td className="font-mono text-xs">~/Projects/xeme-os/xeme-campaigns</td><td>—</td></tr>
            <tr><td>Xeme Frontend (this)</td><td className="font-mono text-xs">next dev -p 3847</td><td>:3847</td></tr>
          </tbody>
        </table>
      </section>
      <section className="card">
        <h2 className="text-lg font-semibold mb-3">Environment</h2>
        <pre className="font-mono text-xs bg-[var(--bg-3)] p-3 rounded overflow-x-auto"><code>{`# SMTP (optional — for campaign email send)
export OUTREACH_SMTP_HOST=smtp.gmail.com
export OUTREACH_SMTP_PORT=587
export OUTREACH_SMTP_USER=...
export OUTREACH_SMTP_PASS=...
export OUTREACH_FROM="Sales <sales@example.com>"

# Twilio (optional — for SMS)
export XEME_TWILIO_SID=...
export XEME_TWILIO_AUTH_TOKEN=...
export XEME_TWILIO_FROM=+1...

# AI (optional — for personalization)
export XEME_AI_KEY=...

# Daemon tick intervals
export XEME_WORKFLOWS_INTERVAL=60s
export XEME_CAMPAIGNS_TICK_INTERVAL=60s`}</code></pre>
      </section>
    </div>
  );
}
