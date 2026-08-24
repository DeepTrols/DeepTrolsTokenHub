import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TeamManagementContent } from "./TeamManagement";
import { renderWithProviders } from "../test/test-utils";

// Mock the api module
vi.mock("../lib/api", () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  },
}));

// Mock auth - TeamManagement is wrapped by RequireAuth tenantAdminOnly
vi.mock("../lib/auth", () => ({
  useAuth: () => ({
    user: {
      id: "owner-id",
      email: "owner@company.com",
      name: "老板",
      role: "user",
      tenant_role: "owner",
      status: "active",
    },
    isLoading: false,
    isAuthenticated: true,
    logout: vi.fn(),
  }),
}));

import { api } from "../lib/api";

const mockApiGet = api.get as ReturnType<typeof vi.fn>;
const mockApiPost = api.post as ReturnType<typeof vi.fn>;
const mockApiPut = api.put as ReturnType<typeof vi.fn>;
const mockApiDelete = api.delete as ReturnType<typeof vi.fn>;

// Balances are decimal strings — the API boundary never uses floats for money.
const baseMembers = [
  { id: "owner-id", name: "老板", email: "owner@company.com", role: "owner", status: "active", balance: "0" },
  { id: "bob-id", name: "Bob", email: "bob@company.com", role: "admin", status: "active", balance: "120.5" },
  { id: "alice-id", name: "Alice", email: "alice@company.com", role: "member", status: "active", balance: "0" },
];

// The admin's own wallet, read for the "available" ceiling in the dialog.
const walletBalance = {
  balance: "10000",
  frozen: "0",
  available: "10000",
  currency: "CNY",
  total_charged: "0",
};

type Member = typeof baseMembers[number];

/** Routes GET /team and GET /wallet to the given fixtures. */
function mockRoutes(options: { members?: Member[] } = {}) {
  const members = options.members ?? baseMembers;
  mockApiGet.mockImplementation((path: string) => {
    if (path === "/wallet") return Promise.resolve(walletBalance);
    return Promise.resolve({ members });
  });
}

function bobRow() {
  return screen.getByRole("row", { name: /bob@company\.com/ });
}

