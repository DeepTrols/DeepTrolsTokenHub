// Pure helpers for API key limit form state and wire-format bodies.
// Kept side-effect free so they are unit-testable without a DOM.

import i18n from "../../i18n";

export interface KeyLimitValues {
  monthlyLimit: string;
  weeklyLimit: string;
  cumulativeLimit: string;
  rateLimitRpm: string;
  rateLimitTpm: string;
}

export interface KeyLimitsBody {
  monthly_limit?: string;
  weekly_limit?: string;
  cumulative_limit?: string;
  rate_limit_rpm?: number;
  rate_limit_tpm?: number;
}

export interface KeyLimitsResult {
  body: KeyLimitsBody;
  errors: string[];
}

/**
 * Parses a rate-limit input. Empty string means "not set" (undefined);
 * anything that is not a non-negative integer yields NaN.
 */
export function parseRateLimit(value: string): number | undefined {
  const trimmed = value.trim();
  if (trimmed === "") return undefined;
  const n = Number(trimmed);
  if (!Number.isInteger(n) || n < 0) return NaN;
  return n;
}

/** Builds the wire-format limits object from form strings, omitting empty fields. */
export function buildKeyLimitsBody(values: KeyLimitValues): KeyLimitsResult {
  const body: KeyLimitsBody = {};
  const errors: string[] = [];
  if (values.monthlyLimit.trim()) body.monthly_limit = values.monthlyLimit.trim();
  if (values.weeklyLimit.trim()) body.weekly_limit = values.weeklyLimit.trim();
  if (values.cumulativeLimit.trim()) body.cumulative_limit = values.cumulativeLimit.trim();

  const rpm = parseRateLimit(values.rateLimitRpm);
  if (rpm !== undefined) {
    if (Number.isNaN(rpm)) {
    errors.push(i18n.t("lib.rpmInvalid"));
    } else {
      body.rate_limit_rpm = rpm;
    }
  }
  const tpm = parseRateLimit(values.rateLimitTpm);
  if (tpm !== undefined) {
    if (Number.isNaN(tpm)) {
    errors.push(i18n.t("lib.tpmInvalid"));
    } else {
      body.rate_limit_tpm = tpm;
    }
  }
  return { body, errors };
}

/** Human-readable limit summary for the keys table; "不限" when nothing is set. */
export function formatRateLimit(rpm?: number | null, tpm?: number | null): string {
  const parts: string[] = [];
  if (typeof rpm === "number" && rpm > 0) parts.push(`${rpm} RPM`);
  if (typeof tpm === "number" && tpm > 0) parts.push(`${tpm} TPM`);
  return parts.length > 0 ? parts.join(" · ") : i18n.t("lib.unlimited");
}
