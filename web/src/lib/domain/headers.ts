// Pure helpers for the custom request-headers editor (Key: Value lines).

/**
 * Parses "Key: Value" lines into a headers map. Blank lines and lines without
 * a colon are skipped; keys and values are trimmed.
 */
export function parseHeaderLines(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line) continue;
    const idx = line.indexOf(":");
    if (idx <= 0) continue;
    const key = line.slice(0, idx).trim();
    const value = line.slice(idx + 1).trim();
    if (key) out[key] = value;
  }
  return out;
}

/** Formats a headers map as sorted "Key: Value" lines for the textarea. */
export function formatHeaderLines(headers: Record<string, string> | null | undefined): string {
  if (!headers || Object.keys(headers).length === 0) return "";
  return Object.keys(headers)
    .sort()
    .map((k) => `${k}: ${headers[k]}`)
    .join("\n");
}
