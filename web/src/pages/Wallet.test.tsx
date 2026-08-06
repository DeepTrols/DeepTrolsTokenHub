import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Wallet from "./Wallet";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

vi.mock("../lib/auth", () => ({
  useAuth: () => ({
    user: { id: "test-user", email: "test@test.com", name: "Test", role: "user", status: "active", totp_enabled: false },
    isLoading: false,
    isAuthenticated: true,
    logout: vi.fn(),
  }),
}));

import { api } from "../lib/api";
const mockApiGet = api.get as ReturnType<typeof vi.fn>;
const mockApiPost = api.post as ReturnType<typeof vi.fn>;

const wallet = { balance: "100.00", frozen: "5.00", available: "95.00", currency: "CNY", total_charged: "50.00" };
const invite = { invite_code: "A1B2C3D4", total_rewards: "25.00", referral_count: 3 };

function seedTxs() {
  return [
    { id: "tx-1", type: "topup", amount: "+50.00", balance_after: "150.00", reference: "余额充值", created_at: "2026-08-01T00:00:00Z" },
    { id: "tx-2", type: "charge", amount: "-0.50", balance_after: "149.50", reference: "模型调用扣费", created_at: "2026-08-01T00:01:00Z" },
  ];
}

describe("Wallet", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders page title and three balance cards", async () => {
    mockApiGet.mockResolvedValueOnce(wallet);
    mockApiGet.mockResolvedValueOnce({ data: seedTxs() });
    mockApiGet.mockResolvedValueOnce(invite);

    renderWithProviders(<Wallet />);

    expect(screen.getByText("钱包管理")).toBeInTheDocument();
    expect(await screen.findByText("可用余额")).toBeInTheDocument();
    expect(screen.getByText("冻结金额")).toBeInTheDocument();
    expect(screen.getByText("累计消费")).toBeInTheDocument();
  });

  it("shows three tabs: 在线充值, 兑换码, 邀请返利", async () => {
    mockApiGet.mockResolvedValueOnce(wallet);
    mockApiGet.mockResolvedValueOnce({ data: seedTxs() });
    mockApiGet.mockResolvedValueOnce(invite);

    renderWithProviders(<Wallet />);

    expect(await screen.findByText("在线充值")).toBeInTheDocument();
    expect(screen.getByText("兑换码")).toBeInTheDocument();
    expect(screen.getByText("邀请返利")).toBeInTheDocument();
  });

  it("shows invite code when invite data loads", async () => {
    mockApiGet.mockResolvedValueOnce(wallet);
    mockApiGet.mockResolvedValueOnce({ data: seedTxs() });
    mockApiGet.mockResolvedValueOnce(invite);

    renderWithProviders(<Wallet />);

    const inviteTab = await screen.findByText("邀请返利");
    await userEvent.click(inviteTab);

    expect(await screen.findByText("A1B2C3D4")).toBeInTheDocument();
    expect(screen.getByText("25.00 CNY")).toBeInTheDocument();
  });

  it("shows error with retry on fetch failure", async () => {
    mockApiGet.mockRejectedValue(new Error("wallet down"));

    renderWithProviders(<Wallet />);

    expect(await screen.findByText("wallet down")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });

  it("submits a payment from the 在线充值 tab", async () => {
    const user = userEvent.setup();
    mockApiGet.mockResolvedValueOnce(wallet);
    mockApiGet.mockResolvedValueOnce({ data: seedTxs() });
    mockApiGet.mockResolvedValueOnce(invite);
    mockApiPost.mockResolvedValue({ data: { balance_after: "145.00" } });

    renderWithProviders(<Wallet />);

    await waitFor(() => expect(screen.getByText("50 CNY")).toBeInTheDocument());
    await user.click(screen.getByText("50 CNY"));
    await user.click(screen.getByRole("button", { name: "充值" }));

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith("/wallet/topup", { amount: "50" });
    });
  });

  it("redeems a code from the 兑换码 tab", async () => {
    const user = userEvent.setup();
    mockApiGet.mockResolvedValueOnce(wallet);
    mockApiGet.mockResolvedValueOnce({ data: seedTxs() });
    mockApiGet.mockResolvedValueOnce(invite);
    mockApiPost.mockResolvedValue({
      data: { amount: "10", balance_after: "105.00", message: "成功兑换 10 CNY" },
    });

    renderWithProviders(<Wallet />);

    const redeemTab = await screen.findByText("兑换码");
    await user.click(redeemTab);

    await user.type(
      screen.getByPlaceholderText("粘贴或输入管理员提供的兑换码"),
      "DEEP-TEST-CODE"
    );
    await user.click(screen.getByRole("button", { name: "兑换" }));

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith("/wallet/redeem", {
        code: "DEEP-TEST-CODE",
      });
    });
  });

  it("shows transfer rewards button disabled when no rewards", async () => {
    const user = userEvent.setup();
    mockApiGet.mockResolvedValueOnce(wallet);
    mockApiGet.mockResolvedValueOnce({ data: seedTxs() });
    mockApiGet.mockResolvedValueOnce({ invite_code: "NO_REWARD", total_rewards: "0", referral_count: 0 });

    renderWithProviders(<Wallet />);

    const inviteTab = await screen.findByText("邀请返利");
    await user.click(inviteTab);

    const btn = await screen.findByRole("button", { name: /转入余额/ });
    expect(btn).toBeDisabled();
  });
});
