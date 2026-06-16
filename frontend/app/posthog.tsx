'use client';
// PostHog analytics — auto-tracks page views + button clicks.
// Enable by setting NEXT_PUBLIC_POSTHOG_KEY env var.
import { useEffect } from 'react';
import { usePathname } from 'next/navigation';

const POSTHOG_KEY = process.env.NEXT_PUBLIC_POSTHOG_KEY;
const POSTHOG_HOST = process.env.NEXT_PUBLIC_POSTHOG_HOST || 'https://us.i.posthog.com';

declare global {
  interface Window { posthog?: any; }
}

export function PostHog() {
  const pathname = usePathname();
  useEffect(() => {
    if (!POSTHOG_KEY || typeof window === 'undefined') return;
    if ((window as any).posthog) return;
    const s = document.createElement('script');
    s.async = true;
    s.src = `${POSTHOG_HOST}/static/array.js`;
    s.onload = () => {
      (window as any).posthog?.init(POSTHOG_KEY, { api_host: POSTHOG_HOST, capture_pageview: true });
      (window as any).posthog?.capture('$pageview', { path: pathname });
    };
    document.head.appendChild(s);
  }, [pathname]);
  return null;
}
