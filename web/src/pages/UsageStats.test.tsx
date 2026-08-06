import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import UsageStats from "./UsageStats";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

vi.mock("../lib/auth", () => ({
  useAuth: () => ({
    user: { id: "test-user", email: "test@test.com", name: "Test", role: "user", status: "active", totp_enabled: false },
    isLoading: false,
    isAuthenticated: true,
    logout: vi.fn(),
  }),
}));

import { api } from "../lib/api";
const mockApiGet = api.get as ReturnType<typeof vi.fn>;

function seedLogs() {
  return [
    { id: "log-1", model: "gpt-4o", status: "completed", input_tokens: 100, output_tokens: 200, cost: "3.00", created_at: "2026-08-01T00:00:00Z" },
    { id: "log-2", model: "claude-sonnet", status: "completed", input_tokens: 50, output_tokens: 50, cost: "1.50", created_at: "2026-08-01T00:01:00Z" },
  ];
}

describe("UsageStats", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders page title and loading skeleton", () => {
    mockApiGet.mockImplementation(() => new Promise(() => {}));

    renderWithProviders(<UsageStats />);

    expect(screen.getByText("用量统计")).toBeInTheDocument();
    expect(screen.getByText(/Token 消耗趋势/)).toBeInTheDocument();
  });

  it("renders model breakdown and daily trend when data loads", async () => {
    mockApiGet.mockResolvedValue({ data: seedLogs(), total: 2 });

    renderWithProviders(<UsageStats />);

    expect(await screen.findByText("各模型 Token 用量")).toBeInTheDocument();
    expect(screen.getByText("费用趋势（近7日）")).toBeInTheDocument();
    expect(screen.getByText("gpt-4o")).toBeInTheDocument();
    expect(screen.getByText("claude-sonnet")).toBeInTheDocument();
  });

  it("shows empty state when there is no data", async () => {
    mockApiGet.mockResolvedValue({ data: [], total: 0 });

    renderWithProviders(<UsageStats />);

    expect(await screen.findByText("暂无用量数据")).toBeInTheDocument();
  });

  it("shows error with retry on fetch failure", async () => {
    mockApiGet.mockRejectedValue(new Error("stats failed"));

    renderWithProviders(<UsageStats />);

    expect(await screen.findByText("stats failed")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });
});
