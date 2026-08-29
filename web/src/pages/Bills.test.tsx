import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import Bills from "./Bills";
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
const mockApiPut = api.put as ReturnType<typeof vi.fn>;

function seedTxs() {
  return [
    { id: "tx-1", type: "topup", amount: "+50.00", balance_after: "150.00", reference: "余额充值", created_at: "2026-08-01T00:00:00Z" },
    { id: "tx-2", type: "charge", amount: "-0.50", balance_after: "149.50", reference: "模型调用扣费", created_at: "2026-08-01T00:01:00Z" },
  ];
}

describe("Bills", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApiPut.mockResolvedValue({ threshold: "50.00" });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders page title and recharge records without balance cards", async () => {
    mockApiGet.mockResolvedValueOnce({ data: seedTxs() });

    renderWithProviders(
      <MemoryRouter initialEntries={["/bills"]}>
        <Bills />
      </MemoryRouter>,
    );

    expect(screen.getByRole("heading", { name: "账单" })).toBeInTheDocument();
    expect(await screen.findByText("充值记录")).toBeInTheDocument();
    expect(screen.queryByText("可用余额")).not.toBeInTheDocument();
    expect(screen.queryByText("累计消费")).not.toBeInTheDocument();
    expect(screen.queryByText("扣费")).not.toBeInTheDocument();
    expect(screen.queryByText("在线支付充值")).not.toBeInTheDocument();
  });

  it("shows error with retry on fetch failure", async () => {
    mockApiGet.mockRejectedValue(new Error("wallet down"));

    renderWithProviders(
      <MemoryRouter initialEntries={["/bills"]}>
        <Bills />
      </MemoryRouter>,
    );

    expect(await screen.findByText("wallet down")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });

  it("saves the balance alert threshold", async () => {
    const user = userEvent.setup();
    mockApiGet
      .mockResolvedValueOnce({ data: seedTxs() })
      .mockResolvedValueOnce({ threshold: "0.00" });

    renderWithProviders(
      <MemoryRouter initialEntries={["/bills"]}>
        <Bills />
      </MemoryRouter>,
    );

    await screen.findByText("充值记录");
    const input = screen.getByLabelText("未设置");
    await user.clear(input);
    await user.type(input, "50");
    await user.click(screen.getByRole("button", { name: "保存阈值" }));

    await waitFor(() => {
      expect(mockApiPut).toHaveBeenCalledWith("/wallet/alert", { threshold: "50" });
    });
  });

  it("renders the monthly statement with per-model spend", async () => {
    mockApiGet.mockImplementation((path: string) => {
      if (path.startsWith("/wallet/transactions")) return Promise.resolve({ data: seedTxs() });
      if (path.startsWith("/wallet/alert")) return Promise.resolve({ threshold: "0.00" });
      if (path.startsWith("/billing/statement")) {
        return Promise.resolve({
          year: 2026,
          month: 8,
          total_cost: "25.00",
          total_topup: "150.00",
          charge_count: 3,
          by_model: [
            { model: "gpt-4o", cost: "20.00", count: 2 },
            { model: "claude-sonnet", cost: "5.00", count: 1 },
          ],
        });
      }
      return Promise.resolve({ data: [] });
    });

    renderWithProviders(
      <MemoryRouter initialEntries={["/bills"]}>
        <Bills />
      </MemoryRouter>,
    );

    expect(await screen.findByText("月度账单")).toBeInTheDocument();
    expect(await screen.findByText("¥25.00")).toBeInTheDocument();
    expect(screen.getByText("¥150.00")).toBeInTheDocument();
    expect(screen.getByText("gpt-4o")).toBeInTheDocument();
    expect(screen.getByText("claude-sonnet")).toBeInTheDocument();
  });
});
