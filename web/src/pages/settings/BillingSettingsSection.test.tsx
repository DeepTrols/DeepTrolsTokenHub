import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import BillingSettingsSection from "./BillingSettingsSection";
import { renderWithProviders } from "../../test/test-utils";

vi.mock("../../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  publicApi: { get: vi.fn() },
}));

import { adminApi } from "../../lib/api";
const mockAdminGet = adminApi.get as ReturnType<typeof vi.fn>;
const mockAdminPut = adminApi.put as ReturnType<typeof vi.fn>;

describe("BillingSettingsSection 用户分组", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAdminGet.mockImplementation((path: string) => {
      if (path.startsWith("/payment/orders")) return Promise.resolve({ orders: [] });
      return Promise.resolve({
        payment_enabled: "true",
        user_groups: '[{"name":"vip","ratio":"0.8"}]',
        discount_tiers: '[{"min_tokens":1000000,"ratio":"0.95"}]',
      });
    });
    mockAdminPut.mockResolvedValue({ ok: true });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("adds a group and saves the user_groups JSON", async () => {
    const user = userEvent.setup();
    renderWithProviders(<BillingSettingsSection />);

    await user.click(await screen.findByRole("tab", { name: "分组" }));
    // Hydrated from settings: one group vip / 0.8.
    expect(screen.getAllByLabelText("分组名称")[0]).toHaveValue("vip");
    expect(screen.getAllByLabelText("倍率")[0]).toHaveValue(0.8);

    await user.click(screen.getByRole("button", { name: "添加分组" }));
    const names = screen.getAllByLabelText("分组名称");
    const ratios = screen.getAllByLabelText("倍率");
    await user.type(names[1], "enterprise");
    await user.clear(ratios[1]);
    await user.type(ratios[1], "0.6");
    await user.click(screen.getByRole("button", { name: "保存" }));

    await waitFor(() => {
      expect(mockAdminPut).toHaveBeenCalledWith("/settings/site", {
        user_groups: '[{"name":"vip","ratio":"0.8"},{"name":"enterprise","ratio":"0.6"}]',
      });
    });
  });

  it("adds a discount tier and saves the discount_tiers JSON", async () => {
    const user = userEvent.setup();
    renderWithProviders(<BillingSettingsSection />);

    await user.click(await screen.findByRole("tab", { name: "折扣" }));
    expect(screen.getAllByLabelText("最低 tokens")[0]).toHaveValue(1000000);
    expect(screen.getAllByLabelText("折扣率 (0-1)")[0]).toHaveValue(0.95);

    await user.click(screen.getByRole("button", { name: "添加档位" }));
    const tokens = screen.getAllByLabelText("最低 tokens");
    const ratios = screen.getAllByLabelText("折扣率 (0-1)");
    expect(tokens.length).toBe(2);
    await user.clear(tokens[1]);
    await user.type(tokens[1], "5000000");
    await user.clear(ratios[1]);
    await user.type(ratios[1], "0.9");
    await user.click(screen.getByRole("button", { name: "保存" }));

    await waitFor(() => {
      expect(mockAdminPut).toHaveBeenCalledWith("/settings/site", {
        discount_tiers:
          '[{"min_tokens":1000000,"ratio":"0.95"},{"min_tokens":5000000,"ratio":"0.9"}]',
      });
    });
  });
});
