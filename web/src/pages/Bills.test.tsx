import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen } from "@testing-library/react";
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

function seedTxs() {
  return [
    { id: "tx-1", type: "topup", amount: "+50.00", balance_after: "150.00", reference: "余额充值", created_at: "2026-08-01T00:00:00Z" },
    { id: "tx-2", type: "charge", amount: "-0.50", balance_after: "149.50", reference: "模型调用扣费", created_at: "2026-08-01T00:01:00Z" },
  ];
}

describe("Bills", () => {
  beforeEach(() => {
    vi.clearAllMocks();
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
});
