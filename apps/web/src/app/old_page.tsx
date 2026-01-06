"use client";

import { useEffect, useMemo, useState } from "react";
import { Button } from "@/components/ui/button";

type PairDTO = {
  pairId: number;
  promptId: number;
  title: string;
  prompt: string;
  a: ResponseDTO;
  b: ResponseDTO;
};

type ResponseDTO = {
  responseId: number;
  provider: string;
  model: string;
  content: string;
};

function getOrCreateVoterId(): string {
  const key = "crowdaudit_voter_id";
  const existing = localStorage.getItem(key);
  if (existing) return existing;

  // lightweight anonymous id (good enough for prototype)
  const id = crypto.randomUUID();
  localStorage.setItem(key, id);
  return id;
}

export default function Home() {
  const [pair, setPair] = useState<PairDTO | null>(null);
  const [loading, setLoading] = useState(true);
  const [msg, setMsg] = useState<string>("");

  const voterId = useMemo(() => {
    if (typeof window === "undefined") return "";
    return getOrCreateVoterId();
  }, []);

  async function loadPair() {
    setLoading(true);
    setMsg("");
    const res = await fetch(`/api/pairs/random`, { cache: "no-store" });
    if (!res.ok) {
      setPair(null);
      setLoading(false);
      setMsg("No pairs available yet.");
      return;
    }
    const data = (await res.json()) as PairDTO;
    setPair(data);
    setLoading(false);
  }

  async function vote(choice: "A" | "B" | "TIE") {
    if (!pair) return;
    setMsg("");

    const res = await fetch(`/api/votes`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ pairId: pair.pairId, voterId, choice }),
    });

    if (!res.ok) {
      setMsg("Vote failed. Try again.");
      return;
    }

    const data = (await res.json()) as { status: "recorded" | "duplicate" };
    setMsg(
      data.status === "duplicate"
        ? "You already voted on this one."
        : "Vote recorded!",
    );
    await loadPair();
  }

  useEffect(() => {
    let cancelled = false;

    (async () => {
      setLoading(true);
      setMsg("");
      try {
        const res = await fetch(`/api/pairs/random`, {
          cache: "no-store",
        });
        if (!res.ok) {
          if (!cancelled) {
            setPair(null);
            setMsg("No pairs available yet.");
          }
          return;
        }
        const data = (await res.json()) as PairDTO;
        if (!cancelled) setPair(data);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, []);

  if (loading) return null;

  return (
    <main style={{ padding: 24, maxWidth: 1100, margin: "0 auto" }}>
      <h1 style={{ fontSize: 28, fontWeight: 700 }}>CrowdAudit</h1>

      {msg && <p style={{ marginTop: 12 }}>{msg}</p>}

      {!pair ? (
        <p style={{ marginTop: 16 }}>No pairs found.</p>
      ) : (
        <>
          <section style={{ marginTop: 16 }}>
            <h2 style={{ fontSize: 18, fontWeight: 600 }}>{pair.title}</h2>
            <p style={{ marginTop: 8, whiteSpace: "pre-wrap" }}>
              {pair.prompt}
            </p>
          </section>

          <section
            style={{
              display: "grid",
              gridTemplateColumns: "1fr 1fr",
              gap: 16,
              marginTop: 16,
            }}
          >
            <Card title={`A`} body={pair.a.content} />
            <Card title={`B`} body={pair.b.content} />
          </section>

          <section style={{ display: "flex", gap: 12, marginTop: 16 }}>
            <Button variant="outline" onClick={() => vote("A")}>
              A is better
            </Button>
            <Button variant="outline" onClick={() => vote("TIE")}>
              Tie
            </Button>
            <Button variant="outline" onClick={() => vote("B")}>
              B is better
            </Button>
            <Button onClick={loadPair} style={{ marginLeft: "auto" }}>
              Skip
            </Button>
          </section>
        </>
      )}
    </main>
  );
}

function Card({ title, body }: { title: string; body: string }) {
  return (
    <div style={{ border: "1px solid #ddd", borderRadius: 12, padding: 16 }}>
      <div style={{ fontWeight: 700 }}>{title}</div>
      <pre
        style={{
          marginTop: 10,
          whiteSpace: "pre-wrap",
          fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
        }}
      >
        {body}
      </pre>
    </div>
  );
}
