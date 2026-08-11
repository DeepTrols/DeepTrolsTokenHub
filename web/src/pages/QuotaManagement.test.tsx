import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import QuotaManagement from "./QuotaManagement";
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

function seedPools() {
  return [
    { id: "pool-1", tenant_id: "t1", tenant_name: "Tenant A", model_id: "m1", model_code: "gpt-4o", model_name: "GPT-4o", dimension: "token", total_amount: 1000000, allocated_amount: 500000, used_amount: 200000, unit_name: "tokens", created_at: "2026-08-01T00:00:00Z", updated_at: "2026-08-01T00:00:00Z" },
  ];
}

describe("QuotaManagement", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders page title and summary cards", async () => {
    mockAdminGet.mockResolvedValue({ data: seedPools(), total: 1 });

    renderWithProviders(<QuotaManagement />);

    expect(screen.getByText("配额管理")).toBeInTheDocument();
    expect(await screen.findByText("配额池总数")).toBeInTheDocument();
    expect(screen.getByText("总配额量")).toBeInTheDocument();
    expect(screen.getAllByText("已使用").length).toBeGreaterThan(0);
  });

  it("shows loading spinner while fetching", () => {
    mockAdminGet.mockImplementation(() => new Promise(() => {}));

    renderWithProviders(<QuotaManagement />);

    expect(screen.getByText("加载配额数据...")).toBeInTheDocument();
  });

  it("displays quota table rows when loaded", async () => {
    mockAdminGet.mockResolvedValue({ data: seedPools(), total: 1 });

    renderWithProviders(<QuotaManagement />);

    expect(await screen.findByText("Tenant A")).toBeInTheDocument();
    expect(screen.getByText("GPT-4o")).toBeInTheDocument();
  });

  it("shows empty state when there are no pools", async () => {
    mockAdminGet.mockResolvedValue({ data: [], total: 0 });

    renderWithProviders(<QuotaManagement />);

    expect(await screen.findByText("暂无配额数据")).toBeInTheDocument();
  });

  it("shows error with retry on failure", async () => {
    mockAdminGet.mockRejectedValue(new Error("quota failed"));

    renderWithProviders(<QuotaManagement />);

    expect(await screen.findByText("quota failed")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });
});
