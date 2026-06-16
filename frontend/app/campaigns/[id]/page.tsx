import { api } from '@/lib/api';
import Link from 'next/link';
import { notFound } from 'next/navigation';

export const dynamic = 'force-dynamic';

export default async function CampaignDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let c;
  try { c = await api.campaign(id); } catch (e) { notFound(); }
  if (!c) notFound();

  const steps = c.steps || [];
  const events = c.events || [];
  const contacts = c.contacts || [];

  // Group events by contact for the timeline
  const eventsByContact: Record<string, any[]> = {};
  for (const e of events) {
    if (!eventsByContact[e.email]) eventsByContact[e.email] = [];
    eventsByContact[e.email].push(e);
  }

  return (
    <div>
      <div className="flex items-baseline justify-between mb-6">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">{c.name}</h1>
          <p className="text-sm text-[var(--fg-dim)] mt-1">
            <span className="font-mono">{c.template_id || 'no template'}</span> · {steps.length} steps · {contacts.length} enrolled
          </p>
        </div>
        <Link href="/campaigns" className="text-sm text-[var(--fg-dim)]">← Campaigns</Link>
      </div>

      <section className="grid grid-cols-2 md:grid-cols-4 gap-3 mb-6">
        <Stat label="Status" value={c.status} />
        <Stat label="Enrolled" value={String(contacts.length)} />
        <Stat label="Steps" value={String(steps.length)} />
        <Stat label="Events" value={String(events.length)} />
      </section>

      <section className="card mb-6">
        <h2 className="text-sm font-semibold uppercase tracking-wider text-[var(--fg-dim)] mb-4">Sequence</h2>
        <ol className="space-y-2">
          {steps.map((s: any, i: number) => (
            <li key={s.id} className="flex items-start gap-3 p-3 border border-[var(--border)] rounded-md">
              <span className="w-6 h-6 rounded-full bg-[var(--accent)] text-[var(--bg)] text-xs font-semibold grid place-items-center flex-shrink-0">{i + 1}</span>
              <div className="flex-1">
                <div className="flex items-center gap-2">
                  <span className="font-mono text-xs uppercase tracking-wider text-[var(--accent)]">{s.channel}</span>
                  <span className="font-medium">{s.id}</span>
                  {s.day_offset > 0 && <span className="text-xs text-[var(--fg-dim)]">day +{s.day_offset}</span>}
                </div>
                {s.subject && <div className="text-xs text-[var(--fg-dim)] mt-1">Subject: {s.subject}</div>}
                {s.body && <pre className="text-xs text-[var(--fg-dim)] mt-1 whitespace-pre-wrap font-mono">{s.body.slice(0, 200)}</pre>}
              </div>
            </li>
          ))}
        </ol>
      </section>

      <section className="mb-6">
        <h2 className="text-sm font-semibold uppercase tracking-wider text-[var(--fg-dim)] mb-3">Enrolled contacts</h2>
        <div className="table-wrap">
          <table className="t">
            <thead><tr><th>Email</th><th>Name</th><th>Company</th><th>State</th><th>Step</th><th>Enrolled</th></tr></thead>
            <tbody>
              {contacts.length === 0 ? (
                <tr><td colSpan={6} className="text-center text-[var(--fg-dim)] py-6">No contacts enrolled.</td></tr>
              ) : (
                contacts.map((c: any) => (
                  <tr key={c.id}>
                    <td className="font-mono text-xs">{c.email}</td>
                    <td>{c.first_name} {c.last_name}</td>
                    <td className="text-[var(--fg-dim)]">{c.company || '—'}</td>
                    <td><span className={'pill ' + (c.state === 'replied' || c.state === 'meeting_booked' ? 'pill-t1' : c.state === 'bounced' || c.state === 'stopped' ? 'pill-t3' : 'pill-t2')}>{c.state}</span></td>
                    <td className="tabular-nums">{c.step_index}</td>
                    <td className="text-xs text-[var(--fg-dim)]">{new Date(c.enrolled_at).toLocaleDateString()}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </section>

      <section>
        <h2 className="text-sm font-semibold uppercase tracking-wider text-[var(--fg-dim)] mb-3">Event timeline</h2>
        {events.length === 0 ? (
          <div className="card text-center text-[var(--fg-dim)] py-8">No events yet. Run <code className="text-xs">xeme-campaigns</code> to advance.</div>
        ) : (
          <div className="card space-y-3">
            {events.map((e: any) => (
              <div key={e.id} className="flex items-start gap-3 text-sm">
                <span className="text-xs text-[var(--fg-dim)] tabular-nums w-32 flex-shrink-0">{new Date(e.at).toLocaleString()}</span>
                <span className={'pill ' + (e.event === 'sent' || e.event === 'captured' ? 'pill-t1' : e.event === 'failed' || e.event === 'bounced' ? 'pill-t3' : 'pill-t2')}>{e.event}</span>
                <span className="font-mono text-xs">{e.email}</span>
                <span className="text-xs text-[var(--fg-dim)]">· {e.step_id} · {e.channel}</span>
                {e.payload && <span className="text-xs text-[var(--fg-dim)] truncate">— {e.payload}</span>}
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="card">
      <div className="text-xs uppercase tracking-wider text-[var(--fg-dim)]">{label}</div>
      <div className="text-2xl font-semibold mt-1 truncate">{value}</div>
    </div>
  );
}
