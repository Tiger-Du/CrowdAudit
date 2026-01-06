// "use client";

// import { useEffect, useMemo, useState } from "react";
// import { Button } from "@/components/ui/button";

// export function Social() {

// return (
// <div className="max-w-[1100px] mx-auto">
//       {/* {msg && <p className="mt-3 text-sm">{msg}</p>} */}

//       {<>
//           {/* <section className="mt-4">
//             <h2 className="text-lg font-semibold">{pair.title}</h2>
//             <p className="mt-2 whitespace-pre-wrap">{pair.prompt}</p>
//           </section> */}

//           <section className="mt-4">
//             <h2 className="text-lg font-semibold text-center">Social Evaluation</h2>
//             {/* <p className="mt-2 whitespace-pre-wrap">{pair.prompt}</p> */}
//           </section>

//           {/* <section className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
//             <VoteCard title="A" body={pair.a.content} />
//             <VoteCard title="B" body={pair.b.content} />
//           </section> */}

//           <section className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
//           </section>
//         </>
//       }
//     </div>
//     )
// }

"use client";

import * as React from "react";
import { VoteCard } from "@/components/evaluate-pair"; // Use your existing VoteCard
import {
  getCommunityConversations,
  updateFeedbackScore,
  getCommunityConversationCount,
  type CommunityConversation,
} from "@/src/app/actions/social";
import { ArrowBigUp, ArrowBigDown } from "lucide-react";

export function Social() {
  const [loading, setLoading] = React.useState(true);
  const [conversations, setConversations] = React.useState<CommunityConversation[]>([]);
  //   const [conversationCount, setConversationCount] = React.useState(0)
  const [userVotes, setUserVotes] = React.useState<Record<string, number>>({});

  // 1. Load votes from localStorage on mount. Single source of truth for votes (Local Storage)
  React.useEffect(() => {
    const savedVotes = localStorage.getItem("community-votes");
    if (savedVotes) {
      setUserVotes(JSON.parse(savedVotes));
    }
  }, []);

  // 2. Fetch data from DB
  React.useEffect(() => {
    async function loadData() {
      try {
        const data = await getCommunityConversations();
        setConversations(data.items);

        // const count = await getCommunityConversationCount();
        // console.log(count)
        // setConversationCount(count);
      } catch (err) {
        console.error("Fetch failed", err);
      } finally {
        setLoading(false);
      }
    }
    loadData();
  }, []);

  const handleVote = async (id: string, direction: number) => {
    const prevVote = userVotes[id] || 0;

    // toggle
    // If user clicks the same button again, they are "un-voting"
    const nextVote = prevVote === direction ? 0 : direction;
    // Calculate the change needed for the database
    // e.g., switching from -1 to +1 requires a delta of +2
    const delta = nextVote - prevVote;

    // Update Local Storage / State immediately (Optimistic)
    // optimistic local vote state
    const optimisticVotes = { ...userVotes, [id]: nextVote };
    setUserVotes(optimisticVotes);
    localStorage.setItem("community-votes", JSON.stringify(optimisticVotes));

    // Update UI State / Score (Optimistic)
    setConversations((prev) =>
      prev.map((c) =>
        c.conversation_id === id
          ? {
              ...c,
              feedback_score: Number(c.feedback_score || 0) + delta,
            }
          : c,
      ),
    );

    // Update Database
    try {
      const newScore = await updateFeedbackScore(id, delta);

      // server truth wins
      setConversations((prev) =>
        prev.map((c) =>
          c.conversation_id === id ? { ...c, feedback_score: newScore } : c,
        ),
      );
    } catch (err) {
      // Revert score on error if needed / if DB fails
      setConversations((prev) =>
        prev.map((c) =>
          c.conversation_id === id
            ? {
                ...c,
                feedback_score: Number(c.feedback_score || 0) - delta,
              }
            : c,
        ),
      );

      // revert vote state + localStorage
      const revertedVotes = { ...userVotes, [id]: prevVote };
      setUserVotes(revertedVotes);
      localStorage.setItem("community-votes", JSON.stringify(revertedVotes));
    }
  };

  if (loading)
    return <div className="p-8 text-center text-muted-foreground"></div>;
  if (conversations.length === 0)
    return (
      <div className="p-8 text-center text-muted-foreground">
        No conversations found.
      </div>
    );

  return (
    <>
      <section className="mt-4">
        <h2 className="text-lg font-semibold text-center">Social Evaluation</h2>
      </section>
      {/* <p>{conversationCount}</p> */}
      <div className="space-y-12">
        {conversations.map((conv) => (
          <section
            key={conv.conversation_id}
            className="border-b last:border-0 pb-10"
          >
            <div className="mb-4 flex items-center justify-between">
              <div>
                <span className="text-[10px] font-mono bg-muted px-2 py-0.5 rounded uppercase tracking-wider">
                  Conv #{conv.conversation_id}
                </span>
                <p className="mt-2 text-sm font-medium leading-relaxed">
                  &quot;{conv.first_turn_prompt}&quot;
                </p>
              </div>
              <div className="text-xs text-muted-foreground bg-secondary px-2 py-1 rounded">
                {conv.assigned_lang}
              </div>
            </div>

            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <VoteCard title="Response A" body={conv.first_turn_response_a} />
              <VoteCard title="Response B" body={conv.first_turn_response_b} />
            </div>

            {conv.first_turn_feedback && (
              // <div className="mt-4 p-3 rounded-lg border bg-blue-50/50 dark:bg-blue-950/20 text-sm">
              <div className="mt-4 p-3 rounded-lg border bg-blue-50/50 dark:bg-blue-950/20 flex items-center justify-between">
                <div className="text-sm">
                  <span className="font-semibold text-blue-700 dark:text-blue-400">
                    Feedback:{" "}
                  </span>
                  <span className="italic text-muted-foreground">
                    {conv.first_turn_feedback}
                  </span>
                </div>

                {/* Voting Controls */}
                <div className="flex items-center gap-1 ml-4 border rounded-md bg-background p-1 shadow-sm">
                  <button
                    onClick={() => handleVote(conv.conversation_id, 1)}
                    className={`p-1 rounded transition-all ${
                      userVotes[conv.conversation_id] === 1
                        ? "text-green-600 bg-green-50 dark:bg-green-900/20"
                        : "text-muted-foreground hover:bg-muted"
                    }`}
                  >
                    <ArrowBigUp
                      className={`size-5 ${userVotes[conv.conversation_id] === 1 ? "fill-current" : ""}`}
                    />
                  </button>

                  <span
                    className={`text-xs font-mono font-bold px-1 w-8 text-center ${
                      userVotes[conv.conversation_id] === 1
                        ? "text-green-600"
                        : userVotes[conv.conversation_id] === -1
                          ? "text-red-600"
                          : ""
                    }`}
                  >
                    {conv.feedback_score || 0}
                  </span>

                  <button
                    onClick={() => handleVote(conv.conversation_id, -1)}
                    className={`p-1 rounded transition-all ${
                      userVotes[conv.conversation_id] === -1
                        ? "text-red-600 bg-red-50 dark:bg-red-900/20"
                        : "text-muted-foreground hover:bg-muted"
                    }`}
                  >
                    <ArrowBigDown
                      className={`size-5 ${userVotes[conv.conversation_id] === -1 ? "fill-current" : ""}`}
                    />
                  </button>
                </div>
              </div>
            )}
          </section>
        ))}
      </div>
    </>
  );
}
