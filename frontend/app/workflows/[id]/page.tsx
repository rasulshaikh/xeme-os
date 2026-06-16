import { api } from '@/lib/api';
import Link from 'next/link';
import { notFound } from 'next/navigation';

export const dynamic = 'force-dynamic';

export default async function WorkflowDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let wf;
  try {
    wf = await api.workflow(id);
  } catch (e) { notFound(); }
  if (!wf) notFound();

  // Build the DAG from nodes
  const nodes = wf.nodes || [];
  const byID: Record<string, any> = {};
  for (const n of nodes) byID[n.id] = n;
  // Calculate levels (topo-sort approximation)
  const levels: string[][] = [];
  const placed: Set<string> = new Set();
  while (placed.size < nodes.length) {
    const level: string[] = [];
    for (const n of nodes) {
      if (placed.has(n.id)) continue;
      const deps = n.depends_on || [];
      if (deps.every((d: string) => placed.has(d))) level.push(n.id);
    }
    if (level.length === 0) break;
    levels.push(level);
    for (const id of level) placed.add(id);
  }

  return (
    <div>
      <div className="flex items-baseline justify-between mb-6">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">{wf.name}</h1>
          {wf.description && <p className="text-sm text-[var(--fg-dim)] mt-1">{wf.description}</p>}
        </div>
        <Link href="/workflows" className="text-sm text-[var(--fg-dim)]">← Workflows</Link>
      </div>

      <section className="card mb-6">
        <h2 className="text-sm font-semibold uppercase tracking-wider text-[var(--fg-dim)] mb-4">DAG ({nodes.length} nodes)</h2>
        <div className="space-y-3">
          {levels.map((level, i) => (
            <div key={i} className="flex items-center gap-3">
              <span className="text-[10px] uppercase tracking-wider text-[var(--fg-dim)] w-12">L{i}</span>
              {level.map((id) => {
                const n = byID[id];
                return (
                  <div key={id} className="px-3 py-2 border border-[var(--border)] rounded-md bg-[var(--bg-3)] text-sm">
                    <div className="font-mono text-xs text-[var(--accent)]">{n.type}</div>
                    <div>{n.id}</div>
                  </div>
                );
              })}
            </div>
          ))}
        </div>
      </section>

      <section>
        <h2 className="text-sm font-semibold uppercase tracking-wider text-[var(--fg-dim)] mb-3">Run history</h2>
        {wf.runs && wf.runs.length > 0 ? (
          <div className="table-wrap">
            <table className="t">
              <thead><tr><th>Run ID</th><th>Status</th><th>Started</th><th>Duration</th><th>Error</th></tr></thead>
              <tbody>
                {wf.runs.map((r: any) => (
                  <tr key={r.id}>
                    <td className="font-mono text-xs">{r.id.slice(0, 24)}…</td>
                    <td><span className={'pill ' + (r.status === 'completed' ? 'pill-t1' : r.status === 'failed' ? 'pill-t3' : 'pill-t2')}>{r.status}</span></td>
                    <td className="text-xs text-[var(--fg-dim)]">{new Date(r.started_at).toLocaleString()}</td>
                    <td className="text-xs text-[var(--fg-dim)]">
                      {r.started_at && r.finished_at ? `${Math.round((new Date(r.finished_at).getTime() - new Date(r.started_at).getTime()) / 1000)}s` : '—'}
                    </td>
                    <td className="text-xs text-[var(--red)]">{r.error || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="card text-center text-[var(--fg-dim)] py-8">No runs yet. Execute via <code className="text-xs">xeme workflow run {wf.id}.json</code></div>
        )}
      </section>
    </div>
  );
}
