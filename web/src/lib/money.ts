/**
 * Money helpers for the console UI.
 *
 * Money is always handled as decimal strings across the API boundary and never
 * as floats. These helpers sanitize, validate, format, and compare those
 * strings without ever performing arithmetic in floating point.
 */

/** Parses a non-negative decimal string ("10.50") into integer cents as BigInt. */
export function toCents(amount: string): bigint {
  const [int = "0", frac = ""] = amount.split(".");
  const fracPadded = (frac + "00").slice(0, 2);
  return BigInt(int) * 100n + BigInt(fracPadded);
}

/** Formats a decimal-string money amount as "1,234.56". Display only. */
export function fmtMoney(amount: string): string {
  const cents = toCents(amount);
  const int = cents / 100n;
  const frac = cents % 100n;
  return `${int.toLocaleString("en-US")}.${frac.toString().padStart(2, "0")}`;
}

/** Keeps only the digits of a CNY amount, allowing up to two decimals. */
export function toMoneyInput(raw: string): string {
  const cleaned = raw.replace(/[^\d.]/g, "");
  const dot = cleaned.indexOf(".");
  if (dot === -1) return cleaned;
  return (
    cleaned.slice(0, dot) +
    "." +
    cleaned.slice(dot + 1).replace(/\./g, "").slice(0, 2)
  );
}

/** True when the string is a positive amount with at most two decimals. */
export function isValidAmount(amount: string): boolean {
  return /^\d+(\.\d{1,2})?$/.test(amount) && toCents(amount) > 0n;
}
