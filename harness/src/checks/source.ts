import path from "node:path";
import type { HarnessConfig } from "../config.ts";
import { aiRoot, backendWebRoot, projectRoot } from "../lib/paths.ts";
import { hashFile, pathExists, readText, walkFiles } from "../lib/files.ts";
import { capture, fail, pass, warn, type CheckResult } from "../lib/result.ts";

const legacyBrandPattern = /DeepTrols|DEEPTROLS|deeptrols|智曜算力超市|api\.deeptrols/i;

function relative(filePath: string) {
  return path.relative(projectRoot, filePath);
}

async function checkProjectStructure(): Promise<CheckResult> {
  const required = ["ai-nuxt/app", "cmd/api", "cmd/worker", "internal", "web/src", "harness/src", "go.mod", "package.json", "README.md"];
  const missing: string[] = [];

  for (const item of required) {
    if (!(await pathExists(path.join(projectRoot, item)))) {
      missing.push(item);
    }
  }

  if (missing.length > 0) {
    return fail("project.structure", "项目根目录缺少必要工程目录。", missing);
  }

  return pass("project.structure", "前台、后台和 harness 工程目录齐全。");
}

async function checkLegacyArtifacts(): Promise<CheckResult> {
  const denied = ["ai", "webNew", "webNew-local", "clear-sw.html"];
  const existing: string[] = [];

  for (const item of denied) {
    if (await pathExists(path.join(projectRoot, item))) {
      existing.push(item);
    }
  }

  if (existing.length > 0) {
    return fail("project.legacy-artifacts", "仍存在应清理的旧资源。", existing);
  }

  return pass("project.legacy-artifacts", "旧静态目录和 clear-sw.html 均未出现。");
}

