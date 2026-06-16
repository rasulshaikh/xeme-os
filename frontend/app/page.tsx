import { api } from '@/lib/api';
import Link from 'next/link';

export const dynamic = 'force-dynamic';

export default async function Home() {
  const [stats, contacts, companies, health] = await Promise.all([
    api.stats().catch(() => null),
    api.contacts({ limit: 8 }).catch(() => []),
    api.companies().catch(() => []),
    api.health().catch(() => null),
  ]);

  const engineOk = health?.ok ?? false;
  const c = stats?.contacts ?? 0;
  const t1 = stats?.by_tier?.['Tier 1 - Hot'] ?? 0;
  const t2 = stats?.by_tier?.['Tier 2 - Warm'] ?? 0;
  const t3 = stats?.by_tier?.['Tier 3 - Nurture'] ?? 0;

  return (
    <div className="space-y-12">
      <section>
        <div className="label mb-3">Xeme OS · v1.1</div>
        <h1 className="text-5xl font-semibold leading-[1.05] tracking-tight max-w-2xl mb-3">
          The Xeme Ledger — your GTM data, owned.
        </h1>
        <p className="text-lg text-[var(--fg-dim)] max-w-xl">
          A local-first CRM and contact store. Pattern-match emails, ICP-score leads, and persist results to a SQLite database you own. Built in pure Go.
        </p>
        <div className="mt-4 flex items-center gap-3">
          <span className={'pill ' + (engineOk ? 'pill-t1' : 'pill-t3')}>
            {engineOk ? '● all engines OK' : '○ engines degraded'}
          </span>
          <span className="text-sm text-[var(--fg-dim)] font-mono">http://localhost:8088</span>
        </div>
      </section>

      <section className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-6 border-y border-[var(--border)]">
        <Stat label="Contacts" value={c} sub={`${stats?.companies ?? 0} companies`} />
        <Stat label="Deals" value={stats?.deals ?? 0} sub="in pipeline" />
        <Stat label="Outcomes" value={stats?.outcomes ?? 0} sub="events logged" />
        <Stat label="T1 Hot" value={t1} sub="ready to send" />
        <Stat label="T2 Warm" value={t2} sub="nurture" />
        <Stat label="T3 Nurture" value={t3} sub="content drip" />
      </section>

      <section>
        <div className="flex items-baseline justify-between mb-4">
          <h2 className="text-xl font-semibold tracking-tight">Recent contacts</h2>
          <Link href="/contacts" className="text-sm text-[var(--fg-dim)] hover:text-[var(--fg)]">View all →</Link>
        </div>
        <div className="table-wrap">
          <table className="t">
            <thead>
              <tr>
                <th>Name</th><th>Title</th><th>Email</th><th>Score</th><th>Tier</th><th>Source</th>
              </tr>
            </thead>
            <tbody>
              {contacts.length === 0 ? (
                <tr><td colSpan={6} className="text-center text-[var(--fg-dim)] py-10">No contacts yet. POST to /v1/contacts or run <code className="text-xs">xeme pipe</code>.</td></tr>
              ) : (
                contacts.map((c) => (
                  <tr key={c.id}>
                    <td className="font-medium">{c.first_name} {c.last_name}</td>
                    <td>{c.job_title}</td>
                    <td className="text-[var(--fg-dim)]"><a href={`mailto:${c.email}`} className="hover:underline">{c.email}</a></td>
                    <td className="tabular-nums font-semibold">{c.score}</td>
                    <td><span className={'pill ' + tierClass(c.tier)}>{c.tier}</span></td>
                    <td className="text-[var(--fg-dim)] text-xs">{c.source}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </section>

      <section className="grid md:grid-cols-3 gap-4">
        <ModuleCard href="/workflows" title="Workflows" desc="DAG-based automation. Schedule, chain, and orchestrate." />
        <ModuleCard href="/campaigns" title="Campaigns" desc="Multi-step, multi-channel sequences. Auto-stop on reply." />
        <ModuleCard href="/contacts" title="Contacts" desc="CRM and contact store. SQLite-backed. Your data." />
      </section>
    </div>
  );
}

function Stat({ label, value, sub }: { label: string; value: number; sub?: string }) {
  return (
    <div className="px-5 py-6 border-r border-[var(--border)] last:border-r-0">
      <div className="label">{label}</div>
      <div className="stat-num mt-2">{value}</div>
      {sub && <div className="text-xs text-[var(--fg-soft)] mt-1">{sub}</div>}
    </div>
  );
}

function ModuleCard({ href, title, desc }: { href: string; title: string; desc: string }) {
  return (
    <Link href={href} className="card hover:border-[var(--accent)] transition-colors block">
      <h3 className="text-lg font-semibold mb-1 tracking-tight">{title}</h3>
      <p className="text-sm text-[var(--fg-dim)]">{desc}</p>
    </Link>
  );
}

function tierClass(tier: string): string {
  if (tier === 'Tier 1 - Hot') return 'pill-t1';
  if (tier === 'Tier 2 - Warm') return 'pill-t2';
  return 'pill-t3';
}
