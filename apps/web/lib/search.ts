export type SearchPairItem = {
  pairId: number;
  promptId: number;
  title: string;
  prompt: string;
  a: { responseId: number; provider: string; model: string; content: string };
  b: { responseId: number; provider: string; model: string; content: string };
  votes: { total: number; a: number; b: number; tie: number };
  disagreementScore: number;
  updatedAt: string;
};

export type SearchPairsResponse = {
  items: SearchPairItem[];
  nextCursor?: string;
};

export type SortMode = "relevance" | "disagreement" | "recent";

export async function searchPairs(params: {
  q?: string;
  sort?: SortMode;
  cursor?: string;
  limit?: number;
  visibility?: string;
}): Promise<SearchPairsResponse> {
  const sp = new URLSearchParams();
  if (params.q) sp.set("q", params.q);
  if (params.sort) sp.set("sort", params.sort);
  if (params.cursor) sp.set("cursor", params.cursor);
  if (params.limit) sp.set("limit", String(params.limit));
  if (params.visibility) sp.set("visibility", params.visibility);

  const res = await fetch(`/api/search/pairs?${sp.toString()}`, {
    cache: "no-store",
  });

  if (!res.ok) {
    const body = await res.text();
    throw new Error(`search failed status=${res.status} body=${body}`);
  }

  return res.json();
}
