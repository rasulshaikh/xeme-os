'use client';
import { useRouter, useSearchParams } from 'next/navigation';
import { useState, useEffect } from 'react';

export function SearchBar({ placeholder = 'Search...', filters = [] as { key: string; label: string; values: string[] }[] }) {
  const router = useRouter();
  const params = useSearchParams();
  const [q, setQ] = useState(params.get('q') || '');

  useEffect(() => { setQ(params.get('q') || ''); }, [params]);

  function update(next: Record<string, string>) {
    const sp = new URLSearchParams(params);
    Object.entries(next).forEach(([k, v]) => {
      if (v) sp.set(k, v); else sp.delete(k);
    });
    router.push(`?${sp.toString()}`);
  }

  return (
    <form onSubmit={(e) => { e.preventDefault(); update({ q }); }} className="flex items-center gap-2 mb-4">
      <input
        type="text"
        value={q}
        onChange={(e) => setQ(e.target.value)}
        placeholder={placeholder}
        className="flex-1 max-w-sm px-3 py-1.5 text-sm border border-[var(--border)] rounded-md bg-[var(--bg)] focus:outline-none focus:border-[var(--accent)]"
      />
      <button type="submit" className="btn">Search</button>
      {filters.map((f) => (
        <select
          key={f.key}
          value={params.get(f.key) || ''}
          onChange={(e) => update({ [f.key]: e.target.value })}
          className="text-sm border border-[var(--border)] rounded-md px-2 py-1.5 bg-[var(--bg)]"
        >
          <option value="">All {f.label}</option>
          {f.values.map((v) => <option key={v} value={v}>{v}</option>)}
        </select>
      ))}
      {Array.from(params.entries()).length > 0 && (
        <a href="?" className="text-xs text-[var(--fg-dim)] hover:text-[var(--fg)]">Clear</a>
      )}
    </form>
  );
}
