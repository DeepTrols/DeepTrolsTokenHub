import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen } from "@testing-library/react";
import Finance from "./Finance";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

vi.mock("../lib/auth", () => ({
  useAuth: () => ({
    user: { id: "admin", email: "admin@test.com", name: "Admin", role: "admin", status: "active" },
    isLoading: false,
    isAuthenticated: true,
    logout: vi.fn(),
  }),
}));

import { adminApi } from "../lib/api";
const mockAdminGet = adminApi.get as ReturnType<typeof vi.fn>;

function seedLedger() {
  return [
    {
      id: "u1",
      email: "alice@example.com",
      display_name: "Alice",
      role: "user",
      status: "active",
      user_type: "personal",
      tenant_id: "",
      tenant_name: "",
      balance: "100.00",
      frozen: "0.00",
      total_topup: "200.00",
      total_spend: "50.00",
      request_count: 10,
      total_tokens: 1234,
      top_models: ["gpt-4o", "claude-3.5-sonnet"],
    },
    {
      id: "u2",
      email: "bob@corp.com",
      display_name: "Bob Corp",
      role: "admin",
      status: "active",
      user_type: "enterprise",
      tenant_id: "t1",
      tenant_name: "Tenant B",
      balance: "50.00",
      frozen: "0.00",
      total_topup: "100.00",
      total_spend: "30.00",
      request_count: 5,
      total_tokens: 678,
      top_models: ["gpt-4o"],
    },
  ];
}

describe("Finance（账务管理）", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders page title", async () => {
    mockAdminGet.mockResolvedValue({ data: seedLedger(), total: 2 });

    renderWithProviders(<Finance />);

    expect(screen.getByText("账务管理")).toBeInTheDocument();
  });

  it("shows loading spinner while fetching", () => {
    mockAdminGet.mockImplementation(() => new Promise(() => {}));

    renderWithProviders(<Finance />);

    expect(screen.getByText("加载账务数据...")).toBeInTheDocument();
  });

  it("shows aggregate summary cards", async () => {
    mockAdminGet.mockResolvedValue({ data: seedLedger(), total: 2 });

    renderWithProviders(<Finance />);

    expect(await screen.findByText("150.00 CNY")).toBeInTheDocument(); // 总余额
    expect(screen.getByText("300.00 CNY")).toBeInTheDocument(); // 累计充值
    expect(screen.getByText("80.00 CNY")).toBeInTheDocument(); // 累计消费
    expect(screen.getByText("15")).toBeInTheDocument(); // 总调用量
  });

  it("lists accounts with type and tenant", async () => {
    mockAdminGet.mockResolvedValue({ data: seedLedger(), total: 2 });

    renderWithProviders(<Finance />);

    expect(await screen.findByText("alice@example.com")).toBeInTheDocument();
    expect(screen.getByText("bob@corp.com")).toBeInTheDocument();
    expect(screen.getByText("个人")).toBeInTheDocument();
    expect(screen.getByText("企业")).toBeInTheDocument();
    expect(screen.getByText("Tenant B")).toBeInTheDocument();
  });

  it("reveals top models when a row is expanded", async () => {
    mockAdminGet.mockResolvedValue({ data: seedLedger(), total: 2 });

    renderWithProviders(<Finance />);

    const toggle = (await screen.findAllByLabelText("展开 Top 模型"))[0];
    toggle.click();

    expect(await screen.findByText("claude-3.5-sonnet")).toBeInTheDocument();
    expect(screen.getByText("gpt-4o")).toBeInTheDocument();
  });

  it("shows empty state when there is no data", async () => {
    mockAdminGet.mockResolvedValue({ data: [], total: 0 });

    renderWithProviders(<Finance />);

    expect(await screen.findByText("暂无账务数据")).toBeInTheDocument();
  });

  it("shows error with retry on failure", async () => {
    mockAdminGet.mockRejectedValue(new Error("ledger down"));

    renderWithProviders(<Finance />);

    expect(await screen.findByText("ledger down")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });
});
