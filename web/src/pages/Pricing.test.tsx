import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Pricing, { type PricingResponse } from "./Pricing";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  publicApi: { get: vi.fn() },
}));

import { publicApi } from "../lib/api";
const mockPublicGet = publicApi.get as ReturnType<typeof vi.fn>;

const payload: PricingResponse = {
  data: [
    {
      id: "1",
      code: "deepseek-chat",
      provider: "deepseek",
      category: "chat",
      display_name: "DeepSeek Chat",
      context_window: 128000,
      pricings: [
        { dimension: "input", unit_name: "1M tokens", unit_price: "2.50", price_type: "sell", conditions: { max_total_tokens: 200000 } },
        { dimension: "input", unit_name: "1M tokens", unit_price: "2.00", price_type: "sell", conditions: { min_total_tokens: 200001 } },
        { dimension: "output", unit_name: "1M tokens", unit_price: "10.00", price_type: "sell" },
      ],
      pricing: { input: "2.50", output: "10.00" },
    },
    {
      id: "2",
      code: "glm-4",
      provider: "zhipu",
      category: "chat",
      display_name: "GLM-4",
      context_window: 200000,
      pricings: [
        { dimension: "input", unit_name: "1M tokens", unit_price: "3.00", price_type: "sell" },
      ],
      pricing: { input: "3.00" },
    },
  ],
  total: 2,
};

describe("Pricing", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockPublicGet.mockResolvedValue(payload);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders model cards with input/output pricing", async () => {
    renderWithProviders(<Pricing />);

    expect(await screen.findByText("DeepSeek Chat")).toBeInTheDocument();
    expect(screen.getByText("GLM-4")).toBeInTheDocument();
    expect(screen.getByText("¥2.50")).toBeInTheDocument();
    expect(screen.getByText("¥10.00")).toBeInTheDocument();
    expect(screen.getByText("128K 上下文")).toBeInTheDocument();
  });

  it("expands the full pricing detail", async () => {
    const user = userEvent.setup();
    renderWithProviders(<Pricing />);

    await screen.findByText("DeepSeek Chat");
    await user.click(screen.getAllByRole("button", { name: /查看完整定价/ })[0]);

    expect((await screen.findAllByText("输入")).length).toBeGreaterThan(0);
    expect(screen.getAllByText("售价").length).toBeGreaterThan(0);
    expect(screen.getAllByText("1M tokens").length).toBeGreaterThan(0);
  });

  it("shows tier conditions in the expanded pricing table", async () => {
    const user = userEvent.setup();
    renderWithProviders(<Pricing />);

    await screen.findByText("DeepSeek Chat");
    await user.click(screen.getAllByRole("button", { name: /查看完整定价/ })[0]);

    expect(await screen.findByText("≤ 200000")).toBeInTheDocument();
    expect(screen.getByText("≥ 200001")).toBeInTheDocument();
    expect(screen.getByText("档位")).toBeInTheDocument();
  });

  it("filters by search", async () => {
    const user = userEvent.setup();
    renderWithProviders(<Pricing />);

    await screen.findByText("DeepSeek Chat");
    await user.type(screen.getByPlaceholderText("搜索模型 / 厂商..."), "glm");

    await waitFor(() => {
      expect(screen.queryByText("DeepSeek Chat")).not.toBeInTheDocument();
    });
    expect(screen.getByText("GLM-4")).toBeInTheDocument();
  });

  it("shows the empty state when the catalog is empty", async () => {
    mockPublicGet.mockResolvedValue({ data: [], total: 0 });
    renderWithProviders(<Pricing />);

    expect(await screen.findByText("暂无模型定价")).toBeInTheDocument();
  });
});
