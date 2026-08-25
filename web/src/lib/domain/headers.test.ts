import { describe, it, expect } from "vitest";
import { parseHeaderLines, formatHeaderLines } from "./headers";

describe("parseHeaderLines", () => {
  it("parses Key: Value lines and trims whitespace", () => {
    expect(parseHeaderLines("X-Gateway-Id: gw-east-1\n  X-Tenant : acme  ")).toEqual({
      "X-Gateway-Id": "gw-east-1",
      "X-Tenant": "acme",
    });
  });

  it("skips blank lines and lines without a colon", () => {
    expect(parseHeaderLines("\nX-A: 1\nnot a header\n\nX-B:2\n")).toEqual({ "X-A": "1", "X-B": "2" });
  });

  it("handles empty input", () => {
    expect(parseHeaderLines("")).toEqual({});
  });
});

describe("formatHeaderLines", () => {
  it("formats sorted Key: Value lines", () => {
    expect(formatHeaderLines({ "X-B": "2", "X-A": "1" })).toBe("X-A: 1\nX-B: 2");
  });

  it("returns empty string for empty or null input", () => {
    expect(formatHeaderLines({})).toBe("");
    expect(formatHeaderLines(null)).toBe("");
    expect(formatHeaderLines(undefined)).toBe("");
  });
});
