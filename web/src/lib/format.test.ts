import { describe, it, expect } from "vitest";
import { formatAmount } from "./format";

describe("formatAmount (四舍六入五成双 / banker's rounding to 2 dp)", () => {
  it.each([
    // 四舍: 3rd decimal <= 4 → truncate
    ["1.004", "1.00"],
    ["0.763000", "0.76"],
    ["1.0049", "1.00"],
    // 六入: 3rd decimal >= 6 → round up
    ["9999.237", "9999.24"],
    ["1.006", "1.01"],
    // 五成双: exactly 5, no trailing non-zero → round toward even cent
    ["1.005", "1.00"],
    ["1.025", "1.02"],
    ["0.005", "0.00"],
    ["1.015", "1.02"],
    ["1.035", "1.04"],
    ["9.995", "10.00"],
    // 5 with trailing non-zero digits → round up regardless of parity
    ["1.0051", "1.01"],
    ["1.025001", "1.03"],
    // negative amounts
    ["-0.763000", "-0.76"],
    ["-1.005", "-1.00"],
    ["-1.015", "-1.02"],
    // integers / zero / already-fixed decimals
    ["5", "5.00"],
    ["0", "0.00"],
    ["50", "50.00"],
    ["0.5", "0.50"],
    ["95.00", "95.00"],
    ["+50.00", "50.00"],
    // negative-zero normalization and leading-zero stripping
    ["-0.000", "0.00"],
    ["0005.00", "5.00"],
    // scientific notation, expanded by string arithmetic (no float loss)
    ["1e-7", "0.00"],
    ["9.995e0", "10.00"],
  ])("formats %s -> %s", (input, expected) => {
    expect(formatAmount(input)).toBe(expected);
  });

  it("accepts numeric input", () => {
    expect(formatAmount(42)).toBe("42.00");
    expect(formatAmount(0.5)).toBe("0.50");
  });

  it("returns 0.00 for null/undefined/empty/garbage", () => {
    expect(formatAmount(null)).toBe("0.00");
    expect(formatAmount(undefined)).toBe("0.00");
    expect(formatAmount("")).toBe("0.00");
    expect(formatAmount("abc")).toBe("0.00");
  });
});
