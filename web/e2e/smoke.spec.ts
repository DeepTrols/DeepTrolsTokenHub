import { test, expect, type Page } from "@playwright/test";
import os from "node:os";
import path from "node:path";

// Requires a local stack:
//   - API on 127.0.0.1:8082 (CORS_ORIGIN=http://localhost:3000)
//   - vite dev server on :3000 with PROXY_TARGET=http://127.0.0.1:8082
//   - echo upstream on 127.0.0.1:8090 (go run ./scripts/echo_upstream)
const ECHO = "http://127.0.0.1:8090";
const AUTH_STATE = path.join(os.tmpdir(), "deeptrols-e2e-auth.json");

test.describe.configure({ mode: "serial" });

// 登录一次并保存会话（登录接口有 IP 限流 5 次/分钟，串行复跑时逐条登录会
// 被 429 限流；改用 storageState 共享同一会话）。
test("登录并保存会话", async ({ page }) => {
  await page.context().clearCookies();
  let ok = false;
  for (let attempt = 0; attempt < 3 && !ok; attempt++) {
    await page.goto("/login");
    await page.locator("#email").fill("deeptrols@admin.com");
    await page.locator("#password").fill("deeptrols@2026");
    await page.getByRole("button", { name: "登 录" }).click();
    try {
      await page.waitForURL(/dashboard/, { timeout: 10_000 });
      ok = true;
    } catch {
      await page.waitForTimeout(15_000);
    }
  }
  if (!ok) {
    throw new Error("登录失败（可能被限流或凭据错误）");
  }
  await page.context().storageState({ path: AUTH_STATE });
});

