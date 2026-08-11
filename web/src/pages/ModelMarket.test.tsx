import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import ModelMarket from "./ModelMarket";
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

function seedModels() {
  return [
    { code: "gpt-4o", provider: "openai", category: "chat", display_name: "GPT-4o", context_window: 128000, pricing: { input: "2.50", output: "10.00" } },
    { code: "claude-sonnet", provider: "anthropic", category: "chat", display_name: "Claude Sonnet", context_window: 200000, pricing: { input: "3.00", output: "15.00" } },
  ];
}

describe("ModelMarket", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders page title and grouping tabs", async () => {
    mockApiGet.mockResolvedValue({ data: seedModels() });

    renderWithProviders(<ModelMarket />);

    expect(screen.getByText("模型广场")).toBeInTheDocument();
    expect(await screen.findByText("模型商家")).toBeInTheDocument();
    expect(screen.getByText("Token Plan")).toBeInTheDocument();
    expect(screen.getByText("模型工厂")).toBeInTheDocument();
  });

  it("shows loading spinner while fetching", () => {
    mockApiGet.mockImplementation(() => new Promise(() => {}));

    renderWithProviders(<ModelMarket />);

    expect(screen.getByText("加载模型列表...")).toBeInTheDocument();
  });

  it("displays model groups when loaded", async () => {
    mockApiGet.mockResolvedValue({ data: seedModels() });

    renderWithProviders(<ModelMarket />);

    expect(await screen.findByText("GPT-4o")).toBeInTheDocument();
    expect(screen.getByText("Claude Sonnet")).toBeInTheDocument();
  });

  it("shows error with retry on failure", async () => {
    mockApiGet.mockRejectedValue(new Error("models down"));

    renderWithProviders(<ModelMarket />);

    expect(await screen.findByText("models down")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });
});
