import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen } from "@testing-library/react";
import Dashboard, {
  aggregateDaily,
  formatRangeLabel,
  gmt8DayKey,
  sumUsage,
  topModelByCost,
} from "./Dashboard";
import { renderWithProviders } from "../test/test-utils";
import { UsageLog } from "../lib/api";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

vi.mock("../lib/auth", () => ({
  useAuth: () => ({
    user: { id: "test-user", email: "test@test.com", name: "Test", role: "user", status: "active" },
    isLoading: false,
    isAuthenticated: true,
    logout: vi.fn(),
  }),
}));

import { api } from "../lib/api";
const mockApiGet = api.get as ReturnType<typeof vi.fn>;

const wallet = { balance: "60.00", frozen: "6.79", available: "53.21", currency: "CNY", total_charged: "306.78" };
const keys = [
  { id: "key-1", name: "codex", masked_key: "sk-codex-...", status: "active", created_at: "2026-08-01T00:00:00Z" },
];

function seedUsageLogs(): UsageLog[] {
  const iso = (daysAgo: number) => new Date(Date.now() - daysAgo * 24 * 60 * 60 * 1000).toISOString();
  return [
    {
      id: "log-1",
      model: "deepseek-v4-flash",
      request_id: "req-1",
      api_key_id: "key-1",
      api_key_name: "codex",
      status: "completed",
      input_tokens: 100_000_000,
      output_tokens: 100_000_000,
      cost: "30.00",
      created_at: iso(2),
    },
    {
      id: "log-2",
      model: "deepseek-v4-flash",
      request_id: "req-2",
      api_key_id: "key-1",
      api_key_name: "codex",
      status: "completed",
      input_tokens: 20_000_000,
      output_tokens: 15_000_000,
      cost: "5.00",
      created_at: iso(1),
    },
    {
      id: "log-3",
      model: "gpt-4o",
      request_id: "req-3",
      api_key_id: "key-1",
      api_key_name: "codex",
      status: "completed",
      input_tokens: 500_000,
      output_tokens: 467_318,
      cost: "1.90",
      created_at: iso(3),
    },
  ];
}

function mockEndpoints(logs: UsageLog[]) {
  mockApiGet.mockImplementation((path: string) => {
    if (path.startsWith("/wallet")) return Promise.resolve(wallet);
    if (path.startsWith("/api-keys")) return Promise.resolve({ data: keys });
    if (path.startsWith("/usage")) return Promise.resolve({ data: logs });
    return Promise.resolve({});
  });
}

