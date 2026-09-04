import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";
import { pathToFileURL } from "node:url";
import { aiRoot, backendRoot, backendWebRoot, harnessRoot, projectRoot, reportsRoot } from "./paths.ts";
import { loadConfig } from "../config.ts";
import { runSourceChecks } from "../checks/source.ts";

test("all packages resolve inside the single repository", () => {
  assert.equal(backendRoot, projectRoot);
  assert.equal(aiRoot, path.join(projectRoot, "ai-nuxt"));
  assert.equal(backendWebRoot, path.join(projectRoot, "web"));
  assert.equal(harnessRoot, path.join(projectRoot, "harness"));
  assert.equal(reportsRoot, path.join(harnessRoot, "reports"));
  for (const file of ["go.mod", "README.md", "package.json", "ai-nuxt/package.json", "web/package.json", "harness/package.json"]) {
    assert.ok(fs.existsSync(path.join(projectRoot, file)), `Missing repository file: ${file}`);
  }
  assert.equal(fs.existsSync(path.join(aiRoot, ".git")), false, "Nuxt must not be a nested Git repository");
});

test("root commands target the relocated packages and preserve the DB test guard", () => {
  const { scripts } = JSON.parse(fs.readFileSync(path.join(projectRoot, "package.json"), "utf8"));
  assert.equal(scripts.dev, "pnpm --dir ai-nuxt dev");
  assert.equal(scripts["backend:web:dev"], "pnpm --dir web run dev --host 127.0.0.1");
  assert.equal(scripts["backend:test"], "make guard-test-db && go test -cover -count=1 ./cmd/... ./internal/... ./migrations/... ./ops/... ./scripts/... ./tools/...");
  assert.equal(scripts["install:ai"], "pnpm --dir ai-nuxt install --frozen-lockfile && pnpm --dir ai-nuxt exec nuxt prepare");
  assert.equal(scripts["install:web"], "npm --prefix web ci");
  assert.equal(scripts["install:harness"], "pnpm --dir harness install --frozen-lockfile");
  for (const command of Object.values(scripts) as string[]) {
    assert.doesNotMatch(command, /DeepTrolsTokenHub[\\/]|\.\.[\\/]/);
  }
});

test("path resolution does not depend on the caller's working directory", () => {
  const moduleUrl = pathToFileURL(path.join(harnessRoot, "src/lib/paths.ts")).href;
  const child = spawnSync(process.execPath, [
    "--experimental-strip-types", "--input-type=module", "-e",
    `const paths = await import(${JSON.stringify(moduleUrl)}); console.log(JSON.stringify(paths));`,
  ], { cwd: os.tmpdir(), encoding: "utf8" });
  assert.equal(child.status, 0, child.stderr || child.error?.message);
  const result = JSON.parse(child.stdout);
  assert.equal(result.projectRoot, projectRoot);
  assert.equal(result.backendRoot, projectRoot);
  assert.equal(result.backendWebRoot, backendWebRoot);
});

test("source and brand checks pass against the relocated repository", async () => {
  const results = await runSourceChecks(loadConfig());
  const failures = results.filter((result) => result.status === "fail");
  assert.deepEqual(failures, []);
  assert.ok(results.some((result) => result.name === "project.structure" && result.status === "pass"));
  assert.ok(results.some((result) => result.name === "brand.logo-assets" && result.status === "pass"));
});
