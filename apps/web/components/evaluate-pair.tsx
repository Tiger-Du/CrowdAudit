"use client";

import { useEffect, useMemo, useState } from "react";
import { Button } from "@/components/ui/button";

export type PairDTO = {
  pairId: number;
  promptId: number;
  title: string;
  prompt: string;
  a: ResponseDTO;
  b: ResponseDTO;
};

export type ResponseDTO = {
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

export function EvaluatePair() {
  const [pair, setPair] = useState<PairDTO | null>(null);
  const [loading, setLoading] = useState(true);
  const [msg, setMsg] = useState<string>("");
  const [inferPrompt, setInferPrompt] = useState<string>("");
  const [inferModel, setInferModel] = useState<string>("");
  const [inferLoading, setInferLoading] = useState<boolean>(false);
  const [inferResult, setInferResult] = useState<string>("");
  const [inferStream, setInferStream] = useState<boolean>(true);

  const [modelA, setModelA] = useState("");
  const [modelB, setModelB] = useState("");
  const [resultA, setResultA] = useState("");
  const [resultB, setResultB] = useState("");
  const [isInferring, setIsInferring] = useState(false);
  const [clearing, setClearing] = useState(false);

  // Disable voting until both vote cards have non-default, non-empty content
  const DEFAULT_PLACEHOLDERS = [
    "Waiting for response...",
    "Waiting for your prompt...",
  ];

  const canVote = useMemo(() => {
    const isDefault = (s: string) => {
      const t = (s ?? "").trim();
      return t === "" || DEFAULT_PLACEHOLDERS.includes(t);
    };

    return !isDefault(resultA) && !isDefault(resultB) && !isInferring;
  }, [resultA, resultB, isInferring]);

  const ELIGIBLE_MODELS = [
    "allenai/olmo-3.1-32b-think:free",
    // 'nvidia/nemotron-3-nano-30b-a3b:free',
    "mistralai/devstral-2512:free",
    "allenai/olmo-3-32b-think:free",
    // 'nvidia/nemotron-nano-12b-v2-vl:free',
    "nvidia/nemotron-nano-9b-v2:free",
    // 'openai/gpt-oss-120b:free',
    // 'openai/gpt-oss-20b:free',
    // 'google/gemma-3n-e2b-it:free',
    // 'google/gemma-3n-e4b-it:free',
    "mistralai/mistral-small-3.1-24b-instruct:free",
    // 'google/gemma-3-4b-it:free',
    // 'google/gemma-3-12b-it:free',
    "google/gemma-3-27b-it:free",
    // 'google/gemini-2.0-flash-exp:free',
    "meta-llama/llama-3.3-70b-instruct:free",
    "meta-llama/llama-3.2-3b-instruct:free",
    "meta-llama/llama-3.1-405b-instruct:free",
    // 'mistralai/mistral-7b-instruct:free',
  ];

  function getRandomModels(count: number) {
    const shuffled = [...ELIGIBLE_MODELS].sort(() => 0.5 - Math.random());
    return shuffled.slice(0, count);
  }

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
    if (!pair && !modelA && !modelB) return;
    setMsg("");

    // const res = await fetch(`/api/votes`, {
    //   method: "POST",
    //   headers: { "Content-Type": "application/json" },
    //   body: JSON.stringify({ pairId: pair.pairId, voterId, choice }),
    // });

    // if (!res.ok) {
    //   setMsg("Vote failed. Try again.");
    //   return;
    // }

    // const data = (await res.json()) as { status: "recorded" | "duplicate" };
    // setMsg(
    //   data.status === "duplicate"
    //     ? "You already voted on this one."
    //     : "Vote recorded!",
    // );

    // Fade out current responses, clear, then advance
    setClearing(true);
    setTimeout(async () => {
      setResultA("");
      setResultB("");
      await loadPair();
      setClearing(false);
      // clear transient message after a short window
      setTimeout(() => setMsg(""), 2000);
    }, 300);

    await loadPair();
  }

  // Helper to stream a single model
  async function streamModel(
    prompt: string,
    model: string,
    setter: React.Dispatch<React.SetStateAction<string>>,
  ) {
    setter(""); // Clear previous
    try {
      const res = await fetch(`/api/infer/stream`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt, model }),
      });

      if (!res.ok) {
        setter(`Error: ${res.status}`);
        return;
      }

      const reader = res.body?.getReader();
      if (!reader) return;

      const decoder = new TextDecoder();
      let buffer = "";

      outerLoop: while (true) {
        const { done, value } = await reader.read();

        // Final flush when stream ends
        if (done) {
          if (buffer.trim()) processLine(buffer, setter);
          break;
        }

        buffer += decoder.decode(value, { stream: true });

        const lines = buffer.split("\n");
        // Keep the last partial line in the buffer
        buffer = lines.pop() || "";

        for (const line of lines) {
          const cleanLine = line.trim();
          if (!cleanLine) continue;
          if (cleanLine === "data: [DONE]") break outerLoop;

          processLine(cleanLine, setter);
        }
      }
    } catch (e) {
      setter((prev) => prev + "\n[Stream Interrupted]");
    }
  }

  // Helper to extract text from various provider formats
  function processLine(
    line: string,
    setter: React.Dispatch<React.SetStateAction<string>>,
  ) {
    const payload = line.startsWith("data: ") ? line.slice(6) : line;
    try {
      const parsed = JSON.parse(payload);
      const delta = parsed.choices?.[0]?.delta;

      // Look for content, then reasoning (for think models), then legacy text
      const chunk =
        delta?.content || delta?.reasoning || parsed.choices?.[0]?.text || "";

      if (chunk) {
        // Remove leading newlines only when appending to an empty result so we don't
        // strip intentional newlines in the middle of a response.
        setter((prev) => {
          if (!prev) {
            // strip any leading LF/CR characters from the very first chunk
            return chunk.replace(/^[\r\n]+/, "");
          }
          return prev + chunk;
        });
      }
    } catch (e) {
      // Ignore parse errors for partial JSON
    }
  }

  // async function runInfer() {
  //   if (!inferPrompt) {
  //     setInferResult("Please enter a prompt to send to the model.");
  //     return;
  //   }

  //   setInferLoading(true);
  //   setInferResult("");

  //   try {
  //     const endpoint = inferStream ? `/api/infer/stream` : `/api/infer`;

  //     const res = await fetch(endpoint, {
  //       method: "POST",
  //       headers: { "Content-Type": "application/json" },
  //       body: JSON.stringify({ prompt: inferPrompt, model: inferModel }),
  //     });

  //     if (!res.ok) {
  //       setInferResult(`Request failed: ${res.status} ${res.statusText}`);
  //       return;
  //     }

  //     if (res.body) {
  //       const reader = res.body.getReader();
  //       const decoder = new TextDecoder();
  //       let buffer = "";

  //       // Label the outer loop so we can break out of it from the inner loop
  //       outerLoop: while (true) {
  //         const { done, value } = await reader.read();
  //         if (done) break;

  //         buffer += decoder.decode(value, { stream: true });

  //         while (true) {
  //           const lineEnd = buffer.indexOf("\n");
  //           if (lineEnd === -1) break;

  //           const line = buffer.slice(0, lineEnd).trim();
  //           buffer = buffer.slice(lineEnd + 1);

  //           if (!line) continue;

  //           const payload = line.startsWith("data: ") ? line.slice(6) : line;

  //           // Handle the stop signal immediately
  //           if (payload === "[DONE]") {
  //             break outerLoop;
  //           }

  //           try {
  //             const parsed = JSON.parse(payload);
  //             const choice = parsed.choices?.[0];
  //             const delta = choice?.delta;

  //             // FIX: Priority-based selection instead of concatenation (+=)
  //             // This prevents duplicate content if a provider sends reasoning and content in one chunk
  //             let chunk = "";

  //             if (delta) {
  //               chunk = delta.content ||
  //                       delta.reasoning ||
  //                       (Array.isArray(delta.reasoning_details) ? delta.reasoning_details.map((rd: any) => rd?.text).join("") : "") ||
  //                       "";
  //             }

  //             // Only fall back to these if chunk is still empty
  //             if (!chunk) {
  //               chunk = choice?.text ||
  //                       choice?.message?.content ||
  //                       parsed.text ||
  //                       parsed.output ||
  //                       "";
  //             }

  //             if (chunk) {
  //               setInferResult((p) => p + chunk);
  //             }
  //           } catch (e) {
  //             // If it's not valid JSON yet (fragmented), it'll stay in the buffer logic
  //             // or append raw if it's clearly just text
  //             if (!payload.startsWith("{")) {
  //                setInferResult((p) => p + payload + "\n");
  //             }
  //           }
  //         }
  //       }

  //       // Final cleanup of the decoder
  //       const rest = decoder.decode();
  //       if (rest && !rest.includes("[DONE]")) {
  //          // Handle any final trailing text not followed by a newline
  //          try {
  //            const p = JSON.parse(rest);
  //            setInferResult(prev => prev + (p.choices?.[0]?.delta?.content || p.text || ""));
  //          } catch {
  //            if (rest.trim()) setInferResult(prev => prev + rest);
  //          }
  //       }

  //     } else {
  //       const data = await res.json();
  //       const out = data.output ?? data.result ?? data.text ?? data.content ?? data;
  //       setInferResult(typeof out === "string" ? out : JSON.stringify(out, null, 2));
  //     }
  //   } catch (err: any) {
  //     setInferResult(String(err?.message ?? err));
  //   } finally {
  //     setInferLoading(false);
  //   }
  // }

  async function runInfer() {
    if (!inferPrompt) return;

    // Pick two unique models
    const [pickedA, pickedB] = getRandomModels(2);
    setModelA(pickedA);
    setModelB(pickedB);

    setIsInferring(true);
    setResultA("");
    setResultB("");

    // Run both streams simultaneously
    await Promise.all([
      streamModel(inferPrompt, pickedA, setResultA),
      streamModel(inferPrompt, pickedB, setResultB),
    ]);

    setIsInferring(false);
  }

  useEffect(() => {
    loadPair();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // if (loading) return null;

  return (
    <div className="max-w-[1100px] mx-auto">
      {/* {msg && <p className="mt-3 text-sm">{msg}</p>} */}

      {
        <>
          {/* <section className="mt-4">
            <h2 className="text-lg font-semibold">{pair.title}</h2>
            <p className="mt-2 whitespace-pre-wrap">{pair.prompt}</p>
          </section> */}

          <section className="mt-4">
            <h2 className="text-lg font-semibold text-center">
              Live Evaluation
            </h2>
            {/* <p className="mt-2 whitespace-pre-wrap">{pair.prompt}</p> */}
          </section>

          {/* <section className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
            <VoteCard title="A" body={pair.a.content} />
            <VoteCard title="B" body={pair.b.content} />
          </section> */}

          <section className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
            <VoteCard
              // title={modelA ? `Model A: ${modelA.split('/')[1]}` : "Model A"}
              title="A"
              body={
                resultA ||
                (isInferring && !resultA
                  ? "Waiting for response..."
                  : "Waiting for your prompt...")
              }
              isClearing={clearing}
            />
            <VoteCard
              title="B"
              body={
                resultB ||
                (isInferring && !resultB
                  ? "Waiting for response..."
                  : "Waiting for your prompt...")
              }
              isClearing={clearing}
            />
          </section>

          <section className="mt-4 flex flex-wrap gap-2 justify-center">
            <Button
              variant="outline"
              onClick={() => vote("A")}
              disabled={!canVote}
            >
              A is better
            </Button>
            <Button
              variant="outline"
              onClick={() => vote("TIE")}
              disabled={!canVote}
            >
              Tie
            </Button>
            <Button
              variant="outline"
              onClick={() => vote("B")}
              disabled={!canVote}
            >
              B is better
            </Button>

            {/* Removed md:ml-auto to keep it in the center group */}
            {/* <Button onClick={loadPair}>
              Skip
            </Button> */}
          </section>

          <section className="mt-6">
            <div className="flex items-center justify-between">
              {/* <h3 className="text-md font-semibold">Model Battle (Dual Stream)</h3> */}
              {/* <Button onClick={runInfer} disabled={isInferring}>
                {isInferring ? "Generating..." : "Start"}
              </Button> */}
            </div>

            {/* <textarea
              value={inferPrompt}
              onChange={(e) => setInferPrompt(e.target.value)}
              placeholder="Ask anything..."
              className="w-full mt-2 rounded-md border p-2 font-mono text-sm resize-none"
              rows={1}
            /> */}

            {/* Flex container to hold the input and the button */}
            <div className="flex gap-2 items-start">
              {/* <textarea
                value={inferPrompt}
                onChange={(e) => setInferPrompt(e.target.value)}
                placeholder="Enter a prompt to test two models at once..."
                className="flex-1 rounded-md border p-2 font-mono text-sm resize-none h-[42px]" 
                // h-[42px] roughly matches the height of a standard Shadcn button
              /> */}
              <input
                type="text"
                value={inferPrompt}
                onChange={(e) => setInferPrompt(e.target.value)}
                placeholder="Ask anything..."
                className="flex-1 rounded-md border px-3 font-mono text-sm h-[42px]"
              />
              <Button
                onClick={runInfer}
                disabled={isInferring || canVote || clearing}
                className="shrink-0 h-[42px]"
              >
                {isInferring ? "Generating..." : "Start"}
              </Button>
            </div>

            {/* <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
              <div className="flex flex-col rounded-md border bg-muted/30 p-3 h-[300px]">
                <div className="text-xs font-bold uppercase text-muted-foreground mb-2">
                  Model A: {modelA || "Waiting..."}
                </div>
                <div className="flex-1 overflow-y-auto whitespace-pre-wrap font-mono text-sm">
                  {resultA}
                </div>
              </div>

              <div className="flex flex-col rounded-md border bg-muted/30 p-3 h-[300px]">
                <div className="text-xs font-bold uppercase text-muted-foreground mb-2">
                  Model B: {modelB || "Waiting..."}
                </div>
                <div className="flex-1 overflow-y-auto whitespace-pre-wrap font-mono text-sm">
                  {resultB}
                </div>
              </div>
            </div> */}
          </section>

          {/* <section className="mt-6">
            <h3 className="text-md font-semibold">Send custom prompt to models</h3>
            <textarea
              value={inferPrompt}
              onChange={(e) => setInferPrompt(e.target.value)}
              placeholder="Ask anything..."
              className="w-full mt-2 rounded-md border p-2 font-mono text-sm resize-none"
              rows={1}
            />

            <div className="mt-2 flex gap-2">
              <label className="flex items-center gap-2">
                <input type="checkbox" checked={inferStream} onChange={(e) => setInferStream(e.target.checked)} />
                <span className="text-sm">Stream</span>
              </label>
              <input
                value={inferModel}
                onChange={(e) => setInferModel(e.target.value)}
                placeholder="model (optional)"
                className="rounded-md border p-2 text-sm"
              />

              <Button onClick={runInfer} disabled={inferLoading}>
                {inferLoading ? "Running..." : "Run model"}
              </Button>

              <Button variant="outline" onClick={() => { setInferPrompt(""); setInferResult(""); setInferModel(""); }}>
                Clear
              </Button>
            </div>

            {inferResult && (
              <pre className="mt-3 whitespace-pre-wrap font-mono text-sm rounded-md border p-3">{inferResult}</pre>
            )}
          </section> */}
        </>
      }
    </div>
  );
}

export function VoteCard({
  title,
  body,
  isClearing,
}: {
  title: string;
  body: string;
  isClearing?: boolean;
}) {
  return (
    // Change min-h-[400px] to h-[400px] (or whatever height you prefer)
    <div className="rounded-xl border p-4 flex flex-col h-[400px] bg-background">
      <div className="font-bold text-center border-b pb-2 mb-2 shrink-0">
        {title}
      </div>

      {/* Scrollable Container */}
      <pre
        className={`flex-1 overflow-y-auto whitespace-pre-wrap font-mono text-sm pr-6 custom-scrollbar transition-opacity duration-300 ${
          isClearing ? "opacity-0" : "opacity-100"
        }`}
      >
        {body}
      </pre>
    </div>
  );
}
