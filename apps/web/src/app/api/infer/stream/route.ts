export const runtime = "nodejs";

const OPENROUTER_BASE =
  process.env.OPENROUTER_BASE_URL ?? "https://openrouter.ai/api/v1";
const API_KEY = process.env.OPENROUTER_API_KEY ?? "";

export async function POST(req: Request) {
  if (!API_KEY) {
    return new Response("OPENROUTER_API_KEY not configured", {
      status: 500,
    });
  }

  const body = await req.json();

  // Ensure streaming is requested
  const upstreamBody = {
    model: body.model ?? "openai/gpt-4o",
    messages: [{ role: "user", content: body.prompt }],
    stream: true,
  };

  const res = await fetch(`${OPENROUTER_BASE}/chat/completions`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${API_KEY}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify(upstreamBody),
  });

  if (!res.ok) {
    const text = await res.text();
    return new Response(text, {
      status: res.status,
      headers: {
        "content-type": res.headers.get("content-type") ?? "text/plain",
      },
    });
  }

  // Forward the streaming body to the client, preserving content type
  return new Response(res.body, {
    status: res.status,
    headers: {
      "content-type": res.headers.get("content-type") ?? "text/event-stream",
    },
  });
}
