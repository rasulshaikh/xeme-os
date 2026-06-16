import { api } from '@/lib/api';
import Link from 'next/link';

export const dynamic = 'force-dynamic';

export default async function CompaniesPage() {
  const companies = await api.companies().catch(() => []);
  return (
    <div>
      <div className="flex items-baseline justify-between mb-6">
        <h1 className="text-3xl font-semibold tracking-tight">Companies</h1>
        <Link href="/" className="text-sm text-[var(--fg-dim)]">← Overview</Link>
      </div>
      <div className="table-wrap">
        <table className="t">
          <thead><tr><th>Domain</th><th>Name</th><th>Industry</th><th>Size</th><th>Added</th></tr></thead>
          <tbody>
            {companies.length === 0 ? (
              <tr><td colSpan={5} className="text-center text-[var(--fg-dim)] py-10">No companies yet — auto-extracted from email domains when contacts are upserted.</td></tr>
            ) : (
              companies.map((c) => (
                <tr key={c.id}>
                  <td className="font-mono text-sm">{c.domain}</td>
                  <td>{c.name || '—'}</td>
                  <td>{c.industry || '—'}</td>
                  <td>{c.size || '—'}</td>
                  <td className="text-xs text-[var(--fg-dim)]">{new Date(c.created_at).toLocaleDateString()}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
