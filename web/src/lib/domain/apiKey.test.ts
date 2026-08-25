import { describe, it, expect } from "vitest";
import { parseRateLimit, buildKeyLimitsBody, formatRateLimit } from "./apiKey";

const empty = {
  monthlyLimit: "",
  weeklyLimit: "",
  cumulativeLimit: "",
  rateLimitRpm: "",
  rateLimitTpm: "",
};

describe("parseRateLimit", () => {
  it("returns undefined for empty/whitespace input", () => {
    expect(parseRateLimit("")).toBeUndefined();
    expect(parseRateLimit("   ")).toBeUndefined();
  });

  it("parses non-negative integers", () => {
    expect(parseRateLimit("0")).toBe(0);
    expect(parseRateLimit("120")).toBe(120);
    expect(parseRateLimit("64000")).toBe(64000);
  });

  it("rejects decimals, negatives and non-numeric input", () => {
    expect(parseRateLimit("1.5")).toBeNaN();
    expect(parseRateLimit("-1")).toBeNaN();
    expect(parseRateLimit("abc")).toBeNaN();
  });
});

describe("buildKeyLimitsBody", () => {
  it("omits empty fields", () => {
    expect(buildKeyLimitsBody(empty)).toEqual({ body: {}, errors: [] });
  });

  it("carries monetary limits as strings and rate limits as numbers", () => {
    const { body, errors } = buildKeyLimitsBody({
      ...empty,
      monthlyLimit: "500",
      weeklyLimit: "200",
      cumulativeLimit: "5000",
      rateLimitRpm: "120",
      rateLimitTpm: "64000",
    });
    expect(errors).toEqual([]);
    expect(body).toEqual({
      monthly_limit: "500",
      weekly_limit: "200",
      cumulative_limit: "5000",
      rate_limit_rpm: 120,
      rate_limit_tpm: 64000,
    });
  });

  it("reports invalid rate-limit input without sending it", () => {
    const { body, errors } = buildKeyLimitsBody({
      ...empty,
      rateLimitRpm: "abc",
      rateLimitTpm: "-5",
    });
    expect(errors).toHaveLength(2);
    expect(body).toEqual({});
  });
});

describe("formatRateLimit", () => {
  it("returns 不限 when unset or zero", () => {
    expect(formatRateLimit()).toBe("不限");
    expect(formatRateLimit(0, 0)).toBe("不限");
    expect(formatRateLimit(null, undefined)).toBe("不限");
  });

  it("formats single and combined limits", () => {
    expect(formatRateLimit(120)).toBe("120 RPM");
    expect(formatRateLimit(undefined, 64000)).toBe("64000 TPM");
    expect(formatRateLimit(120, 64000)).toBe("120 RPM · 64000 TPM");
  });
});
