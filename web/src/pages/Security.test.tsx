import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen } from "@testing-library/react";
import Security from "./Security";
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

function seedHistory() {
  return [
    { ip_address: "192.168.1.1", user_agent: "Chrome/120", success: true, created_at: "2026-08-01T00:00:00Z" },
  ];
}

describe("Security", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders page title and login history card", async () => {
    mockApiGet.mockResolvedValue({ data: seedHistory(), total: 1 });

    renderWithProviders(<Security />);

    expect(screen.getByText("安全设置")).toBeInTheDocument();
    expect(screen.getByText("登录记录")).toBeInTheDocument();
    expect(await screen.findByText("192.168.1.1")).toBeInTheDocument();
  });

  it("displays login history rows when loaded", async () => {
    mockApiGet.mockResolvedValue({ data: seedHistory(), total: 1 });

    renderWithProviders(<Security />);

    expect(await screen.findByText("192.168.1.1")).toBeInTheDocument();
  });

  it("shows empty state when there is no login history", async () => {
    mockApiGet.mockResolvedValue({ data: [], total: 0 });

    renderWithProviders(<Security />);

    expect(await screen.findByText("暂无登录记录")).toBeInTheDocument();
  });
});
