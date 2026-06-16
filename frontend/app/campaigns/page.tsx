import Link from 'next/link';
import { api } from '@/lib/api';

export const dynamic = 'force-dynamic';

const TEMPLATES = [
  { id: '3_email_1_li', name: '3 Email + 1 LinkedIn Touch', steps: 4, days: 10, desc: 'Email → LinkedIn connect → email → email.' },
  { id: '5_email_cold', name: '5 Email Cold', steps: 5, days: 21, desc: 'Longer 5-touch nurture over 3 weeks.' },
  { id: '2_li_1_email_warm', name: '2 LinkedIn + 1 Email Warm', steps: 3, days: 5, desc: 'LinkedIn-first warm touch.' },
];

export default async function CampaignsPage() {
  const campaigns = await api.campaigns().catch(() => []);
  return (
    <div>
      <div className="flex items-baseline justify-between mb-6">
        <h1 className="text-3xl font-semibold tracking-tight">Campaigns</h1>
        <Link href="/" className="text-sm text-[var(--fg-dim)]">← Overview</Link>
      </div>
      <p className="text-sm text-[var(--fg-dim)] mb-6 max-w-xl">
        Multi-step, multi-channel sequences. Per-contact state, auto-stop on reply, SMTP-sendable.
      </p>
      <div className="card mb-6">
        <h2 className="font-semibold mb-2">Manage campaigns</h2>
        <pre className="font-mono text-sm bg-[var(--bg-3)] p-3 rounded overflow-x-auto"><code>{`$ xeme campaign create --name "Q2 CMO" --template 3_email_1_li
$ xeme campaign enroll <id> --in leads.csv
$ xeme campaign track <id>
$ xeme-campaigns --once
$ xeme-campaigns                     # daemon`}</code></pre>
      </div>
      <h2 className="text-lg font-semibold mb-3">Active campaigns</h2>
      <div className="grid gap-3">
        {campaigns.length === 0 ? (
          <div className="card text-center text-[var(--fg-dim)] py-8">No campaigns yet. Create one with <code className="text-xs">xeme campaign create</code>.</div>
        ) : (
          campaigns.map((c) => (
            <Link key={c.id} href={`/campaigns/${c.id}`} className="card hover:border-[var(--accent)] transition-colors block">
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="font-semibold">{c.name}</h3>
                  <p className="text-xs text-[var(--fg-dim)] mt-1">
                    <span className="font-mono">{c.template_id || 'no template'}</span> · {(c.steps || []).length} steps
                  </p>
                </div>
                <span className={'pill ' + (c.status === 'active' ? 'pill-t1' : 'pill-t2')}>{c.status}</span>
              </div>
            </Link>
          ))
        )}
      </div>
      <h2 className="text-lg font-semibold mb-3 mt-10">Templates</h2>
      <div className="grid md:grid-cols-3 gap-3">
        {TEMPLATES.map((t) => (
          <div key={t.id} className="card">
            <h3 className="font-semibold mb-1">{t.name}</h3>
            <p className="text-sm text-[var(--fg-dim)] mb-3">{t.desc}</p>
            <div className="flex gap-2">
              <span className="pill pill-t2">{t.steps} steps</span>
              <span className="pill pill-t3">{t.days} days</span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
