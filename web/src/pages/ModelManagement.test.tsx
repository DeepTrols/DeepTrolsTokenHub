import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ModelManagement from "./ModelManagement";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  publicApi: { get: vi.fn() },
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
const mockAdminPut = adminApi.put as ReturnType<typeof vi.fn>;

function seedModels() {
  return [
    { id: "m1", code: "gpt-4o", provider: "openai", category: "chat", display_name: "GPT-4o", context_window: 128000, status: "active", pricings: [{ dimension: "input", unit_name: "token", unit_price: "2.50" }] },
  ];
}

const tieredModel = {
  id: "m1",
  code: "deepseek-tiers",
  provider: "deepseek",
  category: "chat",
  display_name: "DeepSeek Tiers",
  description: "",
  context_window: 128000,
  max_output_tokens: 0,
  status: "active",
  pricings: [
    { dimension: "input", unit_name: "1M tokens", unit_price: "3.00", price_type: "sell", period: "off_peak", conditions: { max_total_tokens: 200000 } },
    { dimension: "input", unit_name: "1M tokens", unit_price: "2.00", price_type: "sell", period: "off_peak", conditions: { min_total_tokens: 200001 } },
  ],
};

describe("ModelManagement 分级定价", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAdminGet.mockResolvedValue({ data: [tieredModel], total: 1 });
    mockAdminPut.mockResolvedValue({ ok: true });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders tier conditions on the model card", async () => {
    renderWithProviders(<ModelManagement />);

    expect(await screen.findByText("DeepSeek Tiers")).toBeInTheDocument();
    expect(screen.getByText("≤ 200000")).toBeInTheDocument();
    expect(screen.getByText("≥ 200001")).toBeInTheDocument();
  });

  it("edits a tier condition and submits it in the update body", async () => {
    const user = userEvent.setup();
    renderWithProviders(<ModelManagement />);

    await screen.findByText("DeepSeek Tiers");
    await user.click(screen.getByTitle("编辑"));

    const maxInputs = screen.getAllByPlaceholderText("最大 tokens");
    expect(maxInputs.length).toBeGreaterThan(0);
    await user.clear(maxInputs[0]);
    await user.type(maxInputs[0], "300000");

    await user.click(screen.getByRole("button", { name: "保存更改" }));

    await waitFor(() => {
      expect(mockAdminPut).toHaveBeenCalledWith(
        "/models/m1",
        expect.objectContaining({
          pricings: expect.arrayContaining([
            expect.objectContaining({
              dimension: "input",
              unit_price: "3.00",
              conditions: { max_total_tokens: 300000 },
            }),
          ]),
        }),
      );
    });

    // The sibling tier (min_total_tokens) is preserved untouched.
    const body = mockAdminPut.mock.calls[0][1] as { pricings: Array<{ unit_price: string; conditions?: Record<string, number> }> };
    const minTier = body.pricings.find((p) => p.unit_price === "2.00");
    expect(minTier?.conditions).toEqual({ min_total_tokens: 200001 });
  });
});

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
