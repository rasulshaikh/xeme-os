import './globals.css';
import type { Metadata } from 'next';
import Link from 'next/link';
import { cookies, headers } from 'next/headers';
import { PostHog } from './posthog';
import { authEnabled } from '@/lib/api';

export const metadata: Metadata = {
  title: 'Xeme OS — AI-Native GTM',
  description: 'Operating system for AI-native GTM teams.',
};

const NAV = [
  { href: '/', label: 'Overview' },
  { href: '/workflows', label: 'Workflows' },
  { href: '/campaigns', label: 'Campaigns' },
  { href: '/contacts', label: 'Contacts' },
  { href: '/companies', label: 'Companies' },
  { href: '/stats', label: 'Stats' },
  { href: '/settings', label: 'Settings' },
];

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const theme = (await cookies()).get('xeme-theme')?.value || 'deepline';
  const ws = (await cookies()).get('xeme-ws')?.value || 'default';
  return (
    <html lang="en" data-theme={theme} data-ws={ws} suppressHydrationWarning>
      <head>
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="" />
        <link
          rel="stylesheet"
          href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&family=Cormorant+Garamond:wght@400;500;600&display=swap"
        />
      </head>
      <body>
        <PostHog />
        <header className="border-b border-[var(--border)]">
          <div className="max-w-6xl mx-auto px-6 py-3 flex items-center justify-between">
            <Link href="/" className="flex items-center gap-2 font-semibold text-[var(--fg)]">
              <span className="w-6 h-6 grid place-items-center rounded-md bg-[var(--accent)] text-[var(--bg)] text-[11px] font-bold tracking-tighter">X</span>
              <span>Xeme</span>
              {theme === 'xeme' && (
                <span className="ml-2 text-[10px] uppercase tracking-wighter text-[var(--accent)] font-medium">xeme.co</span>
              )}
            </Link>
            <nav className="flex items-center gap-1">
              {NAV.map((n) => (
                <Link key={n.href} href={n.href} className="nav-link">{n.label}</Link>
              ))}
              <form action="/api/theme" method="post" className="ml-3 inline-flex border border-[var(--border)] rounded-full p-0.5 bg-[var(--bg-2)]">
                <ThemeButton active={theme === 'deepline'} value="deepline" label="Deepline" />
                <ThemeButton active={theme === 'xeme'} value="xeme" label="xeme.co" />
              </form>
            </nav>
          </div>
        </header>
        <main className="max-w-6xl mx-auto px-6 py-10">{children}</main>
        <footer className="max-w-6xl mx-auto px-6 py-10 mt-20 border-t border-[var(--border)] text-xs text-[var(--fg-dim)] flex justify-between">
          <span>
            Xeme OS v1.2 · single static Go binary + Next.js frontend
            {authEnabled() && <span className="ml-2 text-[var(--green)]">● auth enabled</span>}
          </span>
          <span className="font-mono">GET /v1/stats</span>
        </footer>
      </body>
    </html>
  );
}

function ThemeButton({ active, value, label }: { active: boolean; value: string; label: string }) {
  return (
    <button
      type="submit"
      name="theme"
      value={value}
      className={'text-[11px] font-medium px-2.5 py-1 rounded-full transition-colors ' + (active ? 'bg-[var(--accent)] text-[var(--bg)]' : 'text-[var(--fg-dim)] hover:text-[var(--fg)]')}
    >
      {label}
    </button>
  );
}
