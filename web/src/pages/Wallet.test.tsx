import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import Wallet from "./Wallet";
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
const mockApiPost = api.post as ReturnType<typeof vi.fn>;

const wallet = { balance: "100.00", frozen: "5.00", available: "95.00", currency: "CNY", total_charged: "50.00" };

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

  it("renders page title and two balance cards (no frozen funds)", async () => {
    mockApiGet.mockResolvedValueOnce(wallet);
    mockApiGet.mockResolvedValueOnce({ data: seedTxs() });

    renderWithProviders(
      <MemoryRouter initialEntries={["/wallet"]}>
        <Wallet />
      </MemoryRouter>,
    );

    expect(screen.getByRole("heading", { name: "充值" })).toBeInTheDocument();
    expect(await screen.findByText("可用余额")).toBeInTheDocument();
    expect(screen.getByText("累计消费")).toBeInTheDocument();
    expect(screen.queryByText("冻结金额")).not.toBeInTheDocument();
  });

  it("renders the bills view with only recharge records", async () => {
    mockApiGet.mockResolvedValueOnce(wallet);
    mockApiGet.mockResolvedValueOnce({ data: seedTxs() });

    renderWithProviders(
      <MemoryRouter initialEntries={["/wallet?view=bills"]}>
        <Wallet />
      </MemoryRouter>,
    );

    expect(screen.getByRole("heading", { name: "账单" })).toBeInTheDocument();
    expect(await screen.findByText("充值记录")).toBeInTheDocument();
    expect(screen.queryByText("扣费")).not.toBeInTheDocument();
  });

  it("formats wallet amounts to 2 decimals with banker's rounding", async () => {
    mockApiGet.mockResolvedValueOnce({
      ...wallet,
      available: "95.237",
      total_charged: "-0.763000",
    });
    mockApiGet.mockResolvedValueOnce({ data: seedTxs() });

    renderWithProviders(
      <MemoryRouter initialEntries={["/wallet"]}>
        <Wallet />
      </MemoryRouter>,
    );

    // The ￥ symbol is rendered inside a nested <span>, so assert the number
    // itself (the card's direct text node). 累计消费 shows the absolute value,
    // so a negative total_charged renders without the minus sign.
    expect(await screen.findByText("95.24")).toBeInTheDocument();
    expect(screen.getByText("0.76")).toBeInTheDocument();
    expect(screen.queryByText("-0.76")).not.toBeInTheDocument();
  });

  it("shows error with retry on fetch failure", async () => {
    mockApiGet.mockRejectedValue(new Error("wallet down"));

    renderWithProviders(
      <MemoryRouter initialEntries={["/wallet"]}>
        <Wallet />
      </MemoryRouter>,
    );

    expect(await screen.findByText("wallet down")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });

  it("submits a payment", async () => {
    const user = userEvent.setup();
    mockApiGet.mockResolvedValueOnce(wallet);
    mockApiGet.mockResolvedValueOnce({ data: seedTxs() });
    mockApiPost.mockResolvedValue({ data: { balance_after: "145.00" } });

    renderWithProviders(
      <MemoryRouter initialEntries={["/wallet"]}>
        <Wallet />
      </MemoryRouter>,
    );

    await waitFor(() => expect(screen.getByText("50 ￥")).toBeInTheDocument());
    await user.click(screen.getByText("50 ￥"));
    await user.click(screen.getByRole("button", { name: "充值" }));

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith("/wallet/topup", {
        amount: "50",
        payment_method: "alipay",
      });
    });
  });

  it("offers alipay and wechat payment methods with alipay selected by default", async () => {
    mockApiGet.mockResolvedValueOnce(wallet);
    mockApiGet.mockResolvedValueOnce({ data: seedTxs() });

    renderWithProviders(
      <MemoryRouter initialEntries={["/wallet"]}>
        <Wallet />
      </MemoryRouter>,
    );

    expect(await screen.findByText("支付宝")).toBeInTheDocument();
    expect(screen.getByText("微信支付")).toBeInTheDocument();
    expect((screen.getByLabelText(/支付宝/) as HTMLInputElement).checked).toBe(true);
    expect((screen.getByLabelText(/微信支付/) as HTMLInputElement).checked).toBe(false);
  });
});
