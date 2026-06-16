// Xeme OS — basic auth middleware.
// Enable by setting XEME_ADMIN_USER and XEME_ADMIN_PASS env vars.
import { NextResponse, type NextRequest } from 'next/server';

const ADMIN_USER = process.env.XEME_ADMIN_USER || '';
const ADMIN_PASS = process.env.XEME_ADMIN_PASS || '';

export function middleware(req: NextRequest) {
  if (!ADMIN_USER || !ADMIN_PASS) {
    return NextResponse.next();
  }
  // Skip /api/xeme/* (the Go server has its own auth)
  if (req.nextUrl.pathname.startsWith('/api/xeme/')) {
    return NextResponse.next();
  }
  const auth = req.headers.get('authorization');
  if (auth) {
    const [scheme, encoded] = auth.split(' ');
    if (scheme === 'Basic' && encoded) {
      const decoded = atob(encoded);
      const [u, p] = decoded.split(':');
      if (u === ADMIN_USER && p === ADMIN_PASS) {
        return NextResponse.next();
      }
    }
  }
  return new NextResponse('Authentication required', {
    status: 401,
    headers: { 'WWW-Authenticate': 'Basic realm="Xeme OS"' },
  });
}

export const config = { matcher: ['/((?!_next|api/xeme|favicon).*)'] };
