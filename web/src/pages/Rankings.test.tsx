import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Rankings, { type RankingsSnapshot } from "./Rankings";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  publicApi: { get: vi.fn() },
}));

import { publicApi } from "../lib/api";
const mockPublicGet = publicApi.get as ReturnType<typeof vi.fn>;

const snapshot: RankingsSnapshot = {
  models: [
    {
      rank: 1,
      previous_rank: 2,
      model_name: "deepseek-chat",
      vendor: "deepseek",
      category: "all",
      total_tokens: 1_500_000,
      share: 0.6,
      growth_pct: 25,
    },
    {
      rank: 2,
      model_name: "glm-4",
      vendor: "zhipu",
      category: "all",
      total_tokens: 500_000,
      share: 0.2,
      growth_pct: -10,
    },
  ],
  vendors: [
    { rank: 1, vendor: "deepseek", total_tokens: 1_500_000, share: 0.6, growth_pct: 25, models_count: 2, top_model: "deepseek-chat" },
  ],
  top_movers: [{ model_name: "deepseek-chat", vendor: "deepseek", rank_delta: 1, current_rank: 1, growth_pct: 25 }],
  top_droppers: [{ model_name: "glm-4", vendor: "zhipu", rank_delta: -1, current_rank: 2, growth_pct: -10 }],
  models_history: {
    points: [
      { ts: "2026-08-28T00:00:00Z", label: "08-28", model: "deepseek-chat", vendor: "deepseek", tokens: 800 },
      { ts: "2026-08-29T00:00:00Z", label: "08-29", model: "deepseek-chat", vendor: "deepseek", tokens: 1200 },
    ],
    models: [{ name: "deepseek-chat", vendor: "deepseek", total: 2000 }],
    buckets: 2,
  },
  vendor_share_history: {
    points: [
      { ts: "2026-08-28T00:00:00Z", label: "08-28", vendor: "deepseek", share: 0.8, tokens: 800 },
      { ts: "2026-08-29T00:00:00Z", label: "08-29", vendor: "deepseek", share: 0.9, tokens: 1200 },
    ],
    vendors: [{ name: "deepseek", total: 2000, share: 0.85 }],
    buckets: 2,
  },
};

describe("Rankings", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockPublicGet.mockResolvedValue(snapshot);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("fetches the default week period and renders the leaderboard", async () => {
    renderWithProviders(<Rankings />);

    expect((await screen.findAllByText("deepseek-chat")).length).toBeGreaterThan(0);
    expect((await screen.findAllByText("glm-4")).length).toBeGreaterThan(0);
    expect(screen.getByText("zhipu")).toBeInTheDocument();
    expect(screen.getByText("1.5M")).toBeInTheDocument();
    expect(screen.getAllByText(/↑1/).length).toBeGreaterThan(0);
    expect(mockPublicGet).toHaveBeenCalledWith("/rankings?period=week");
  });

  it("switches periods and refetches", async () => {
    const user = userEvent.setup();
    renderWithProviders(<Rankings />);

    await screen.findAllByText("deepseek-chat");
    await user.click(screen.getByRole("button", { name: "今日" }));

    await waitFor(() => {
      expect(mockPublicGet).toHaveBeenCalledWith("/rankings?period=today");
    });
  });

  it("shows the empty state when there is no usage data", async () => {
    mockPublicGet.mockResolvedValue({ ...snapshot, models: [], vendors: [] });
    renderWithProviders(<Rankings />);

    expect(await screen.findByText("暂无用量数据")).toBeInTheDocument();
  });
});
