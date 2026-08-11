import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Policies from "./Policies";
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
const mockAdminPost = adminApi.post as ReturnType<typeof vi.fn>;

function seedPolicies() {
  return [
    { id: "p1", name: "GPT-4o 主路由", tenant_id: null, user_level: "all", model_id: "m1", priority: 10, candidate_channel_ids: ["c1"], candidate_channel_names: ["Channel 1"], fallback_policy: "disabled", is_active: true },
  ];
}

describe("Policies", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders page title and create button", async () => {
    mockAdminGet.mockResolvedValue({ data: seedPolicies() });

    renderWithProviders(<Policies />);

    expect(screen.getByText("路由策略")).toBeInTheDocument();
    expect(await screen.findByText("创建策略")).toBeInTheDocument();
  });

  it("shows loading spinner while fetching", () => {
    mockAdminGet.mockImplementation(() => new Promise(() => {}));

    renderWithProviders(<Policies />);

    expect(screen.getByText("加载路由策略...")).toBeInTheDocument();
  });

  it("displays policy list when loaded", async () => {
    mockAdminGet.mockResolvedValue({ data: seedPolicies() });

    renderWithProviders(<Policies />);

    expect(await screen.findByText("GPT-4o 主路由")).toBeInTheDocument();
  });

  it("shows empty state when there are no policies", async () => {
    mockAdminGet.mockResolvedValue({ data: [] });

    renderWithProviders(<Policies />);

    expect(await screen.findByText("暂无路由策略")).toBeInTheDocument();
  });

  it("shows error with retry on failure", async () => {
    mockAdminGet.mockRejectedValue(new Error("policies down"));

    renderWithProviders(<Policies />);

    expect(await screen.findByText("policies down")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });

  it("creates a policy on submit", async () => {
    const user = userEvent.setup();
    mockAdminGet.mockResolvedValue({ data: [] });
    mockAdminPost.mockResolvedValue({ id: "new" });

    renderWithProviders(<Policies />);

    await user.click(await screen.findByText("创建策略"));
    await user.type(screen.getByRole("textbox", { name: "策略名称" }), "New Policy");
    await user.type(screen.getByRole("textbox", { name: "模型编码" }), "gpt-4o");
    await user.click(screen.getByRole("button", { name: "创建" }));

    await waitFor(() => {
      expect(mockAdminPost).toHaveBeenCalled();
    });
  });
});
