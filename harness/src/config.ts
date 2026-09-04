import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { harnessRoot } from "./lib/paths.ts";

export interface HarnessConfig {
  aiBaseUrl: string;
  apiBaseUrl: string;
  consoleBaseUrl: string;
  expectedBrand: string;
  expectedLogoSha256: string;
  timeoutMs: number;
}

function loadEnvFile(filePath: string) {
  if (!fs.existsSync(filePath)) {
    return;
  }

  const content = fs.readFileSync(filePath, "utf8");
  for (const rawLine of content.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#")) {
      continue;
    }

    const eq = line.indexOf("=");
    if (eq === -1) {
      continue;
    }

    const key = line.slice(0, eq).trim();
    const value = line.slice(eq + 1).trim().replace(/^["']|["']$/g, "");
    if (key && process.env[key] === undefined) {
      process.env[key] = value;
    }
  }
}

function envValue(key: string, fallback: string) {
  const value = process.env[key];
  return value && value.trim() ? value.trim() : fallback;
}

function envNumber(key: string, fallback: number) {
  const value = Number.parseInt(envValue(key, String(fallback)), 10);
  return Number.isFinite(value) && value > 0 ? value : fallback;
}

function trimTrailingSlash(value: string) {
  return value.replace(/\/+$/, "");
}

export function loadConfig(): HarnessConfig {
  loadEnvFile(path.join(harnessRoot, ".env"));

  return {
    aiBaseUrl: trimTrailingSlash(envValue("AI_BASE_URL", "http://127.0.0.1:4173/ai")),
    apiBaseUrl: trimTrailingSlash(envValue("API_BASE_URL", "http://127.0.0.1:8080")),
    consoleBaseUrl: trimTrailingSlash(envValue("CONSOLE_BASE_URL", "http://127.0.0.1:3000")),
    expectedBrand: envValue("EXPECTED_BRAND", "智曜TokenHub"),
    expectedLogoSha256: envValue(
      "EXPECTED_LOGO_SHA256",
      "19e29c89e02e8995894ab651e24401ac93539cc153aed8269328e92f2b06dce5",
    ),
    timeoutMs: envNumber("HTTP_TIMEOUT_MS", 8000),
  };
}