describe("TeamManagement", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(window, "confirm").mockReturnValue(true);
    mockRoutes();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("fetches members and the admin wallet on mount", async () => {
    renderWithProviders(<TeamManagementContent />);

    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledWith("/team");
      expect(mockApiGet).toHaveBeenCalledWith("/wallet");
    });
  });

  it("shows loading state before data arrives", async () => {
    let resolveGet!: (v: unknown) => void;
    const pending = new Promise((r) => {
      resolveGet = r;
    });
    mockApiGet.mockImplementation(() => pending);

    renderWithProviders(<TeamManagementContent />);

    expect(screen.getByText("加载团队成员...")).toBeInTheDocument();

    resolveGet({ members: [] });
    expect(await screen.findByText("暂无团队成员")).toBeInTheDocument();
  });

  it("renders members with role and status badges", async () => {
    renderWithProviders(<TeamManagementContent />);

    expect(await screen.findByText("老板")).toBeInTheDocument();
    expect(screen.getByText("bob@company.com")).toBeInTheDocument();
    expect(screen.getByText("alice@company.com")).toBeInTheDocument();
    expect(screen.getByText("拥有者")).toBeInTheDocument();
    expect(screen.getByText("管理员")).toBeInTheDocument();
    expect(screen.getAllByText("正常").length).toBeGreaterThanOrEqual(3);
  });

  it("shows empty state when no members exist", async () => {
    mockRoutes({ members: [] });

    renderWithProviders(<TeamManagementContent />);

    expect(await screen.findByText("暂无团队成员")).toBeInTheDocument();
  });

  it("shows the owner row as non-operable", async () => {
    renderWithProviders(<TeamManagementContent />);

    await screen.findByText("老板");
    expect(screen.getByText("所有者不可操作")).toBeInTheDocument();

    const ownerRow = screen.getByRole("row", { name: /owner@company\.com/ });
    expect(within(ownerRow).queryByRole("button", { name: /分配余额/ })).not.toBeInTheDocument();
    expect(within(ownerRow).queryByRole("button", { name: /移除成员/ })).not.toBeInTheDocument();
  });

  it("shows each member's current balance in the list column", async () => {
    renderWithProviders(<TeamManagementContent />);

    await screen.findByText("Bob");

    // Bob has 120.5; the owner and Alice have zero balances.
    expect(screen.getByText("¥120.50")).toBeInTheDocument();
    expect(screen.getAllByText("¥0.00").length).toBe(2);
  });

  it("creates a sub-account with role member", async () => {
    const user = userEvent.setup();
    mockApiPost.mockResolvedValue({
      id: "new-id",
      email: "colleague@company.com",
      display_name: "张三",
      role: "member",
    });

    renderWithProviders(<TeamManagementContent />);
    await screen.findByText("Bob");

    await user.click(screen.getByRole("button", { name: /添加子账号/ }));

    expect(screen.getAllByText("添加子账号").length).toBeGreaterThanOrEqual(2);
    await user.type(screen.getByPlaceholderText("colleague@company.com"), "colleague@company.com");
    await user.type(screen.getByPlaceholderText("张三"), "张三");
    await user.type(screen.getByPlaceholderText("至少 8 位"), "password123");
    await user.click(screen.getByRole("button", { name: /创建子账号/ }));

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith("/team/members", {
        email: "colleague@company.com",
        display_name: "张三",
        password: "password123",
        role: "member",
      });
    });
  });

  it("blocks creating a sub-account with a short password", async () => {
    const user = userEvent.setup();

    renderWithProviders(<TeamManagementContent />);
    await screen.findByText("Bob");

    await user.click(screen.getByRole("button", { name: /添加子账号/ }));
    await user.type(screen.getByPlaceholderText("colleague@company.com"), "colleague@company.com");
    await user.type(screen.getByPlaceholderText("张三"), "张三");
    await user.type(screen.getByPlaceholderText("至少 8 位"), "123");

    expect(screen.getByText("密码至少 8 位")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /创建子账号/ })).toBeDisabled();
    expect(mockApiPost).not.toHaveBeenCalled();
  });

  it("allocates balance to a member (amount within the admin's available balance)", async () => {
    const user = userEvent.setup();
    mockApiPost.mockResolvedValue({
      transaction_id: "tx-1",
      from_balance: "9500",
      to_balance: "10",
    });

    renderWithProviders(<TeamManagementContent />);
    await screen.findByText("Bob");

    await user.click(within(bobRow()).getByRole("button", { name: /分配余额/ }));

    // The dialog surfaces the admin's own spendable balance as the ceiling.
    expect(screen.getByText("¥10,000.00")).toBeInTheDocument();

    await user.type(screen.getByPlaceholderText("例如 10.00"), "10");
    await user.click(screen.getByRole("button", { name: /确认分配/ }));

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith("/team/balance/allocate", {
        user_id: "bob-id",
        amount: "10",
      });
    });
  });

  it("rejects an allocation that exceeds the admin's available balance", async () => {
    const user = userEvent.setup();

    renderWithProviders(<TeamManagementContent />);
    await screen.findByText("Bob");

    await user.click(within(bobRow()).getByRole("button", { name: /分配余额/ }));
    await user.type(screen.getByPlaceholderText("例如 10.00"), "99999");

    expect(screen.getByText("超出您的可用余额")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /确认分配/ })).toBeDisabled();
    expect(mockApiPost).not.toHaveBeenCalled();
  });

  it("disables allocation and surfaces a retry when the admin wallet fails to load", async () => {
    const user = userEvent.setup();
    mockApiGet.mockImplementation((path: string) => {
      if (path === "/wallet") return Promise.reject(new Error("wallet down"));
      return Promise.resolve({ members: baseMembers });
    });

    renderWithProviders(<TeamManagementContent />);
    await screen.findByText("Bob");

    await user.click(within(bobRow()).getByRole("button", { name: /分配余额/ }));

    expect(await screen.findByText("可用余额加载失败")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /确认分配/ })).toBeDisabled();
    expect(mockApiPost).not.toHaveBeenCalled();
  });

  it("changes a member's role (owner only)", async () => {
    const user = userEvent.setup();
    mockApiPut.mockResolvedValue({ status: "updated" });

    renderWithProviders(<TeamManagementContent />);
    await screen.findByText("Bob");

    const aliceRow = screen.getByRole("row", { name: /alice@company\.com/ });
    await user.click(within(aliceRow).getByRole("button", { name: /改角色/ }));

    expect(screen.getByText("修改角色")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "管理员" }));
    await user.click(screen.getByRole("button", { name: /保存/ }));

    await waitFor(() => {
      expect(mockApiPut).toHaveBeenCalledWith("/team/alice-id/role", {
        id: "alice-id",
        role: "admin",
      });
    });
  });

  it("suspends a member via the status toggle", async () => {
    const user = userEvent.setup();
    mockApiPut.mockResolvedValue({ status: "updated" });

    renderWithProviders(<TeamManagementContent />);
    await screen.findByText("Bob");

    await user.click(within(bobRow()).getByRole("button", { name: /停用/ }));

    await waitFor(() => {
      expect(mockApiPut).toHaveBeenCalledWith("/team/bob-id/status", {
        id: "bob-id",
        status: "suspended",
      });
    });
  });

  it("removes a member via the trash button", async () => {
    const user = userEvent.setup();
    mockApiDelete.mockResolvedValue({ status: "deleted" });

    renderWithProviders(<TeamManagementContent />);
    await screen.findByText("Bob");

    await user.click(within(bobRow()).getByRole("button", { name: /移除成员/ }));

    await waitFor(() => {
      expect(mockApiDelete).toHaveBeenCalledWith("/team/bob-id");
    });
  });

  it("surfaces a members fetch error with a retry button", async () => {
    mockApiGet.mockImplementation((path: string) => {
      if (path === "/wallet") return Promise.resolve(walletBalance);
      return Promise.reject(new Error("Network error"));
    });

    renderWithProviders(<TeamManagementContent />);

    expect(await screen.findByText("Network error")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });
});

