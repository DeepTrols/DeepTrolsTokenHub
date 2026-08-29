import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Subscriptions from "./Subscriptions";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  publicApi: { get: vi.fn() },
}));

import { api } from "../lib/api";
const mockApiGet = api.get as ReturnType<typeof vi.fn>;
const mockApiPost = api.post as ReturnType<typeof vi.fn>;

describe("Subscriptions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApiGet.mockImplementation((path: string) => {
      if (path === "/subscription/plans") {
        return Promise.resolve({
          data: [
            { id: "p1", name: "Pro 月付", description: "月度套餐", price: "10", duration_days: 30, sort_order: 1 },
            { id: "p2", name: "年付", description: "", price: "100", duration_days: 365, sort_order: 0 },
          ],
        });
      }
      if (path === "/subscription/self") {
        return Promise.resolve({
          subscriptions: [],
          all_subscriptions: [],
        });
      }
      if (path === "/wallet") return Promise.resolve({ balance: "500" });
      return Promise.resolve(null);
    });
    mockApiPost.mockResolvedValue({ ok: true, plan_name: "Pro 月付", price: "10", expires_at: "2026-09-28T00:00:00Z" });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders plan cards and the wallet balance", async () => {
    renderWithProviders(<Subscriptions />);

    expect(await screen.findByText("Pro 月付")).toBeInTheDocument();
    expect(screen.getByText("年付")).toBeInTheDocument();
    expect(screen.getByText("¥10")).toBeInTheDocument();
    expect(screen.getByText("1 个月")).toBeInTheDocument();
    expect(mockApiGet).toHaveBeenCalledWith("/wallet");
  });

  it("purchases a plan after confirmation", async () => {
    const user = userEvent.setup();
    renderWithProviders(<Subscriptions />);

    const buttons = await screen.findAllByRole("button", { name: /立即开通/ });
    await user.click(buttons[0]);
    await screen.findByRole("dialog");
    await user.click(screen.getByRole("button", { name: /确认支付/ }));

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith("/subscription/purchase", expect.objectContaining({ plan_id: "p1" }));
    });
  });

  it("shows the active subscription banner and history", async () => {
    mockApiGet.mockImplementation((path: string) => {
      if (path === "/subscription/plans") {
        return Promise.resolve({ data: [{ id: "p1", name: "Pro 月付", description: "", price: "10", duration_days: 30, sort_order: 1 }] });
      }
      if (path === "/subscription/self") {
        return Promise.resolve({
          subscriptions: [{ id: "s1", plan_id: "p1", plan_name: "Pro 月付", price: "10", starts_at: "2026-08-01T00:00:00Z", expires_at: "2026-09-01T00:00:00Z", status: "active" }],
          all_subscriptions: [{ id: "s1", plan_id: "p1", plan_name: "Pro 月付", price: "10", starts_at: "2026-08-01T00:00:00Z", expires_at: "2026-09-01T00:00:00Z", status: "active" }],
        });
      }
      if (path === "/wallet") return Promise.resolve({ balance: "500" });
      return Promise.resolve(null);
    });

    renderWithProviders(<Subscriptions />);

    expect(await screen.findByText(/当前套餐：Pro 月付/)).toBeInTheDocument();
    expect(screen.getByText("订阅记录")).toBeInTheDocument();
    expect(screen.getAllByText("生效中").length).toBeGreaterThan(0);
  });
});
