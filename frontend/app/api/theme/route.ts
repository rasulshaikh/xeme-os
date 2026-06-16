// POST /api/theme — sets the xeme-theme cookie and redirects back.
import { cookies } from 'next/headers';
import { NextResponse } from 'next/server';

export async function POST(req: Request) {
  const form = await req.formData();
  const theme = form.get('theme');
  if (theme !== 'xeme' && theme !== 'deepline') {
    return new NextResponse('bad theme', { status: 400 });
  }
  const url = new URL(req.url);
  const back = url.searchParams.get('back') || '/';
  const res = NextResponse.redirect(new URL(back, req.url), { status: 303 });
  res.cookies.set('xeme-theme', theme, { path: '/', maxAge: 60 * 60 * 24 * 365 });
  return res;
}
