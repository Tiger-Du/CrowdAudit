export const runtime = "nodejs";

const API_BASE = process.env.API_BASE ?? "http://localhost:8080";

export async function POST(req: Request) {
  const body = await req.text(); // preserve exact JSON
  const res = await fetch(`${API_BASE}/api/votes`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body,
  });

  const text = await res.text();
  return new Response(text, {
    status: res.status,
    headers: {
      "Content-Type": res.headers.get("Content-Type") ?? "application/json",
    },
  });
}
