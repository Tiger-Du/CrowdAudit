"use client";

import { useEffect, useMemo, useState } from "react";
import { searchPairs, type SearchPairItem, type SortMode } from "@/lib/search";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

function clampText(s: string, n: number) {
  if (s.length <= n) return s;
  return s.slice(0, n - 1) + "…";
}

export function PairSearch() {
  const [q, setQ] = useState("");
  const [sort, setSort] = useState<SortMode>("disagreement");
  const [items, setItems] = useState<SearchPairItem[]>([]);
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  async function runSearch(reset: boolean) {
    setLoading(true);
    setErr(null);
    try {
      const data = await searchPairs({
        q: q.trim() || undefined,
        sort,
        cursor: reset ? undefined : cursor,
        limit: 10,
        visibility: "public",
      });

      setItems((prev) => (reset ? data.items : [...prev, ...data.items]));
      setCursor(data.nextCursor);
    } catch (e: unknown) {
      const message =
        e instanceof Error
          ? e.message
          : typeof e === "string"
            ? e
            : "search failed";
      setErr(message);
    } finally {
      setLoading(false);
    }
  }

  // initial load
  useEffect(() => {
    runSearch(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sort]);

  const canLoadMore = useMemo(() => !!cursor && !loading, [cursor, loading]);

  return (
    <div className="space-y-3">
      <div className="flex flex-col gap-2 md:flex-row md:items-center">
        <Input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder='Search e.g. "cap theorem", "rate limiting", "redis"...'
        />
        <div className="flex gap-2">
          <Button
            variant={sort === "relevance" ? "default" : "outline"}
            onClick={() => setSort("relevance")}
          >
            Relevance
          </Button>
          <Button
            variant={sort === "disagreement" ? "default" : "outline"}
            onClick={() => setSort("disagreement")}
          >
            Disagreement
          </Button>
          <Button
            variant={sort === "recent" ? "default" : "outline"}
            onClick={() => setSort("recent")}
          >
            Recent
          </Button>
          <Button onClick={() => runSearch(true)} disabled={loading}>
            Search
          </Button>
        </div>
      </div>

      {err && (
        <div className="rounded-md border p-3 text-sm">
          <div className="font-medium">Search error</div>
          <div className="opacity-80 break-words">{err}</div>
        </div>
      )}

      <div className="grid gap-3">
        {items.map((it) => (
          <Card key={it.pairId}>
            <CardHeader className="space-y-1">
              <CardTitle className="text-base">{it.title}</CardTitle>
              <div className="flex flex-wrap gap-2">
                <Badge variant="secondary">pair: {it.pairId}</Badge>
                <Badge variant="secondary">votes: {it.votes.total}</Badge>
                <Badge variant="secondary">
                  disagree: {it.disagreementScore.toFixed(3)}
                </Badge>
              </div>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="text-sm opacity-90">{it.prompt}</div>

              <div className="grid gap-3 md:grid-cols-2">
                <div className="rounded-lg border p-3">
                  <div className="text-xs opacity-70">
                    A — {it.a.provider} / {it.a.model}
                  </div>
                  <div className="text-sm mt-1">
                    {clampText(it.a.content, 220)}
                  </div>
                </div>
                <div className="rounded-lg border p-3">
                  <div className="text-xs opacity-70">
                    B — {it.b.provider} / {it.b.model}
                  </div>
                  <div className="text-sm mt-1">
                    {clampText(it.b.content, 220)}
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      <div className="flex gap-2">
        <Button
          variant="outline"
          onClick={() => runSearch(false)}
          disabled={!canLoadMore}
        >
          Load more
        </Button>
        {loading && <div className="text-sm opacity-70 self-center"></div>}
      </div>
    </div>
  );
}
