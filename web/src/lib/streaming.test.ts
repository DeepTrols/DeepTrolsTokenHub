import { describe, it, expect, vi, afterEach } from "vitest";
import {
  applyChatChunk,
  parseSSELine,
  streamChatCompletion,
  type ChatStreamState,
} from "./streaming";

describe("parseSSELine", () => {
  it("extracts data payloads", () => {
    expect(parseSSELine("data: {\"a\":1}")).toEqual({ payload: '{"a":1}' });
    expect(parseSSELine("data:  {\"b\":2}  ")).toEqual({ payload: '{"b":2}' });
  });

  it("detects the [DONE] marker", () => {
    expect(parseSSELine("data: [DONE]")).toEqual({ isDone: true });
  });

  it("ignores comments and empty lines", () => {
    expect(parseSSELine(": keep-alive")).toEqual({});
    expect(parseSSELine("")).toEqual({});
  });
});

describe("applyChatChunk", () => {
  it("accumulates content and returns the delta", () => {
    const state: ChatStreamState = { content: "", reasoning: "" };
    const first = applyChatChunk(
      '{"choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}',
      state,
    );
    expect(first.contentDelta).toBe("Hello");
    expect(state.content).toBe("Hello");

    const second = applyChatChunk(
      '{"choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}',
      state,
    );
    expect(second.contentDelta).toBe(" world");
    expect(state.content).toBe("Hello world");
  });

  it("captures reasoning deltas", () => {
    const state: ChatStreamState = { content: "", reasoning: "" };
    const r = applyChatChunk(
      '{"choices":[{"index":0,"delta":{"reasoning_content":"think"},"finish_reason":null}]}',
      state,
    );
    expect(r.reasoningDelta).toBe("think");
    expect(state.reasoning).toBe("think");
  });

  it("captures usage from the final chunk", () => {
    const state: ChatStreamState = { content: "hi", reasoning: "" };
    applyChatChunk(
      '{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}',
      state,
    );
    expect(state.usage?.total_tokens).toBe(15);
  });

  it("ignores malformed payloads", () => {
    const state: ChatStreamState = { content: "x", reasoning: "" };
    const r = applyChatChunk("not json", state);
    expect(r.contentDelta).toBe("");
    expect(state.content).toBe("x");
  });
});

describe("streamChatCompletion", () => {
  const originalFetch = globalThis.fetch;

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  function sseResponse(chunks: string[]) {
    const encoder = new TextEncoder();
    return {
      ok: true,
      body: new ReadableStream({
        start(controller) {
          for (const c of chunks) controller.enqueue(encoder.encode(c));
          controller.close();
        },
      }),
    };
  }

  it("streams deltas, usage and resolves with the full content", async () => {
    const mockFetch = vi.fn().mockResolvedValue(
      sseResponse([
        'data: {"choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}\n\n',
        'data: {"choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}\n\n',
        'data: {"choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}\n\n',
        'data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}\n\n',
        "data: [DONE]\n\n",
      ]),
    );
    globalThis.fetch = mockFetch;

    const deltas: string[] = [];
    const result = await streamChatCompletion({
      url: "/v1/chat/completions",
      apiKey: "sk-test",
      model: "deepseek-chat",
      messages: [{ role: "user", content: "hi" }],
      callbacks: {
        onDelta: (t) => deltas.push(t),
        onUsage: () => undefined,
      },
    });

    expect(deltas).toEqual(["Hello", "!"]);
    expect(result.content).toBe("Hello!");
    expect(result.usage?.total_tokens).toBe(5);
    expect(JSON.parse((mockFetch.mock.calls[0][1] as RequestInit).body as string)).toMatchObject({
      stream: true,
      stream_options: { include_usage: true },
    });
  });

  it("throws a readable error on non-2xx responses", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: async () => ({ error: { message: "Invalid API key" } }),
    });

    await expect(
      streamChatCompletion({
        url: "/v1/chat/completions",
        apiKey: "sk-bad",
        model: "m",
        messages: [],
        callbacks: { onDelta: () => undefined },
      }),
    ).rejects.toThrow("Invalid API key");
  });

  it("handles payloads split across reader chunks", async () => {
    const encoder = new TextEncoder();
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      body: new ReadableStream({
        start(controller) {
          // Split the JSON payload across two network frames.
          controller.enqueue(encoder.encode('data: {"choices":[{"index":0,"delta":{"content":"Hel'));
          controller.enqueue(encoder.encode('lo"},"finish_reason":null}]}\n\n'));
          controller.enqueue(encoder.encode("data: [DONE]\n\n"));
          controller.close();
        },
      }),
    });

    const deltas: string[] = [];
    const result = await streamChatCompletion({
      url: "/v1/chat/completions",
      apiKey: "sk-test",
      model: "m",
      messages: [],
      callbacks: { onDelta: (t) => deltas.push(t) },
    });

    expect(deltas).toEqual(["Hello"]);
    expect(result.content).toBe("Hello");
  });
});
