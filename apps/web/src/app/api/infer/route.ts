export const runtime = "nodejs"; // ensure Node runtime (server-side)

const API_BASE = process.env.API_BASE ?? "http://localhost:8080";

export async function POST(req: Request) {
  const body = await req.json();
  const upstream = await fetch(`${API_BASE}/api/infer`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

  // If upstream returned an error, forward the body as text
  if (!upstream.ok) {
    const text = await upstream.text();
    return new Response(text, {
      status: upstream.status,
      headers: {
        "content-type": upstream.headers.get("content-type") ?? "text/plain",
      },
    });
  }

  // For successful responses, forward the body as a streamed response when possible.
  // Preserve content-type so the browser/client can handle event-streams or chunked JSON.
  const contentType =
    upstream.headers.get("content-type") ?? "application/json";

  return new Response(upstream.body, {
    status: upstream.status,
    headers: { "content-type": contentType },
  });
}
