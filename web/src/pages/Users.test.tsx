import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen } from "@testing-library/react";
import Users from "./Users";
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

function seedUsers() {
  return [
    {
      id: "u1",
      email: "alice@example.com",
      display_name: "Alice",
      role: "user",
      user_type: "personal",
      status: "active",
      created_at: "2026-01-01T00:00:00Z",
    },
  ];
}

describe("Users（个人管理）", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders page title and create button", async () => {
    mockAdminGet.mockResolvedValue({ data: seedUsers(), total: 1 });

    renderWithProviders(<Users />);

    expect(screen.getByText("个人管理")).toBeInTheDocument();
    expect(await screen.findByText("创建个人用户")).toBeInTheDocument();
  });

  it("shows loading spinner while fetching", () => {
    mockAdminGet.mockImplementation(() => new Promise(() => {}));

    renderWithProviders(<Users />);

    expect(screen.getByText("加载个人账号...")).toBeInTheDocument();
  });

  it("displays personal accounts when loaded", async () => {
    mockAdminGet.mockResolvedValue({ data: seedUsers(), total: 1 });

    renderWithProviders(<Users />);

    expect(await screen.findByText("alice@example.com")).toBeInTheDocument();
    expect(screen.getByText("Alice")).toBeInTheDocument();
  });

  it("shows empty state when there are no personal accounts", async () => {
    mockAdminGet.mockResolvedValue({ data: [], total: 0 });

    renderWithProviders(<Users />);

    expect(await screen.findByText("暂无个人账号")).toBeInTheDocument();
  });

  it("shows error with retry on failure", async () => {
    mockAdminGet.mockRejectedValue(new Error("users down"));

    renderWithProviders(<Users />);

    expect(await screen.findByText("users down")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });
});
