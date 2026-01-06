import { NextResponse } from "next/server";

export async function GET(req: Request) {
  const url = new URL(req.url);

  // forward query params
  const qs = url.searchParams.toString();

  // Point this at your Go API
  // e.g. http://localhost:8080 in dev, http://inference-api:8080 in docker, etc.
  const base = process.env.INFERENCE_API_URL ?? "http://localhost:8080";

  const upstream = `${base}/api/search/pairs${qs ? `?${qs}` : ""}`;

  const res = await fetch(upstream, {
    // keep it "live"
    cache: "no-store",
    headers: {
      // forward a couple basics (optional)
      accept: "application/json",
    },
  });

  const text = await res.text(); // preserve error bodies

  return new NextResponse(text, {
    status: res.status,
    headers: {
      "content-type": res.headers.get("content-type") ?? "application/json",
    },
  });
}