async function checkNuxtScriptSetup(): Promise<CheckResult> {
  const files = await walkFiles(path.join(aiRoot, "app"), [".vue"]);
  const violations: string[] = [];

  for (const file of files) {
    const content = await readText(file);
    const matches = content.matchAll(/<script\b([^>]*)>/gi);
    for (const match of matches) {
      const attrs = match[1] ?? "";
      if (!/\bsetup\b/i.test(attrs) || !/\blang=["']ts["']/i.test(attrs)) {
        violations.push(relative(file));
      }
    }
  }

  if (violations.length > 0) {
    return fail("nuxt.script-setup", "发现未使用 <script setup lang=\"ts\"> 的 Vue 文件。", violations);
  }

  return pass("nuxt.script-setup", "所有带脚本的 Vue 文件均使用 <script setup lang=\"ts\">。");
}

async function checkNoVueStyleBlocks(): Promise<CheckResult> {
  const files = await walkFiles(path.join(aiRoot, "app"), [".vue"]);
  const violations: string[] = [];

  for (const file of files) {
    const content = await readText(file);
    if (/<style\b/i.test(content)) {
      violations.push(relative(file));
    }
  }

  if (violations.length > 0) {
    return fail("nuxt.style-ownership", "Vue 组件中出现 style 块，应集中使用 SCSS token 与 Tailwind utilities。", violations);
  }

  return pass("nuxt.style-ownership", "Vue 组件未内联 style 块，样式职责集中。");
}

async function checkTailwindThemeBridge(): Promise<CheckResult> {
  const tailwind = await readText(path.join(aiRoot, "app/assets/css/tailwind.css"));
  const scss = await readText(path.join(aiRoot, "app/assets/scss/main.scss"));
  const missing: string[] = [];

  if (!tailwind.includes("@theme inline")) {
    missing.push("@theme inline");
  }
  if (!tailwind.includes("@source")) {
    missing.push("@source");
  }
  if (!tailwind.includes("var(--opc-")) {
    missing.push("Tailwind token bridge to --opc-*");
  }
  if (!scss.includes(":root") || !scss.includes("--opc-brand")) {
    missing.push("SCSS root design tokens");
  }

  if (missing.length > 0) {
    return fail("nuxt.tailwind-theme-bridge", "Tailwind CSS v4 theme bridge 不完整。", missing);
  }

  return pass("nuxt.tailwind-theme-bridge", "SCSS token 与 Tailwind CSS v4 theme bridge 已连接。");
}

async function checkRootSizingGuard(): Promise<CheckResult> {
  const appVue = await readText(path.join(aiRoot, "app/app.vue"));
  const required = ["w-full", "min-h-screen", "overflow-x-hidden"];
  const missing = required.filter((item) => !appVue.includes(item));

  if (missing.length > 0) {
    return fail("nuxt.root-sizing", "Nuxt 根容器缺少关键尺寸约束。", missing);
  }

  return pass("nuxt.root-sizing", "Nuxt 根容器保留 w-full / min-h-screen / overflow-x-hidden。");
}

async function checkHeaderNavGuard(): Promise<CheckResult> {
  const header = await readText(path.join(aiRoot, "app/components/app/AppHeader.vue"));
  const denied = ["文档", "OPC Store", "OPCstore"];
  const hits = denied.filter((item) => header.includes(item));

  if (hits.length > 0) {
    return fail("nuxt.header-nav", "Header 中仍包含不需要的导航项。", hits);
  }

  return pass("nuxt.header-nav", "Header 导航保持：首页、模型超市、控制中心。");
}

async function checkLegacyBrandInPageSources(): Promise<CheckResult> {
  const roots = [
    path.join(aiRoot, "app"),
    path.join(aiRoot, "nuxt.config.ts"),
    path.join(backendWebRoot, "src"),
    path.join(backendWebRoot, "index.html"),
  ];
  const files: string[] = [];

  for (const root of roots) {
    if (!(await pathExists(root))) {
      continue;
    }

    const statFiles = root.endsWith(".ts") || root.endsWith(".html") ? [root] : await walkFiles(root, [".vue", ".ts", ".tsx", ".css", ".scss", ".html"]);
    files.push(...statFiles);
  }

  const hits: string[] = [];
  for (const file of files) {
    const content = await readText(file);
    if (legacyBrandPattern.test(content)) {
      hits.push(relative(file));
    }
  }

  if (hits.length > 0) {
    return fail("brand.no-legacy-page-source", "页面层源码仍包含旧品牌或旧接口域名。", hits);
  }

  return pass("brand.no-legacy-page-source", "Nuxt 与后台 Web 页面层源码未发现旧品牌残留。");
}

async function checkLogoAssets(config: HarnessConfig): Promise<CheckResult> {
  const aiLogo = path.join(aiRoot, "public/logo.png");
  const backendLogo = path.join(backendWebRoot, "public/brand-logo.png");
  const aiHash = await hashFile(aiLogo);
  const backendHash = await hashFile(backendLogo);

  if (aiHash !== backendHash) {
    return fail("brand.logo-assets", "前台 logo 与后台控制台 logo 不一致。", {
      "ai-nuxt/public/logo.png": aiHash,
      "web/public/brand-logo.png": backendHash,
    });
  }

  if (config.expectedLogoSha256 && aiHash !== config.expectedLogoSha256) {
    return fail("brand.logo-assets", "当前 logo 与 harness 记录的目标 logo 不一致。", {
      current: aiHash,
      expected: config.expectedLogoSha256,
    });
  }

  return pass("brand.logo-assets", "前台与后台 logo 资源一致。", { sha256: aiHash });
}

async function checkPackageManagerBoundary(): Promise<CheckResult> {
  const backendPackageLock = await pathExists(path.join(backendWebRoot, "package-lock.json"));

  if (backendPackageLock) {
    return warn("project.package-manager", "后台控制台仍保留 npm lock；前台与 harness 使用 pnpm，后续可单独统一。");
  }

  return pass("project.package-manager", "未发现额外 npm lock。");
}

async function checkBackendNodeModulesBoundary(): Promise<CheckResult> {
  const backendWebNodeModules = await pathExists(path.join(backendWebRoot, "node_modules"));

  if (backendWebNodeModules) {
    return warn(
      "backend.go-module-scope",
      "前后端共用仓库；backend:test 仅扫描自有 Go 源码并复用 guard-test-db，必须配置独立 TEST_DATABASE_URL，不能将跳过数据库测试视为通过。",
    );
  }

  return pass("backend.go-module-scope", "Go module 目录内未发现前端 node_modules。");
}

export async function runSourceChecks(config: HarnessConfig): Promise<CheckResult[]> {
  return Promise.all([
    capture("project.structure", checkProjectStructure),
    capture("project.legacy-artifacts", checkLegacyArtifacts),
    capture("nuxt.script-setup", checkNuxtScriptSetup),
    capture("nuxt.style-ownership", checkNoVueStyleBlocks),
    capture("nuxt.tailwind-theme-bridge", checkTailwindThemeBridge),
    capture("nuxt.root-sizing", checkRootSizingGuard),
    capture("nuxt.header-nav", checkHeaderNavGuard),
    capture("brand.no-legacy-page-source", checkLegacyBrandInPageSources),
    capture("brand.logo-assets", () => checkLogoAssets(config)),
    capture("project.package-manager", checkPackageManagerBoundary),
    capture("backend.go-module-scope", checkBackendNodeModulesBoundary),
  ]);
}
