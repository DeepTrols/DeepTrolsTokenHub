import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import Reconciliation from "./Reconciliation";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

vi.mock("../lib/auth", () => ({
  useAuth: () => ({
    user: { id: "admin", email: "admin@test.com", name: "Admin", role: "admin", status: "active" },
    isLoading: false,
    isAuthenticated: true,
    logout: vi.fn(),
  }),
}));

import { adminApi } from "../lib/api";
const mockAdminGet = adminApi.get as ReturnType<typeof vi.fn>;

function seedRuns() {
  return [
    { id: "run-1", run_type: "L0", status: "completed", started_at: "2026-08-01T00:00:00Z", completed_at: "2026-08-01T00:01:00Z", total_usage_logs: 100, matched_count: 98, diff_count: 2, period_start: "2026-08-01T00:00:00Z", period_end: "2026-08-01T01:00:00Z" },
  ];
}

describe("Reconciliation", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders page title and summary cards", async () => {
    mockAdminGet.mockResolvedValue({ data: seedRuns(), total: 1 });

    renderWithProviders(<Reconciliation />);

    expect(screen.getByText("对账管理")).toBeInTheDocument();
    expect(await screen.findByText("对账总数")).toBeInTheDocument();
    expect(screen.getAllByText("已完成").length).toBeGreaterThan(0);
    expect(screen.getByText("累计差异")).toBeInTheDocument();
  });

  it("shows loading spinner while fetching", () => {
    mockAdminGet.mockImplementation(() => new Promise(() => {}));

    renderWithProviders(<Reconciliation />);

    expect(screen.getByText("加载对账数据...")).toBeInTheDocument();
  });

  it("displays reconciliation table rows when loaded", async () => {
    mockAdminGet.mockResolvedValue({ data: seedRuns(), total: 1 });

    renderWithProviders(<Reconciliation />);

    expect(await screen.findByText("L0 · 原始用量")).toBeInTheDocument();
    expect(screen.getAllByText("已完成").length).toBeGreaterThan(0);
  });

  it("shows empty state when there are no runs", async () => {
    mockAdminGet.mockResolvedValue({ data: [], total: 0 });

    renderWithProviders(<Reconciliation />);

    expect(await screen.findByText("暂无对账记录")).toBeInTheDocument();
  });

  it("shows error with retry on failure", async () => {
    mockAdminGet.mockRejectedValue(new Error("recon failed"));

    renderWithProviders(<Reconciliation />);

    expect(await screen.findByText("recon failed")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });
});