describe("Dashboard（用量信息）", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders title, subtitle and the two top cards", async () => {
    mockEndpoints(seedUsageLogs());

    renderWithProviders(<Dashboard />);

    expect(screen.getByText("用量信息")).toBeInTheDocument();
    expect(screen.getByText("所有日期均按 GMT+8 时间显示，数据可能有 5 分钟延迟。")).toBeInTheDocument();
    expect(await screen.findByText("充值余额")).toBeInTheDocument();
    expect(screen.getByText(/¥53\.21/)).toBeInTheDocument();
    expect(screen.getByText("累计消费金额")).toBeInTheDocument();
    expect(screen.getByText(/¥306\.78/)).toBeInTheDocument();
    expect(screen.getByText("去充值")).toBeInTheDocument();
    expect(screen.getByText("余额预警已开启 去设置")).toBeInTheDocument();
  });

  it("renders filter toolbar with range, api key and export", async () => {
    mockEndpoints(seedUsageLogs());

    renderWithProviders(<Dashboard />);

    expect(await screen.findByText("清除筛选条件")).toBeInTheDocument();
    expect(screen.getByText("全部 API Key")).toBeInTheDocument();
    expect(screen.getByText("codex")).toBeInTheDocument();
    expect(screen.getByText("导出")).toBeInTheDocument();
    expect(screen.getByText(/近 7 天/)).toBeInTheDocument();
  });

  it("renders aggregated stat cards, chart tabs and the top model section", async () => {
    mockEndpoints(seedUsageLogs());

    renderWithProviders(<Dashboard />);

    expect(await screen.findByText("消费金额")).toBeInTheDocument();
    // 统计卡与底部模型小图都含这两个标题
    expect(screen.getAllByText("API 请求次数").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Tokens").length).toBeGreaterThan(0);
    expect(screen.getByText("¥36.90 CNY")).toBeInTheDocument();
    expect(screen.getAllByText(/¥36\.90/).length).toBeGreaterThan(0);
    expect(screen.getByText("3")).toBeInTheDocument();
    expect(screen.getByText("235,967,318")).toBeInTheDocument();

    expect(screen.getByText("消费金额（CNY）")).toBeInTheDocument();
    expect(screen.getByText("模型")).toBeInTheDocument();
    expect(screen.getByText("API Key")).toBeInTheDocument();

    // deepseek-v4-flash 是费用最高的模型（30 + 5 > 1.9）
    expect(await screen.findByText("deepseek-v4-flash")).toBeInTheDocument();
  });

  it("shows loading spinner while data is pending", () => {
    mockApiGet.mockImplementation(() => new Promise(() => {}));

    renderWithProviders(<Dashboard />);

    expect(screen.getByText("加载...")).toBeInTheDocument();
  });

  it("shows error state with retry when queries fail", async () => {
    mockApiGet.mockRejectedValue(new Error("network down"));

    renderWithProviders(<Dashboard />);

    expect(await screen.findByText("加载失败，请稍后重试")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });

  it("shows empty chart state and hides the model section when there are no logs", async () => {
    mockEndpoints([]);

    renderWithProviders(<Dashboard />);

    expect(await screen.findByText("暂无数据")).toBeInTheDocument();
    expect(screen.queryByText("deepseek-v4-flash")).not.toBeInTheDocument();
  });
});

describe("用量信息聚合工具", () => {
  it("gmt8DayKey buckets by GMT+8 civil date, not UTC", () => {
    expect(gmt8DayKey("2026-08-17T10:00:00Z")).toBe("2026-08-17");
    expect(gmt8DayKey("2026-08-17T20:30:00Z")).toBe("2026-08-18");
  });

  it("formatRangeLabel renders MM/DD-MM/DD", () => {
    const from = new Date("2026-08-17T00:00:00+08:00");
    const to = new Date("2026-08-20T23:59:59+08:00");
    expect(formatRangeLabel(from, to)).toBe("08/17-08/20");
  });

  it("sumUsage aggregates cost, requests and tokens", () => {
    const stats = sumUsage(seedUsageLogs());
    expect(stats.cost).toBe(36.9);
    expect(stats.requests).toBe(3);
    expect(stats.tokens).toBe(235_967_318);
  });

  it("aggregateDaily buckets logs by day and splits by model/key", () => {
    const from = new Date("2026-08-17T00:00:00+08:00");
    const to = new Date("2026-08-20T23:59:59+08:00");
    const daily = aggregateDaily(
      [
        {
          id: "a",
          model: "m1",
          request_id: "req-a",
          api_key_name: "k1",
          status: "completed",
          input_tokens: 1000,
          output_tokens: 500,
          cost: "10.00",
          created_at: "2026-08-17T03:00:00Z", // GMT+8 当天 11:00
        },
        {
          id: "b",
          model: "m2",
          request_id: "req-b",
          api_key_name: "k2",
          status: "completed",
          input_tokens: 100,
          output_tokens: 100,
          cost: "2.00",
          created_at: "2026-08-17T18:00:00Z", // GMT+8 次日 02:00 → 8/18
        },
      ],
      from,
      to,
    );

    expect(daily).toHaveLength(4);
    expect(daily[0].label).toBe("8/17");
    expect(daily[0].cost).toBe(10);
    expect(daily[0].models.m1).toBe(10);
    expect(daily[1].label).toBe("8/18");
    expect(daily[1].cost).toBe(2);
    expect(daily[1].keys.k2).toBe(2);
    expect(daily[0].requests).toBe(1);
    expect(daily[0].tokens).toBe(1500);
  });

  it("topModelByCost returns the most expensive model", () => {
    expect(topModelByCost(seedUsageLogs())).toBe("deepseek-v4-flash");
    expect(topModelByCost([])).toBe("");
  });
});
