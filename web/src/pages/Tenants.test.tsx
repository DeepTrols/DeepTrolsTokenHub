import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen } from "@testing-library/react";
import Tenants from "./Tenants";
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

function seedTenants() {
  return [
    {
      id: "t1",
      code: "tenant-a",
      name: "Tenant A",
      status: "active",
      status_reason: "",
      created_at: "2026-01-01T00:00:00Z",
    },
  ];
}

describe("Tenants（企业管理）", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders page title and create button", async () => {
    mockAdminGet.mockResolvedValue({ data: seedTenants(), total: 1 });

    renderWithProviders(<Tenants />);

    expect(screen.getByText("企业管理")).toBeInTheDocument();
    expect(await screen.findByText("创建企业")).toBeInTheDocument();
  });

  it("shows loading spinner while fetching", () => {
    mockAdminGet.mockImplementation(() => new Promise(() => {}));

    renderWithProviders(<Tenants />);

    expect(screen.getByText("加载企业...")).toBeInTheDocument();
  });

  it("displays enterprise list when loaded", async () => {
    mockAdminGet.mockResolvedValue({ data: seedTenants(), total: 1 });

    renderWithProviders(<Tenants />);

    expect(await screen.findByText("Tenant A")).toBeInTheDocument();
    expect(screen.getByText("tenant-a")).toBeInTheDocument();
  });

  it("shows empty state when there are no enterprises", async () => {
    mockAdminGet.mockResolvedValue({ data: [], total: 0 });

    renderWithProviders(<Tenants />);

    expect(await screen.findByText("暂无企业")).toBeInTheDocument();
  });

  it("shows error with retry on failure", async () => {
    mockAdminGet.mockRejectedValue(new Error("tenants down"));

    renderWithProviders(<Tenants />);

    expect(await screen.findByText("tenants down")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });
});
