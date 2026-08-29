import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import AdminSubscriptionPlans from "./AdminSubscriptionPlans";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  publicApi: { get: vi.fn() },
}));

import { adminApi } from "../lib/api";
const mockAdminGet = adminApi.get as ReturnType<typeof vi.fn>;
const mockAdminPost = adminApi.post as ReturnType<typeof vi.fn>;

describe("AdminSubscriptionPlans", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAdminGet.mockResolvedValue({
      data: [
        { id: "p1", name: "Pro 月付", description: "", price: "10", duration_days: 30, sort_order: 1, enabled: true },
      ],
      total: 1,
    });
    mockAdminPost.mockResolvedValue({ ok: true });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders the plan table", async () => {
    renderWithProviders(<AdminSubscriptionPlans />);

    expect(await screen.findByText("Pro 月付")).toBeInTheDocument();
    expect(screen.getByText("¥10")).toBeInTheDocument();
    expect(screen.getByText("30 天")).toBeInTheDocument();
  });

  it("creates a plan through the dialog", async () => {
    const user = userEvent.setup();
    renderWithProviders(<AdminSubscriptionPlans />);

    await screen.findByText("Pro 月付");
    await user.click(screen.getByRole("button", { name: /新建套餐/ }));
    await screen.findByRole("dialog");
    await user.type(screen.getByLabelText("名称"), "季度套餐");
    await user.type(screen.getByLabelText("价格 (元)"), "25");
    await user.click(screen.getByRole("button", { name: "保存" }));

    await waitFor(() => {
      expect(mockAdminPost).toHaveBeenCalledWith(
        "/subscription-plans",
        expect.objectContaining({ name: "季度套餐", price: "25", duration_days: "30" }),
      );
    });
  });
});
