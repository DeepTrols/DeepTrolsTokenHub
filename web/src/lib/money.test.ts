import { describe, it, expect } from "vitest";
import { fmtMoney, isValidAmount, toCents, toMoneyInput } from "./money";

describe("toCents", () => {
  it("parses whole and fractional amounts", () => {
    expect(toCents("10")).toBe(1000n);
    expect(toCents("0")).toBe(0n);
    expect(toCents("10.5")).toBe(1050n);
    expect(toCents("10.50")).toBe(1050n);
    expect(toCents("120.5")).toBe(12050n);
  });

  it("stays precise beyond the float safe-integer range", () => {
    // Number("9007199254740993") rounds down; BigInt does not.
    expect(toCents("9007199254740993")).toBe(900719925474099300n);
  });
});

describe("fmtMoney", () => {
  it("formats thousands separators and two decimals", () => {
    expect(fmtMoney("10000")).toBe("10,000.00");
    expect(fmtMoney("120.5")).toBe("120.50");
    expect(fmtMoney("0")).toBe("0.00");
  });
});

describe("toMoneyInput", () => {
  it("keeps digits and a single decimal point", () => {
    expect(toMoneyInput("10.5")).toBe("10.5");
    expect(toMoneyInput("abc")).toBe("");
    expect(toMoneyInput("1.2.3")).toBe("1.23");
    expect(toMoneyInput("-5")).toBe("5");
  });

  it("caps fractional digits at two", () => {
    expect(toMoneyInput("1.234")).toBe("1.23");
  });
});

describe("isValidAmount", () => {
  it("accepts positive amounts with at most two decimals", () => {
    expect(isValidAmount("10")).toBe(true);
    expect(isValidAmount("10.5")).toBe(true);
    expect(isValidAmount("0.01")).toBe(true);
  });

  it("rejects zero, negatives, and malformed shapes", () => {
    expect(isValidAmount("0")).toBe(false);
    expect(isValidAmount("0.00")).toBe(false);
    expect(isValidAmount("10.123")).toBe(false);
    expect(isValidAmount("abc")).toBe(false);
    expect(isValidAmount(".5")).toBe(false);
    expect(isValidAmount("1.")).toBe(false);
    expect(isValidAmount("-5")).toBe(false);
  });

  it("allows an amount equal to the ceiling (exceeds is a strict >)", () => {
    expect(toCents("10000") > toCents("10000")).toBe(false);
  });
});
