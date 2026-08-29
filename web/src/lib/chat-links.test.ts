import { describe, it, expect } from "vitest";
import {
  chatLinkRequiresApiKey,
  detectChatLinkType,
  normalizeApiKey,
  parseChatConfig,
  resolveChatUrl,
} from "./chat-links";

describe("parseChatConfig", () => {
  it("parses single-key object entries", () => {
    const presets = parseChatConfig([
      { "Cherry Studio": "https://chat.cherry-ai.com/?api_key={key}" },
      { "Local Fluent": "fluent://chat?base={address}" },
    ]);
    expect(presets).toHaveLength(2);
    expect(presets[0]).toMatchObject({
      id: "0",
      name: "Cherry Studio",
      type: "web",
      url: "https://chat.cherry-ai.com/?api_key={key}",
    });
    expect(presets[1].type).toBe("fluent");
  });

  it("parses a JSON string payload", () => {
    const presets = parseChatConfig('[{"Demo":"https://demo.example"}]');
    expect(presets).toHaveLength(1);
    expect(presets[0].name).toBe("Demo");
  });

  it("rejects malformed and multi-key entries", () => {
    expect(parseChatConfig("not-json")).toEqual([]);
    expect(parseChatConfig([{ a: "x", b: "y" }])).toEqual([]);
    expect(parseChatConfig([])).toEqual([]);
  });
});

describe("chatLinkRequiresApiKey", () => {
  it("detects {key} and config tokens", () => {
    expect(chatLinkRequiresApiKey("https://x?api_key={key}")).toBe(true);
    expect(chatLinkRequiresApiKey("https://x?cfg={cherryConfig}")).toBe(true);
    expect(chatLinkRequiresApiKey("https://x")).toBe(false);
  });
});

describe("resolveChatUrl", () => {
  it("replaces {key} and {address}", () => {
    const url = resolveChatUrl({
      template: "https://chat.example/?api_key={key}&base_url={address}",
      apiKey: "sk-abc123",
      serverAddress: "https://api.example.com",
    });
    expect(url).toBe(
      "https://chat.example/?api_key=sk-abc123&base_url=https%3A%2F%2Fapi.example.com",
    );
  });

  it("normalizes a bare API key", () => {
    const url = resolveChatUrl({
      template: "https://x/?key={key}",
      apiKey: "abc123",
      serverAddress: "",
    });
    expect(url).toContain("key=sk-abc123");
  });

  it("injects the base64 cherryConfig payload", () => {
    const url = resolveChatUrl({
      template: "https://cherry/?cfg={cherryConfig}",
      apiKey: "sk-k",
      serverAddress: "https://api.example.com",
    });
    expect(url).toMatch(/^https:\/\/cherry\/\?cfg=/);
    const encoded = url.split("=").slice(1).join("=");
    const decoded = atob(decodeURIComponent(encoded));
    const payload = JSON.parse(decoded);
    expect(payload).toMatchObject({ id: "new-api", apiKey: "sk-k", baseUrl: "https://api.example.com" });
  });
});

describe("normalizeApiKey", () => {
  it("keeps the sk- prefix and trims", () => {
    expect(normalizeApiKey("  sk-123 ")).toBe("sk-123");
    expect(normalizeApiKey("123")).toBe("sk-123");
    expect(normalizeApiKey("")).toBe("");
  });
});

describe("detectChatLinkType", () => {
  it("classifies http/fluent/protocol links", () => {
    expect(detectChatLinkType("https://a")).toBe("web");
    expect(detectChatLinkType("fluent://x")).toBe("fluent");
    expect(detectChatLinkType("myapp://open")).toBe("custom-protocol");
  });
});
