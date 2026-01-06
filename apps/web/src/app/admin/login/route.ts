import { NextResponse } from "next/server";

export async function POST(req: Request) {
  const { token } = (await req.json()) as { token?: string };
  if (!token || token !== process.env.ADMIN_TOKEN) {
    return NextResponse.json({ ok: false }, { status: 401 });
  }

  const res = NextResponse.json({ ok: true });
  res.cookies.set("crowdaudit_admin", token, {
    httpOnly: true,
    sameSite: "lax",
    path: "/",
  });
  return res;
}
