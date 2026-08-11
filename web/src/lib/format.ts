/**
 * Formats a monetary value to exactly two decimal places using banker's
 * rounding (四舍六入五成双):
 * - 3rd decimal digit 0-4 → truncate (四舍);
 * - 3rd decimal digit 6-9 → round up (六入);
 * - exactly 5 → round toward the even cent (五成双), unless any digit follows
 *   the 5, in which case round up.
 *
 * JS `Number.toFixed` rounds half away from zero, so it cannot express this
 * rule. Amounts arrive as decimal strings from the wallet API, so the value is
 * parsed as a string to avoid float error around the "5" boundary.
 *
 * @param value numeric amount as a string or number; null/undefined/garbage
 *   fall back to "0.00".
 * @returns a fixed 2-decimal string such as "12.34" or "-0.76".
 */
export function formatAmount(value: number | string | null | undefined): string {
  if (value === null || value === undefined) return "0.00";

  let raw = String(value).trim();
  if (raw === "") return "0.00";

  // Expand scientific notation (e.g. "1e-7", "9.995e0") to a plain decimal
  // string before parsing. Expansion is pure string arithmetic so the mantissa
  // digits survive exactly — Number()+toFixed() would lose the last digits
  // (9.995e0 becomes 9.994999…), breaking the banker's-rounding 5-boundary.
  if (/[eE]/.test(raw)) {
    raw = expandExponent(raw) ?? "0.00";
  }

  const negative = raw.startsWith("-");
  const unsigned = raw.replace(/^[+-]/, "");

  // Only a plain decimal with an integer part is accepted; anything else is
  // treated as an unparseable amount.
  if (!/^\d+(\.\d*)?$/.test(unsigned)) return "0.00";

  // Normalize "-0.000" / "0.0000" to a non-negative zero.
  if (/^0+(\.0+)?$/.test(unsigned)) return "0.00";

  const [intRaw, fracRaw = ""] = unsigned.split(".");

  // Keep the two cent digits and inspect the 3rd decimal to decide rounding.
  const cents = (fracRaw + "00").slice(0, 2);
  const third = Number((fracRaw + "00")[2] ?? "0");
  const tailNonZero = /[1-9]/.test(fracRaw.slice(3));

  // 四舍六入五成双: round up when the dropped part is above 5, or exactly 5
  // followed by a non-zero digit, or exactly 5 rounding toward an odd cent.
  const roundUp =
    third > 5 || (third === 5 && (tailNonZero || Number(cents[1]) % 2 === 1));

  let intPart = intRaw.replace(/^0+(?=\d)/, "");
  let centsOut = cents;
  if (roundUp) {
    const [nextInt, nextCents] = incrementCents(intPart, cents);
    intPart = nextInt;
    centsOut = nextCents;
  }

  return `${negative ? "-" : ""}${intPart}.${centsOut}`;
}

/**
 * Adds 1 to a 2-digit cents string, propagating the carry into the integer
 * part (e.g. "99" → "00" with carry, so "9.995" becomes "10.00").
 * Returns [integerPart, cents].
 */
function incrementCents(intPart: string, cents: string): [string, string] {
  const centsArr = cents.split("");
  let carry = 1;
  for (let i = 1; i >= 0 && carry > 0; i--) {
    const sum = Number(centsArr[i]) + carry;
    centsArr[i] = String(sum % 10);
    carry = Math.floor(sum / 10);
  }
  if (carry === 0) return [intPart, centsArr.join("")];

  // Carry escaped past both cent digits → bump the integer part.
  const intArr = intPart.split("");
  for (let i = intArr.length - 1; i >= 0 && carry > 0; i--) {
    const sum = Number(intArr[i]) + carry;
    intArr[i] = String(sum % 10);
    carry = Math.floor(sum / 10);
  }
  if (carry > 0) intArr.unshift(String(carry));
  return [intArr.join(""), centsArr.join("")];
}

/**
 * Rewrites a number in scientific notation ("1.5e-3") as a plain decimal
 * string by shifting the decimal point, never passing through a float, so the
 * mantissa digits stay exact. Returns null when the input is not a
 * well-formed exponent form (e.g. "1e").
 */
function expandExponent(raw: string): string | null {
  const m = raw.match(/^([+-]?\d+(?:\.\d*)?)[eE]([+-]?\d+)$/);
  if (!m) return null;

  const sign = m[1].startsWith("-") ? "-" : "";
  const unsigned = m[1].replace(/^[+-]/, "");
  const [intPart, fracPart = ""] = unsigned.split(".");

  const digits = intPart + fracPart;
  const pointIndex = intPart.length + Number(m[2]);

  if (pointIndex <= 0) {
    return `${sign}0.${"0".repeat(-pointIndex)}${digits}`;
  }
  if (pointIndex >= digits.length) {
    return `${sign}${digits}${"0".repeat(pointIndex - digits.length)}`;
  }
  return `${sign}${digits.slice(0, pointIndex)}.${digits.slice(pointIndex)}`;
}
