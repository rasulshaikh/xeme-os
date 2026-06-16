import { api } from '@/lib/api';
import { SearchBar } from '@/components/SearchBar';
import Link from 'next/link';

export const dynamic = 'force-dynamic';

export default async function ContactsPage({ searchParams }: { searchParams: Promise<Record<string, string>> }) {
  const sp = await searchParams;
  const minScore = sp.min_score ? parseInt(sp.min_score) : 0;
  const contacts = await api.contacts({ q: sp.q, min_score: minScore, limit: 100 }).catch(() => []);
  return (
    <div>
      <div className="flex items-baseline justify-between mb-6">
        <h1 className="text-3xl font-semibold tracking-tight">Contacts</h1>
        <Link href="/" className="text-sm text-[var(--fg-dim)]">← Overview</Link>
      </div>
      <SearchBar
        placeholder="Search by name, email, title..."
        filters={[{ key: 'min_score', label: 'min score', values: ['80', '70', '60', '50'] }]}
      />
      <div className="table-wrap">
        <table className="t">
          <thead><tr><th>Name</th><th>Title</th><th>Email</th><th>Score</th><th>Tier</th><th>Source</th><th>Created</th></tr></thead>
          <tbody>
            {contacts.length === 0 ? (
              <tr><td colSpan={7} className="text-center text-[var(--fg-dim)] py-10">No contacts found.</td></tr>
            ) : (
              contacts.map((c) => (
                <tr key={c.id}>
                  <td className="font-medium"><Link href={`/contacts/${c.id}`} className="hover:underline">{c.first_name} {c.last_name}</Link></td>
                  <td>{c.job_title}</td>
                  <td className="text-[var(--fg-dim)]">{c.email}</td>
                  <td className="tabular-nums">{c.score}</td>
                  <td><span className="pill pill-t2">{c.tier}</span></td>
                  <td className="text-xs text-[var(--fg-dim)]">{c.source}</td>
                  <td className="text-xs text-[var(--fg-dim)]">{new Date(c.created_at).toLocaleDateString()}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
      <p className="mt-4 text-xs text-[var(--fg-dim)]">{contacts.length} contact{contacts.length === 1 ? '' : 's'}</p>
    </div>
  );
}
