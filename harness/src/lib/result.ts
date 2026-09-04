export type CheckStatus = "pass" | "warn" | "fail";

export interface CheckResult {
  name: string;
  status: CheckStatus;
  message: string;
  details?: unknown;
}

export function pass(name: string, message: string, details?: unknown): CheckResult {
  return { name, status: "pass", message, details };
}

export function warn(name: string, message: string, details?: unknown): CheckResult {
  return { name, status: "warn", message, details };
}

export function fail(name: string, message: string, details?: unknown): CheckResult {
  return { name, status: "fail", message, details };
}

export async function capture(name: string, run: () => Promise<CheckResult>): Promise<CheckResult> {
  try {
    return await run();
  } catch (error) {
    return fail(name, error instanceof Error ? error.message : String(error));
  }
}

export function summarize(results: CheckResult[]) {
  return {
    pass: results.filter((item) => item.status === "pass").length,
    warn: results.filter((item) => item.status === "warn").length,
    fail: results.filter((item) => item.status === "fail").length,
  };
}

export function hasFailures(results: CheckResult[]) {
  return results.some((item) => item.status === "fail");
}

function icon(status: CheckStatus) {
  if (status === "pass") {
    return "PASS";
  }
  if (status === "warn") {
    return "WARN";
  }
  return "FAIL";
}

export function formatResults(results: CheckResult[]) {
  const lines: string[] = [];
  const summary = summarize(results);

  lines.push(`Harness result: ${summary.pass} passed, ${summary.warn} warnings, ${summary.fail} failures`);
  lines.push("");

  for (const result of results) {
    lines.push(`[${icon(result.status)}] ${result.name}: ${result.message}`);
    if (result.details !== undefined) {
      const details =
        typeof result.details === "string" ? result.details : JSON.stringify(result.details, null, 2);
      for (const line of details.split("\n")) {
        lines.push(`  ${line}`);
      }
    }
  }

  return lines.join("\n");
}
