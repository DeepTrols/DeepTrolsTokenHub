// GMT+8 date/time helpers shared by the usage dashboard and the range picker.
// All civil dates on these screens are Asia/Shanghai (GMT+8) dates.

export const GMT8_MS = 8 * 60 * 60 * 1000;

export type PresetKey = "today" | "yesterday" | "7d" | "30d" | "month" | "lastMonth" | "custom";

export function gmt8DayKey(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso.slice(0, 10);
  return new Date(d.getTime() + GMT8_MS).toISOString().slice(0, 10);
}

export function dayKey(year: number, month: number, day: number): string {
  const p = (n: number) => String(n).padStart(2, "0");
  return `${year}-${p(month + 1)}-${p(day)}`;
}

export function dayKeyToStart(key: string): Date {
  const [y, m, d] = key.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, d) - GMT8_MS);
}

export function dayKeyToEnd(key: string): Date {
  const [y, m, d] = key.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, d + 1) - GMT8_MS - 1);
}

export function gmt8DayStart(daysAgo: number, now = new Date()): Date {
  const g = new Date(now.getTime() + GMT8_MS);
  const startUtc = new Date(Date.UTC(g.getUTCFullYear(), g.getUTCMonth(), g.getUTCDate() - daysAgo));
  return new Date(startUtc.getTime() - GMT8_MS);
}

export function gmt8DayEnd(daysAgo: number, now = new Date()): Date {
  return new Date(gmt8DayStart(daysAgo, now).getTime() + 24 * 60 * 60 * 1000 - 1);
}

export function gmt8MonthStart(now = new Date()): Date {
  const g = new Date(now.getTime() + GMT8_MS);
  const startUtc = new Date(Date.UTC(g.getUTCFullYear(), g.getUTCMonth(), 1));
  return new Date(startUtc.getTime() - GMT8_MS);
}

export function gmt8LastMonthRange(now = new Date()): { from: Date; to: Date } {
  const g = new Date(now.getTime() + GMT8_MS);
  const firstOfThisMonth = new Date(Date.UTC(g.getUTCFullYear(), g.getUTCMonth(), 1));
  const firstOfLastMonth = new Date(Date.UTC(g.getUTCFullYear(), g.getUTCMonth() - 1, 1));
  return {
    from: new Date(firstOfLastMonth.getTime() - GMT8_MS),
    to: new Date(firstOfThisMonth.getTime() - GMT8_MS - 1),
  };
}

export function formatRangeLabel(from: Date, to: Date): string {
  const f = new Date(from.getTime() + GMT8_MS).toISOString().slice(5, 10).split("-").map(Number);
  const t = new Date(to.getTime() + GMT8_MS).toISOString().slice(5, 10).split("-").map(Number);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(f[0])}/${p(f[1])}-${p(t[0])}/${p(t[1])}`;
}

export function rangeForPreset(preset: PresetKey, now = new Date()): { from: Date; to: Date } {
  switch (preset) {
    case "today":
      return { from: gmt8DayStart(0, now), to: now };
    case "yesterday":
      return { from: gmt8DayStart(1, now), to: gmt8DayEnd(1, now) };
    case "30d":
      return { from: gmt8DayStart(29, now), to: now };
    case "month":
      return { from: gmt8MonthStart(now), to: now };
    case "lastMonth":
      return gmt8LastMonthRange(now);
    case "custom":
      return { from: gmt8DayStart(6, now), to: now };
    case "7d":
    default:
      return { from: gmt8DayStart(6, now), to: now };
  }
}

export function monthKeyFromInstant(instant: Date): { year: number; month: number } {
  const g = new Date(instant.getTime() + GMT8_MS);
  return { year: g.getUTCFullYear(), month: g.getUTCMonth() };
}
