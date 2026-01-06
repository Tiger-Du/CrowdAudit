"use server";

import { Pool } from "pg";

import redis from "@/lib/redis";

export type CommunityConversation = {
  conversation_id: string;
  assigned_lang: string;
  first_turn_prompt: string;
  first_turn_response_a: string;
  first_turn_response_b: string;
  first_turn_feedback?: string;
  feedback_score: number;
};

// We create a single pool instance so we don't
// create a new connection on every single click.
const pool = new Pool({
  connectionString: process.env.POSTGRES_URL,
});

export async function getCommunityConversationCount() {
  try {
    // Standard PG query logic
    console.log("Querying Postgres...");
    const { rows } = await pool.query(`
      SELECT COUNT(*) FROM community_alignment_conversations 
    `);

    return rows;
  } catch (error) {
    console.error("Database Error:", error);
    return [];
  }
}

// Direct to PostgreSQL

// export async function getCommunityConversations() {
//   const CACHE_KEY = "social:conversations:top";

//   try {
//     // 1. Try to get from Redis (if present)
//     if (redis) {
//       const cached = await redis.get(CACHE_KEY);
//       if (cached) {
//         return JSON.parse(cached);
//       }
//     }

//     // 2. If not in Redis, get from Postgres
//     // Standard PG query logic
//     const { rows } = await pool.query(`
//       SELECT * FROM community_alignment_conversations 
//       ORDER BY conversation_id DESC 
//       LIMIT 5
//     `);

//     // 3. Store in Redis for 60 seconds (if present)
//     if (redis) {
//         await redis.set(CACHE_KEY, JSON.stringify(rows), "EX", 60);
//     }

//     return rows;
//   } catch (error) {
//     console.error("Database Error:", error);
//     return [];
//   }
// }

export async function getCommunityConversations(limit = 20, cursor?: string) {
  const url = new URL(`${process.env.API_BASE_URL}/api/community/conversations`);
  url.searchParams.set("limit", String(limit));
  if (cursor) url.searchParams.set("cursor", cursor);

  const res = await fetch(url.toString(), {
    next: { revalidate: 10 },
  });

  if (!res.ok) throw new Error("Failed to fetch community conversations");

  return res.json() as Promise<{ items: CommunityConversation[]; next_cursor?: string }>;
}

export async function updateFeedbackScore(conversationId: string, delta: number) {
  const url = new URL(
    `${process.env.API_BASE_URL}/api/community/conversations/vote`,
  );

  const res = await fetch(url.toString(), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ conversation_id: conversationId, delta }),
    cache: "no-store",
  });

  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(`vote failed: ${res.status} ${text}`);
  }

  const data = (await res.json()) as {
    conversation_id: string;
    feedback_score: number;
  };

  return data.feedback_score;
}

// Direct to PostgreSQL

// export async function updateFeedbackScore(
//   conversationId: number,
//   delta: number,
// ) {
//   try {
//     console.log("POSTGRES_URL =", process.env.POSTGRES_URL);

//     // 1. Update the DB as the source of truth
//     const { rows } = await pool.query(
//       `
//       UPDATE community_alignment_conversations
//       SET feedback_score = feedback_score + $1
//       WHERE conversation_id = $2
//       RETURNING feedback_score
//     `,
//       [delta, conversationId],
//     );

//     // 2. Invalidate the Redis cache so the next refresh shows the new number
//     if (redis) {
//         await redis.del("social:conversations:top");
//     }

//     return rows[0].feedback_score;
//   } catch (error) {
//     console.error("Failed to update score:", error);
//     throw new Error("Score update failed");
//   }
// }
