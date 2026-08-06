import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import ModelManagement from "./ModelManagement";
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
const mockAdminPost = adminApi.post as ReturnType<typeof vi.fn>;

function seedModels() {
  return [
    { id: "m1", code: "gpt-4o", provider: "openai", category: "chat", display_name: "GPT-4o", context_window: 128000, status: "active", pricings: [{ dimension: "input", unit_name: "token", unit_price: "2.50" }] },
  ];
}

describe("ModelManagement", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders page title and add model button", async () => {
    mockAdminGet.mockResolvedValue({ data: seedModels(), total: 1 });

    renderWithProviders(<ModelManagement />);

    expect(screen.getByText("模型管理")).toBeInTheDocument();
    expect(await screen.findByText("添加模型")).toBeInTheDocument();
  });

  it("shows loading spinner while fetching", () => {
    mockAdminGet.mockImplementation(() => new Promise(() => {}));

    renderWithProviders(<ModelManagement />);

    expect(screen.getByText("加载模型中...")).toBeInTheDocument();
  });

  it("displays model list when loaded", async () => {
    mockAdminGet.mockResolvedValue({ data: seedModels(), total: 1 });

    renderWithProviders(<ModelManagement />);

    expect(await screen.findByText("GPT-4o")).toBeInTheDocument();
  });

  it("shows empty state when there are no models", async () => {
    mockAdminGet.mockResolvedValue({ data: [], total: 0 });

    renderWithProviders(<ModelManagement />);

    expect(await screen.findByText("暂无模型")).toBeInTheDocument();
  });

  it("shows error with retry on failure", async () => {
    mockAdminGet.mockRejectedValue(new Error("models down"));

    renderWithProviders(<ModelManagement />);

    expect(await screen.findByText("models down")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });
});
