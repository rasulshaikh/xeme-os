import { api } from '@/lib/api';
import Link from 'next/link';
import { notFound } from 'next/navigation';

export const dynamic = 'force-dynamic';

export default async function ContactDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let contact;
  try {
    contact = await api.contact(parseInt(id));
  } catch (e) {
    notFound();
  }
  if (!contact) notFound();
  return (
    <div>
      <div className="flex items-baseline justify-between mb-6">
        <h1 className="text-3xl font-semibold tracking-tight">{contact.first_name} {contact.last_name}</h1>
        <Link href="/contacts" className="text-sm text-[var(--fg-dim)]">← Contacts</Link>
      </div>

      <div className="grid md:grid-cols-2 gap-6 mb-8">
        <div className="card">
          <h2 className="text-sm font-semibold uppercase tracking-wider text-[var(--fg-dim)] mb-3">Profile</h2>
          <dl className="space-y-2 text-sm">
            <div className="flex justify-between"><dt className="text-[var(--fg-dim)]">Title</dt><dd className="font-medium">{contact.job_title || '—'}</dd></div>
            <div className="flex justify-between"><dt className="text-[var(--fg-dim)]">Email</dt><dd><a href={`mailto:${contact.email}`} className="hover:underline">{contact.email}</a></dd></div>
            <div className="flex justify-between"><dt className="text-[var(--fg-dim)]">LinkedIn</dt><dd>{contact.linkedin_url ? <a href={contact.linkedin_url} className="hover:underline text-xs">{contact.linkedin_url}</a> : '—'}</dd></div>
            <div className="flex justify-between"><dt className="text-[var(--fg-dim)]">Source</dt><dd className="text-xs">{contact.source || '—'}</dd></div>
          </dl>
        </div>
        <div className="card">
          <h2 className="text-sm font-semibold uppercase tracking-wider text-[var(--fg-dim)] mb-3">Score</h2>
          <div className="text-5xl font-semibold tabular-nums tracking-tight">{contact.score}</div>
          <div className="mt-2"><span className="pill pill-t2">{contact.tier}</span></div>
          <div className="mt-4 text-xs text-[var(--fg-dim)]">Created {new Date(contact.created_at).toLocaleString()}</div>
        </div>
      </div>

      <div className="card">
        <h2 className="text-sm font-semibold uppercase tracking-wider text-[var(--fg-dim)] mb-3">Activity</h2>
        <p className="text-sm text-[var(--fg-dim)]">No events yet. Activity will appear here as this contact is enrolled in workflows or campaigns.</p>
      </div>
    </div>
  );
}
