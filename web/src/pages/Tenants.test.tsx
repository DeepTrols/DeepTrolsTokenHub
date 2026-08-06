import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import Tenants from "./Tenants";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

vi.mock("../lib/auth", () => ({
  useAuth: () => ({
    user: { id: "admin", email: "admin@test.com", name: "Admin", role: "admin", status: "active", totp_enabled: false },
    isLoading: false,
    isAuthenticated: true,
    logout: vi.fn(),
  }),
}));

import { adminApi } from "../lib/api";
const mockAdminGet = adminApi.get as ReturnType<typeof vi.fn>;

function seedTenants() {
  return [
    { id: "t1", code: "tenant-a", name: "Tenant A", status: "active", owner_id: null, status_reason: "", domains: [] },
  ];
}

describe("Tenants", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders page title and create button", async () => {
    mockAdminGet.mockResolvedValue({ data: seedTenants() });

    renderWithProviders(<Tenants />);

    expect(screen.getByText("租户管理")).toBeInTheDocument();
    expect(await screen.findByText("创建租户")).toBeInTheDocument();
  });

  it("shows loading spinner while fetching", () => {
    mockAdminGet.mockImplementation(() => new Promise(() => {}));

    renderWithProviders(<Tenants />);

    expect(screen.getByText("加载租户...")).toBeInTheDocument();
  });

  it("displays tenant list when loaded", async () => {
    mockAdminGet.mockResolvedValue({ data: seedTenants() });

    renderWithProviders(<Tenants />);

    expect(await screen.findByText("Tenant A")).toBeInTheDocument();
    expect(screen.getByText("tenant-a")).toBeInTheDocument();
  });

  it("shows empty state when there are no tenants", async () => {
    mockAdminGet.mockResolvedValue({ data: [] });

    renderWithProviders(<Tenants />);

    expect(await screen.findByText("暂无租户")).toBeInTheDocument();
  });

  it("shows error with retry on failure", async () => {
    mockAdminGet.mockRejectedValue(new Error("tenants down"));

    renderWithProviders(<Tenants />);

    expect(await screen.findByText("tenants down")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });
});
