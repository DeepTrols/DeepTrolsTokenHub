/**
 * OpenAI-compatible SSE chat streaming client (port of new-api's
 * use-stream-request without the sse.js dependency). Parses
 * chat.completion.chunk streams and exposes content/reasoning/usage deltas.
 */

import i18n from "../i18n";

export interface ChatStreamUsage {
  prompt_tokens?: number;
  completion_tokens?: number;
  total_tokens?: number;
}

export interface ChatStreamState {
  content: string;
  reasoning: string;
  usage?: ChatStreamUsage;
  finishReason?: string | null;
}

export interface ChatStreamCallbacks {
  onDelta: (text: string) => void;
  onReasoning?: (text: string) => void;
  onUsage?: (usage: ChatStreamUsage) => void;
}

export interface ChatStreamResult {
  content: string;
  reasoning: string;
  usage?: ChatStreamUsage;
}

/** Parses one SSE line: "data: <payload>" → payload, "[DONE]" → done. */
export function parseSSELine(line: string): { payload?: string; isDone?: boolean } {
  const trimmed = line.trim();
  if (!trimmed.startsWith("data:")) return {};
  const payload = trimmed.slice(5).trim();
  if (payload === "[DONE]") return { isDone: true };
  if (payload) return { payload };
  return {};
}

/**
 * Applies one chat.completion.chunk payload to the accumulated state and
 * returns the deltas produced by this chunk.
 */
export function applyChatChunk(
  payload: string,
  state: ChatStreamState,
): { contentDelta: string; reasoningDelta: string; usage?: ChatStreamUsage } {
  let chunk: Record<string, unknown>;
  try {
    chunk = JSON.parse(payload) as Record<string, unknown>;
  } catch {
    return { contentDelta: "", reasoningDelta: "" };
  }

  const choices = Array.isArray(chunk.choices) ? chunk.choices : [];
  const choice = (choices[0] as Record<string, unknown> | undefined) ?? {};
  const delta = (choice.delta as Record<string, unknown> | undefined) ?? {};
  const usage = chunk.usage as ChatStreamUsage | undefined;
  const finishReason = typeof choice.finish_reason === "string" ? choice.finish_reason : null;

  let contentDelta = "";
  if (typeof delta.content === "string" && delta.content) {
    state.content += delta.content;
    contentDelta = delta.content;
  }
  let reasoningDelta = "";
  if (typeof delta.reasoning_content === "string" && delta.reasoning_content) {
    state.reasoning += delta.reasoning_content;
    reasoningDelta = delta.reasoning_content;
  }
  if (usage && Object.keys(usage).length > 0) {
    state.usage = usage;
  }
  if (finishReason) {
    state.finishReason = finishReason;
  }
  return { contentDelta, reasoningDelta, usage: state.usage };
}

export interface StreamChatOptions {
  url: string;
  apiKey: string;
  model: string;
  messages: Array<{ role: string; content: string }>;
  callbacks: ChatStreamCallbacks;
  signal?: AbortSignal;
  requestId?: string;
}

/** Streams a chat completion over fetch + ReadableStream (no new dependency). */
export async function streamChatCompletion(options: StreamChatOptions): Promise<ChatStreamResult> {
  let res: Response;
  try {
    res = await fetch(options.url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${options.apiKey}`,
        ...(options.requestId ? { "X-Request-ID": options.requestId } : {}),
      },
      body: JSON.stringify({
        model: options.model,
        messages: options.messages,
        stream: true,
        stream_options: { include_usage: true },
      }),
      signal: options.signal,
    });
  } catch (e) {
    if ((e as Error).name === "AbortError") throw e;
    throw new Error(i18n.t("lib.streamNetwork", { message: e instanceof Error ? e.message : String(e) }));
  }

  if (!res.ok) {
    const body = (await res.json().catch(() => ({}))) as {
      error?: { message?: string };
    };
    throw new Error(body?.error?.message || i18n.t("lib.streamRequest", { status: res.status }));
  }
  if (!res.body) {
    throw new Error(i18n.t("lib.streamUnsupported"));
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  const state: ChatStreamState = { content: "", reasoning: "" };
  let buffer = "";

  const consumeLine = (line: string) => {
    const { payload, isDone } = parseSSELine(line);
    if (isDone) return true;
    if (!payload) return false;
    const { contentDelta, reasoningDelta, usage } = applyChatChunk(payload, state);
    if (contentDelta) options.callbacks.onDelta(contentDelta);
    if (reasoningDelta && options.callbacks.onReasoning) options.callbacks.onReasoning(reasoningDelta);
    if (usage && options.callbacks.onUsage) options.callbacks.onUsage(usage);
    return false;
  };

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() ?? "";
      for (const line of lines) {
        if (consumeLine(line)) {
          reader.cancel().catch(() => undefined);
          return { content: state.content, reasoning: state.reasoning, usage: state.usage };
        }
      }
    }
  } finally {
    reader.releaseLock();
  }

  return { content: state.content, reasoning: state.reasoning, usage: state.usage };
}
