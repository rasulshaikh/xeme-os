import { api } from '@/lib/api';
import Link from 'next/link';

export const dynamic = 'force-dynamic';

export default async function StatsPage() {
  const stats = await api.stats().catch(() => null);
  if (!stats) {
    return <div className="text-[var(--fg-dim)]">Stats API unreachable. Is the Xeme Ledger server running on :8088?</div>;
  }
  const tierEntries = Object.entries(stats.by_tier).sort((a, b) => b[1] - a[1]);
  const signalEntries = Object.entries(stats.by_signal).sort((a, b) => b[1] - a[1]);
  const maxTier = Math.max(1, ...tierEntries.map(([, n]) => n));
  const maxSig = Math.max(1, ...signalEntries.map(([, n]) => n));

  return (
    <div>
      <div className="flex items-baseline justify-between mb-6">
        <h1 className="text-3xl font-semibold tracking-tight">Stats</h1>
        <Link href="/" className="text-sm text-[var(--fg-dim)]">← Overview</Link>
      </div>
      <section className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-10">
        <Big label="Contacts" value={stats.contacts} />
        <Big label="Companies" value={stats.companies} />
        <Big label="Deals" value={stats.deals} />
        <Big label="Outcomes" value={stats.outcomes} />
      </section>
      <section className="card mb-6">
        <h2 className="text-lg font-semibold mb-4">By tier</h2>
        {tierEntries.length === 0 ? (
          <p className="text-sm text-[var(--fg-dim)]">No tiered contacts yet.</p>
        ) : (
          <div className="space-y-2">
            {tierEntries.map(([tier, n]) => (
              <div key={tier} className="flex items-center gap-3">
                <span className="w-32 text-sm">{tier}</span>
                <div className="flex-1 h-6 bg-[var(--bg-3)] rounded overflow-hidden">
                  <div className="h-full bg-[var(--accent)]" style={{ width: `${(n / maxTier) * 100}%` }} />
                </div>
                <span className="w-8 text-right tabular-nums text-sm font-semibold">{n}</span>
              </div>
            ))}
          </div>
        )}
      </section>
      <section className="card">
        <h2 className="text-lg font-semibold mb-4">By signal source</h2>
        {signalEntries.length === 0 ? (
          <p className="text-sm text-[var(--fg-dim)]">No signals recorded yet.</p>
        ) : (
          <div className="space-y-2">
            {signalEntries.map(([sig, n]) => (
              <div key={sig} className="flex items-center gap-3">
                <span className="w-40 text-sm truncate">{sig}</span>
                <div className="flex-1 h-6 bg-[var(--bg-3)] rounded overflow-hidden">
                  <div className="h-full bg-[var(--accent-2)]" style={{ width: `${(n / maxSig) * 100}%` }} />
                </div>
                <span className="w-8 text-right tabular-nums text-sm font-semibold">{n}</span>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

function Big({ label, value }: { label: string; value: number }) {
  return (
    <div className="card">
      <div className="label">{label}</div>
      <div className="stat-num mt-2">{value}</div>
    </div>
  );
}
