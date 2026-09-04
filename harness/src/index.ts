import fs from "node:fs/promises";
import process from "node:process";
import { loadConfig } from "./config.ts";
import { runRuntimeChecks } from "./checks/runtime.ts";
import { runSourceChecks } from "./checks/source.ts";
import { formatResults, hasFailures, summarize, type CheckResult } from "./lib/result.ts";
import { reportsRoot } from "./lib/paths.ts";

type Mode = "audit" | "smoke" | "check";

function parseMode(raw: string | undefined): Mode {
  if (raw === "audit" || raw === "smoke" || raw === "check" || raw === undefined) {
    return raw ?? "check";
  }

  throw new Error(`Unknown harness mode "${raw}". Use audit, smoke, or check.`);
}

async function writeReport(mode: Mode, results: CheckResult[]) {
  await fs.mkdir(reportsRoot, { recursive: true });
  const report = {
    generatedAt: new Date().toISOString(),
    mode,
    summary: summarize(results),
    results,
  };

  await fs.writeFile(`${reportsRoot}/latest.json`, `${JSON.stringify(report, null, 2)}\n`, "utf8");
}

async function main() {
  const mode = parseMode(process.argv[2]);
  const config = loadConfig();
  const results: CheckResult[] = [];

  if (mode === "audit" || mode === "check") {
    results.push(...(await runSourceChecks(config)));
  }

  if (mode === "smoke" || mode === "check") {
    results.push(...(await runRuntimeChecks(config)));
  }

  await writeReport(mode, results);
  console.log(formatResults(results));

  if (hasFailures(results)) {
    process.exitCode = 1;
  }
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
});
