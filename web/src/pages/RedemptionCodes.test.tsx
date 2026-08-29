import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import RedemptionCodes from "./RedemptionCodes";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { adminApi } from "../lib/api";
const mockAdminGet = adminApi.get as ReturnType<typeof vi.fn>;
const mockAdminPost = adminApi.post as ReturnType<typeof vi.fn>;

const codes = {
  codes: [
    {
      code: "DTP-aaaabbbb1111",
      amount: "10",
      status: "active",
      created_at: "2026-08-29 14:00:00",
    },
    {
      code: "DTP-ccccdddd2222",
      amount: "50",
      status: "used",
      created_at: "2026-08-29 13:00:00",
      used_at: "2026-08-29 13:30:00",
      used_by_email: "user@example.com",
    },
  ],
};

describe("RedemptionCodes", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAdminGet.mockResolvedValue(codes);
    mockAdminPost.mockResolvedValue({ created: 2, codes: ["DTP-new1", "DTP-new2"] });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders the code table with status labels", async () => {
    renderWithProviders(<RedemptionCodes />);

    expect(await screen.findByText("DTP-aaaabbbb1111")).toBeInTheDocument();
    expect(screen.getByText("DTP-ccccdddd2222")).toBeInTheDocument();
    expect(screen.getByText("未使用")).toBeInTheDocument();
    expect(screen.getByText("已使用")).toBeInTheDocument();
    expect(screen.getByText("user@example.com")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /复制未使用 \(1\)/ })).toBeEnabled();
  });

  it("creates codes via the dialog and posts amount + count", async () => {
    const user = userEvent.setup();
    renderWithProviders(<RedemptionCodes />);

    await screen.findByText("DTP-aaaabbbb1111");
    await user.click(screen.getByRole("button", { name: /生成兑换码/ }));
    await screen.findByRole("dialog");

    const amountInput = screen.getByLabelText("面值 (元)");
    const countInput = screen.getByLabelText("数量");
    await user.clear(amountInput);
    await user.type(amountInput, "20");
    await user.clear(countInput);
    await user.type(countInput, "3");
    await user.click(screen.getByRole("button", { name: "生成" }));

    await waitFor(() => {
      expect(mockAdminPost).toHaveBeenCalledWith("/redemption", { amount: "20", count: 3 });
    });
    // The mutation invalidates /redemption so the list refetches.
    await waitFor(() => {
      expect(mockAdminGet).toHaveBeenCalledWith("/redemption");
    });
  });

  it("shows empty state when no codes exist", async () => {
    mockAdminGet.mockResolvedValue({ codes: [] });

    renderWithProviders(<RedemptionCodes />);

    expect(await screen.findByText("暂无兑换码")).toBeInTheDocument();
  });
});
