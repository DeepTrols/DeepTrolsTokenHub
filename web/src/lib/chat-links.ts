/**
 * External chat-client link helpers (port of new-api's chat-links.ts):
 * resolve URL templates like
 *   https://chat.example/?api_key={key}&base_url={address}
 * and the {cherryConfig} / {aionuiConfig} / {deepchatConfig} base64 payloads.
 */

export type ChatLinkType = "web" | "custom-protocol" | "fluent";

export interface ChatPreset {
  id: string;
  name: string;
  url: string;
  type: ChatLinkType;
}

export type RawChatConfig =
  | string
  | Record<string, unknown>
  | Array<Record<string, unknown>>
  | null
  | undefined;

export interface ResolveChatUrlParams {
  template: string;
  apiKey?: string;
  serverAddress: string;
}

const HTTP_REGEX = /^https?:\/\//i;

function toBase64(value: string): string {
  if (typeof window !== "undefined" && typeof window.btoa === "function") {
    return window.btoa(value);
  }
  const bufferCtor = (globalThis as Record<string, unknown>).Buffer;
  if (
    typeof bufferCtor === "function" &&
    typeof (bufferCtor as { from?: unknown }).from === "function"
  ) {
    return (
      bufferCtor as unknown as {
        from(s: string, enc: string): { toString(enc: string): string };
      }
    )
      .from(value, "utf-8")
      .toString("base64");
  }
  return "";
}

export function detectChatLinkType(url: string): ChatLinkType {
  if (HTTP_REGEX.test(url)) return "web";
  if (url.toLowerCase().startsWith("fluent")) return "fluent";
  return "custom-protocol";
}

export function chatLinkRequiresApiKey(url: string): boolean {
  return (
    url.includes("{key}") ||
    url.includes("{cherryConfig}") ||
    url.includes("{aionuiConfig}") ||
    url.includes("{deepchatConfig}")
  );
}

/** Parses presets persisted as a JSON array of {"name": "url"} entries. */
export function parseChatConfig(raw: RawChatConfig): ChatPreset[] {
  let parsed: unknown = raw;
  if (typeof raw === "string") {
    try {
      parsed = JSON.parse(raw);
    } catch {
      return [];
    }
  }
  if (!Array.isArray(parsed)) return [];

  return parsed
    .map((entry, index) => {
      if (!entry || typeof entry !== "object" || Array.isArray(entry)) return null;
      const entries = Object.entries(entry as Record<string, unknown>);
      if (entries.length !== 1) return null;
      const [name, value] = entries[0];
      if (typeof value !== "string" || typeof name !== "string") return null;
      const url = value.trim();
      if (!url) return null;
      return { id: String(index), name, url, type: detectChatLinkType(url) } satisfies ChatPreset;
    })
    .filter((item): item is ChatPreset => item !== null);
}

function replaceToken(source: string, token: string, value: string): string {
  return source.split(token).join(value);
}

export function normalizeApiKey(apiKey: string): string {
  const trimmed = apiKey.trim();
  if (!trimmed) return "";
  return trimmed.startsWith("sk-") ? trimmed : `sk-${trimmed}`;
}

export function resolveChatUrl({ template, apiKey, serverAddress }: ResolveChatUrlParams): string {
  let url = template;
  const safeServerAddress = serverAddress || "";
  const safeApiKey = normalizeApiKey(apiKey || "");

  for (const [token, id] of [
    ["{cherryConfig}", "new-api"],
    ["{aionuiConfig}", "new-api"],
    ["{deepchatConfig}", "new-api"],
  ] as const) {
    if (!url.includes(token)) continue;
    const payload =
      token === "{aionuiConfig}"
        ? { platform: id, baseUrl: safeServerAddress, apiKey: safeApiKey }
        : { id, baseUrl: safeServerAddress, apiKey: safeApiKey };
    const encoded = encodeURIComponent(toBase64(JSON.stringify(payload)));
    return replaceToken(url, token, encoded);
  }

  if (safeServerAddress) {
    url = replaceToken(url, "{address}", encodeURIComponent(safeServerAddress));
  }
  if (safeApiKey) {
    url = replaceToken(url, "{key}", safeApiKey);
  }
  return url;
}
