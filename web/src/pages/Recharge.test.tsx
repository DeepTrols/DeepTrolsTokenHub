import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import Recharge from "./Recharge";
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
const methodsPayload = {
  enabled: true,
  payment_compliance_confirmed: true,
  pay_methods: [
    { name: "支付宝", type: "alipay", color: "#1677FF" },
    { name: "微信支付", type: "wxpay", color: "#07C160" },
  ],
  min_topup: "1.00",
  max_topup: "1000000.00",
  amount_options: ["10", "50", "100", "200", "500"],
};

function mockGet(path: string) {
  if (path === "/wallet") return Promise.resolve(wallet);
  if (path === "/payment/methods") return Promise.resolve(methodsPayload);
  if (path === "/payment/orders") return Promise.resolve({ orders: [] });
  if (path === "/checkin/status") {
    return Promise.resolve({
      enabled: true,
      min_quota: "1",
      max_quota: "5",
      checked_in_today: false,
      total_days: 1,
    });
  }
  return Promise.resolve(null);
}

describe("Recharge", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApiGet.mockImplementation(mockGet);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders page title and two balance cards", async () => {
    renderWithProviders(
      <MemoryRouter initialEntries={["/recharge"]}>
        <Recharge />
      </MemoryRouter>,
    );

    expect(screen.getByRole("heading", { name: "充值" })).toBeInTheDocument();
    expect(await screen.findByText("可用余额")).toBeInTheDocument();
    expect(screen.getByText("累计消费")).toBeInTheDocument();
  });

  it("renders the online topup form", async () => {
    renderWithProviders(
      <MemoryRouter initialEntries={["/recharge"]}>
        <Recharge />
      </MemoryRouter>,
    );

    expect(await screen.findByText("在线支付充值")).toBeInTheDocument();
  });

  it("offers alipay and wechat payment methods with alipay selected by default", async () => {
    renderWithProviders(
      <MemoryRouter initialEntries={["/recharge"]}>
        <Recharge />
      </MemoryRouter>,
    );

    expect(await screen.findByText("支付宝")).toBeInTheDocument();
    expect(screen.getByText("微信支付")).toBeInTheDocument();
    expect((screen.getByLabelText(/支付宝/) as HTMLInputElement).checked).toBe(true);
    expect((screen.getByLabelText(/微信支付/) as HTMLInputElement).checked).toBe(false);
  });

  it("shows error with retry on wallet fetch failure", async () => {
    mockApiGet.mockImplementation(() => Promise.reject(new Error("wallet down")));
    renderWithProviders(
      <MemoryRouter initialEntries={["/recharge"]}>
        <Recharge />
      </MemoryRouter>,
    );

    expect(await screen.findByText("wallet down")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });

  it("creates a real payment order and opens the QR dialog", async () => {
    const user = userEvent.setup();
    mockApiPost.mockResolvedValue({
      order_no: "DTP1",
      amount: "50.00",
      currency: "CNY",
      channel: "epay",
      pay_method: "alipay",
      pay_url: "https://pay.example.com/submit.php",
    });

    renderWithProviders(
      <MemoryRouter initialEntries={["/recharge"]}>
        <Recharge />
      </MemoryRouter>,
    );

    await waitFor(() => expect(screen.getByText("50 ￥")).toBeInTheDocument());
    await user.click(screen.getByText("50 ￥"));
    await user.click(screen.getByRole("button", { name: "充值" }));

    await waitFor(() =>
      expect(mockApiPost).toHaveBeenCalledWith("/payment/order", { amount: "50", pay_method: "alipay" }),
    );
    expect(await screen.findByText(/订单号 DTP1/)).toBeInTheDocument();
  });
});
