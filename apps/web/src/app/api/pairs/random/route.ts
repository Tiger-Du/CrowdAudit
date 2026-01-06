export const runtime = "nodejs"; // important: ensures Node runtime, not Edge

const API_BASE = process.env.API_BASE ?? "http://localhost:8080";

export async function GET() {
  const res = await fetch(`${API_BASE}/api/pairs/random`, {
    cache: "no-store",
  });

  if (!res.ok) {
    return new Response(await res.text(), { status: res.status });
  }

  return Response.json(await res.json());
}