test.describe("已登录", () => {
  test.use({ storageState: AUTH_STATE });

test("核心链路冒烟：建渠道→模型目录→网关调用→账单/审计", async ({ page, context }) => {
  // 1) 登录
  await page.goto("/dashboard");
  await expect(page.getByRole("heading", { name: "用量信息" })).toBeVisible({ timeout: 10_000 });

  // 幂等清理上次运行残留（同一 admin 会话）
  const staleKeys = await context.request.get("/api/console/api-keys");
  if (staleKeys.ok()) {
    const keyList = (await staleKeys.json()) as { data: Array<{ id: string; name: string }> };
    for (const k of keyList.data.filter((k) => k.name === "E2E 冒烟密钥")) {
      await context.request.delete(`/api/console/api-keys/${k.id}`);
    }
  }
  const staleProviders = await context.request.get("/api/admin/providers");
  if (staleProviders.ok()) {
    const providerList = (await staleProviders.json()) as { data: Array<{ id: string; name: string }> };
    for (const p of providerList.data.filter((p) => p.name === "E2E 冒烟渠道")) {
      await context.request.delete(`/api/admin/providers/${p.id}`);
    }
  }

  // 2) 建渠道（Provider 指向本地 echo 上游，确保冒烟不依赖外网）
  await page.goto("/admin/channels");
  await page.getByRole("button", { name: "添加渠道" }).click();
  await page.getByPlaceholder(/例如: DeepSeek 深度求索 生产环境/).fill("E2E 冒烟渠道");
  await page.getByPlaceholder("sk-...").fill("sk-e2e");
  await page.getByPlaceholder("默认自动填充").fill(ECHO);
  await page.getByRole("button", { name: "提交" }).click();
  await expect(page.getByText("E2E 冒烟渠道")).toBeVisible({ timeout: 30_000 });

  // 3) 模型目录出现 echo 发现的模型
  await page.goto("/admin/models");
  await expect(page.getByText("deepseek-chat", { exact: true }).first()).toBeVisible({ timeout: 30_000 });

  // 4) 网关真实调用一次（创建 API key → chat → echo 返回）
  const keyResp = await context.request.post("/api/console/api-keys", {
    data: { name: "E2E 冒烟密钥", allowed_models: ["deepseek-chat"], monthly_limit: "500" },
  });
  expect(keyResp.ok()).toBeTruthy();
  const keyJson = (await keyResp.json()) as { ID?: string; plaintext: string };
  const keyId = keyJson.ID;
  expect(keyJson.plaintext).toBeTruthy();

  const chatResp = await context.request.post("/v1/chat/completions", {
    headers: { Authorization: `Bearer ${keyJson.plaintext}` },
    data: { model: "deepseek-chat", messages: [{ role: "user", content: "hi" }] },
  });
  expect(chatResp.ok()).toBeTruthy();
  const chat = (await chatResp.json()) as { choices?: Array<{ message?: { content?: string } }> };
  expect(chat.choices?.[0]?.message?.content).toBe("echo");

  // 5) 账单/用量可见
  await page.goto("/bills");
  await expect(page.getByRole("heading", { name: "充值记录" })).toBeVisible({ timeout: 30_000 });
  await page.goto("/dashboard");
  await expect(page.getByRole("heading", { name: "用量信息" })).toBeVisible({ timeout: 30_000 });
  await expect(page.getByText("按模型查看")).toBeVisible({ timeout: 30_000 });

  // 6) 审计可见（建渠道是 admin 写操作）
  await page.goto("/admin/audit");
  await expect(page.getByText(/providers|渠道/).first()).toBeVisible({ timeout: 30_000 });

  // 清理：删临时 key 与渠道（API 方式，更稳）
  if (keyId) {
    await context.request.delete(`/api/console/api-keys/${keyId}`);
  }
  const providers = await context.request.get("/api/admin/providers");
  if (providers.ok()) {
    const list = (await providers.json()) as { data: Array<{ id: string; name: string }> };
    for (const p of list.data.filter((p) => p.name === "E2E 冒烟渠道")) {
      await context.request.delete(`/api/admin/providers/${p.id}`);
    }
  }
});

test("i18n 语言切换 + 余额预警阈值", async ({ page, context }) => {
  await page.goto("/dashboard");
  await expect(page.getByRole("heading", { name: "用量信息" })).toBeVisible({ timeout: 10_000 });

  // 切换到英文：导航与用量页标题联动。
  await page.getByRole("button", { name: "Switch language" }).click();
  await expect(page.getByRole("heading", { name: "Usage" })).toBeVisible({ timeout: 10_000 });

  // 账单页出现余额预警卡片（英文），保存阈值。
  await page.goto("/bills");
  await expect(page.getByRole("heading", { name: "Balance Alert" })).toBeVisible({ timeout: 10_000 });
  await page.locator("#balance-alert-threshold").fill("10");
  await page.getByRole("button", { name: "Save Threshold" }).click();
  await expect(page.locator("#balance-alert-threshold")).toHaveValue("10", { timeout: 10_000 });

  // 清理：阈值重置为 0（关闭预警）。
  const reset = await context.request.put("/api/console/wallet/alert", {
    data: { threshold: "0" },
  });
  expect(reset.ok()).toBeTruthy();
});

test("分组与折扣档位管理（Billing 设置）", async ({ page, context }) => {
  await page.goto("/admin/settings/billing");
  await expect(page.getByRole("heading", { name: "计费与支付" })).toBeVisible({ timeout: 10_000 });

  // 分组：添加 enterprise 分组（倍率 0.6）。
  await page.getByRole("tab", { name: "分组" }).click();
  await page.getByRole("button", { name: "添加分组" }).click();
  const groupNames = page.getByLabel("分组名称");
  await groupNames.last().fill("enterprise");
  await page.getByLabel("倍率").last().fill("0.6");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(groupNames.last()).toHaveValue("enterprise", { timeout: 10_000 });

  // 折扣：添加 500 万 tokens 档（0.9）。
  await page.getByRole("tab", { name: "折扣" }).click();
  await page.getByRole("button", { name: "添加档位" }).click();
  const minTokens = page.getByLabel("最低 tokens");
  await minTokens.last().fill("5000000");
  await page.getByLabel("折扣率 (0-1)").last().fill("0.9");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(minTokens.last()).toHaveValue("5000000", { timeout: 10_000 });

  // 清理：恢复空配置。
  const reset = await context.request.put("/api/admin/settings/site", {
    data: { user_groups: "[]", discount_tiers: "[]" },
  });
  expect(reset.ok()).toBeTruthy();
});

test("月度账单卡片可见", async ({ page }) => {
  await page.goto("/bills");
  await expect(page.getByRole("heading", { name: "月度账单" })).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText("本月消费", { exact: true })).toBeVisible({ timeout: 10_000 });
  await expect(page.getByLabel("选择月份")).toBeVisible();
});

test("登录会话卡片可见", async ({ page }) => {
  await page.goto("/account");
  await page.getByRole("tab", { name: "登录记录" }).click();
  await expect(page.getByRole("heading", { name: "登录会话" })).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText("当前会话", { exact: true })).toBeVisible({ timeout: 10_000 });
});
});
