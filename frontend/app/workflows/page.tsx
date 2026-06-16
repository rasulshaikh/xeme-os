import Link from 'next/link';
import { api } from '@/lib/api';

export const dynamic = 'force-dynamic';

export default async function WorkflowsPage() {
  const workflows = await api.workflows().catch(() => []);
  return (
    <div>
      <div className="flex items-baseline justify-between mb-6">
        <h1 className="text-3xl font-semibold tracking-tight">Workflows</h1>
        <Link href="/" className="text-sm text-[var(--fg-dim)]">← Overview</Link>
      </div>
      <p className="text-sm text-[var(--fg-dim)] mb-6 max-w-xl">
        DAG-based automation. Each workflow chains any of the Xeme engines (signal, enrich, ledger, ai, intel) into a multi-step pipeline.
      </p>
      <div className="card mb-4">
        <h2 className="font-semibold mb-2">Run a workflow</h2>
        <pre className="font-mono text-sm bg-[var(--bg-3)] p-3 rounded overflow-x-auto"><code>{`$ xeme workflow run path/to/workflow.json
$ xeme workflow list
$ xeme workflow status <run-id>`}</code></pre>
      </div>
      <h2 className="text-lg font-semibold mb-3">Saved workflows</h2>
      <div className="grid gap-3">
        {workflows.length === 0 ? (
          <div className="card text-center text-[var(--fg-dim)] py-8">No workflows yet. Run <code className="text-xs">xeme workflow run</code> on a JSON file to save it.</div>
        ) : (
          workflows.map((w) => (
            <Link key={w.id} href={`/workflows/${w.id}`} className="card hover:border-[var(--accent)] transition-colors block">
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="font-semibold">{w.name}</h3>
                  {w.description && <p className="text-xs text-[var(--fg-dim)] mt-1">{w.description}</p>}
                  <p className="text-xs text-[var(--fg-soft)] mt-1 font-mono">{w.id}</p>
                </div>
                <span className="pill pill-t2">{(w.nodes || []).length} nodes</span>
              </div>
            </Link>
          ))
        )}
      </div>
    </div>
  );
}
