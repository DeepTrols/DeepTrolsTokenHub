import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import Dashboard, { buildDailySeries, toDayKey, weekdayLabel } from "./Dashboard";
import { renderWithProviders } from "../test/test-utils";

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

const wallet = { balance: "100.00", frozen: "5.00", available: "95.00", currency: "CNY", total_charged: "50.00" };

function seedUsageLogs() {
  return [
    { id: "log-1", model: "gpt-4o", request_id: "req-1", status: "completed", input_tokens: 10, output_tokens: 20, cost: "0.50", created_at: "2026-08-01T00:00:00Z" },
    { id: "log-2", model: "claude-sonnet", request_id: "req-2", status: "failed", input_tokens: 5, output_tokens: 0, cost: "0.10", created_at: "2026-08-01T00:01:00Z" },
  ];
}

describe("Dashboard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders page title and four stat cards", async () => {
    mockApiGet.mockResolvedValueOnce(wallet);
    mockApiGet.mockResolvedValueOnce({ data: seedUsageLogs() });

    renderWithProviders(<Dashboard />);

    expect(screen.getByText("数据看板")).toBeInTheDocument();
    expect(await screen.findByText("可用余额")).toBeInTheDocument();
    expect(screen.getByText("今日请求")).toBeInTheDocument();
    expect(screen.getByText("今日费用")).toBeInTheDocument();
    expect(screen.getByText("异常请求")).toBeInTheDocument();
  });

  it("shows loading spinner while data is pending", () => {
    mockApiGet.mockImplementation(() => new Promise(() => {}));

    renderWithProviders(<Dashboard />);

    expect(screen.getByText("加载...")).toBeInTheDocument();
  });

  it("displays wallet balance and usage logs when loaded", async () => {
    mockApiGet.mockResolvedValueOnce(wallet);
    mockApiGet.mockResolvedValueOnce({ data: seedUsageLogs() });

    renderWithProviders(<Dashboard />);

    expect(await screen.findByText(/95\.00/)).toBeInTheDocument();
    expect(screen.getByText("gpt-4o")).toBeInTheDocument();
    expect(screen.getByText("claude-sonnet")).toBeInTheDocument();
  });

  it("shows error state with retry when queries fail", async () => {
    mockApiGet.mockRejectedValue(new Error("network down"));

    renderWithProviders(<Dashboard />);

    expect(await screen.findByText("network down")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });

  it("shows empty table message when there are no usage logs", async () => {
    mockApiGet.mockResolvedValueOnce(wallet);
    mockApiGet.mockResolvedValueOnce({ data: [] });

    renderWithProviders(<Dashboard />);

    expect(await screen.findByText("暂无调用记录")).toBeInTheDocument();
  });
});

describe("Dashboard daily series weekday labels", () => {
  it("labels the trailing day with its real weekday (Thursday, 2026-08-20)", () => {
    // Local-time constructor so the assertion is timezone-agnostic.
    const now = new Date(2026, 7, 20, 12);
    const daily = buildDailySeries([], now);
    expect(daily).toHaveLength(7);
    expect(daily[6].label).toBe("周四"); // today
    expect(daily[0].label).toBe("周五"); // 2026-08-14, six days earlier
  });

  it("labels a Sunday correctly", () => {
    const now = new Date(2026, 7, 16, 12); // Sunday
    const daily = buildDailySeries([], now);
    expect(daily[6].label).toBe("周日");
    expect(weekdayLabel(now)).toBe("周日");
  });

  it("buckets usage logs by local date, not UTC slice", () => {
    const now = new Date(2026, 7, 20, 12);
    const createdAt = new Date(2026, 7, 20, 0, 30).toISOString();
    const daily = buildDailySeries(
      [
        {
          id: "log-1",
          model: "gpt-4o",
          request_id: "req-1",
          status: "completed",
          input_tokens: 10,
          output_tokens: 5,
          cost: "0.15",
          created_at: createdAt,
        },
      ],
      now,
    );
    expect(daily[6].day).toBe(toDayKey(now));
    expect(daily[6].tokens).toBe(15);
    expect(daily[6].cost).toBe(0.15);
  });
});
