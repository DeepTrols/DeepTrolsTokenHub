import type { HarnessConfig } from "../config.ts";
import { capture, fail, pass, type CheckResult } from "../lib/result.ts";
import { getJson, getText } from "../lib/http.ts";
import { pageText } from "../lib/html.ts";

interface HealthBody {
  status?: string;
}

interface ReadyzBody {
  status?: string;
  checks?: Record<string, string>;
}

interface SiteBody {
  site_name?: string;
  logo_url?: string;
  favicon_url?: string;
}

function oldBrandPattern() {
  return /DeepTrols|DEEPTROLS|deeptrols|智曜算力超市/i;
}

function slash(baseUrl: string, pathName: string) {
  return `${baseUrl}${pathName.startsWith("/") ? pathName : `/${pathName}`}`;
}

async function checkApiHealth(config: HarnessConfig): Promise<CheckResult> {
  const response = await getJson<HealthBody>(slash(config.apiBaseUrl, "/health"), config.timeoutMs);
  if (!response.ok || response.body.status !== "ok") {
    return fail("runtime.api.health", "API 健康检查未返回 ok。", response.body);
  }
  return pass("runtime.api.health", "API health 正常。");
}

async function checkApiReady(config: HarnessConfig): Promise<CheckResult> {
  const response = await getJson<ReadyzBody>(slash(config.apiBaseUrl, "/readyz"), config.timeoutMs);
  if (!response.ok || response.body.status !== "ready" || response.body.checks?.database !== "ok") {
    return fail("runtime.api.readyz", "API 就绪检查未通过。", response.body);
  }
  return pass("runtime.api.readyz", "API readyz 正常，数据库可用。");
}

async function checkPublicSite(config: HarnessConfig): Promise<CheckResult> {
  const response = await getJson<SiteBody>(slash(config.apiBaseUrl, "/api/public/site"), config.timeoutMs);
  const issues: string[] = [];

  if (response.body.site_name !== config.expectedBrand) {
    issues.push(`site_name=${response.body.site_name ?? ""}`);
  }
  if (response.body.logo_url !== "/brand-logo.png") {
    issues.push(`logo_url=${response.body.logo_url ?? ""}`);
  }
  if (response.body.favicon_url !== "/brand-logo.png") {
    issues.push(`favicon_url=${response.body.favicon_url ?? ""}`);
  }

  if (issues.length > 0) {
    return fail("runtime.api.public-site", "站点公开配置与目标品牌不一致。", issues);
  }

  return pass("runtime.api.public-site", "站点公开配置已使用目标品牌。");
}

async function checkConsoleLogin(config: HarnessConfig): Promise<CheckResult> {
  const response = await getText(slash(config.consoleBaseUrl, "/login"), config.timeoutMs);
  if (!response.ok) {
    return fail("runtime.console.login", "后台登录页不可访问。", { status: response.status });
  }

  const issues: string[] = [];
  if (!response.body.includes(`${config.expectedBrand} - AI Token Platform`)) {
    issues.push("页面 title 未包含目标品牌");
  }
  if (oldBrandPattern().test(pageText(response.body))) {
    issues.push("页面标题或文本中出现旧品牌");
  }

  if (issues.length > 0) {
    return fail("runtime.console.login", "后台登录页品牌检查未通过。", issues);
  }

  return pass("runtime.console.login", "后台登录页可访问，HTML 标题品牌正确。");
}

async function checkConsoleLogo(config: HarnessConfig): Promise<CheckResult> {
  const response = await getText(slash(config.consoleBaseUrl, "/brand-logo.png"), config.timeoutMs);
  const contentType = response.headers["content-type"] ?? "";

  if (!response.ok || !contentType.includes("image/png")) {
    return fail("runtime.console.logo", "后台 logo 静态资源不可用。", {
      status: response.status,
      contentType,
    });
  }

  return pass("runtime.console.logo", "后台 logo 静态资源可访问。");
}

async function checkNuxtPage(config: HarnessConfig, route: string): Promise<CheckResult> {
  const url = slash(config.aiBaseUrl, route);
  const response = await getText(url, config.timeoutMs);

  if (!response.ok) {
    return fail(`runtime.nuxt.${route || "home"}`, "Nuxt 页面不可访问。", {
      url,
      status: response.status,
    });
  }

  const issues: string[] = [];
  if (oldBrandPattern().test(pageText(response.body))) {
    issues.push("页面标题或文本中出现旧品牌");
  }
  if (!response.body.includes(config.expectedBrand) && !response.body.includes("/ai/logo.png")) {
    issues.push("HTML 中没有目标品牌或目标 logo 线索");
  }

  if (issues.length > 0) {
    return fail(`runtime.nuxt.${route || "home"}`, "Nuxt 页面品牌检查未通过。", issues);
  }

  return pass(`runtime.nuxt.${route || "home"}`, `Nuxt ${route || "/"} 可访问。`);
}

export async function runRuntimeChecks(config: HarnessConfig): Promise<CheckResult[]> {
  return Promise.all([
    capture("runtime.api.health", () => checkApiHealth(config)),
    capture("runtime.api.readyz", () => checkApiReady(config)),
    capture("runtime.api.public-site", () => checkPublicSite(config)),
    capture("runtime.console.login", () => checkConsoleLogin(config)),
    capture("runtime.console.logo", () => checkConsoleLogo(config)),
    capture("runtime.nuxt.home", () => checkNuxtPage(config, "/")),
    capture("runtime.nuxt.pricing", () => checkNuxtPage(config, "/pricing")),
  ]);
}
