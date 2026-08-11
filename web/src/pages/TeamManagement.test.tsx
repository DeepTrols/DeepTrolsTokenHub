import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import TeamManagement from "./TeamManagement";
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

const baseMembers = [
  { id: "owner-id", name: "老板", email: "owner@company.com", role: "owner", status: "active" },
  { id: "bob-id", name: "Bob", email: "bob@company.com", role: "admin", status: "active" },
  { id: "alice-id", name: "Alice", email: "alice@company.com", role: "member", status: "active" },
];

const basePools = [
  {
    id: "pool-1",
    dimension: "token",
    total_amount: 10000,
    allocated: 3000,
    used: 0,
    remaining: 7000,
    unit_name: "token",
    allocations: [
      { user_id: "bob-id", allocated: 2000, used: 800, remaining: 1200 },
      { user_id: "alice-id", allocated: 1000, used: 200, remaining: 800 },
    ],
  },
  {
    id: "pool-2",
    dimension: "token",
    total_amount: 5000,
    allocated: 1000,
    used: 0,
    remaining: 4000,
    unit_name: "token",
    allocations: [
      { user_id: "alice-id", allocated: 1000, used: 300, remaining: 700 },
    ],
  },
];

type Member = typeof baseMembers[number];
type Pool = typeof basePools[number];

/** Routes GET /team and GET /team/quotas to the given fixtures. */
function mockRoutes(options: { members?: Member[]; pools?: Pool[] } = {}) {
  const members = options.members ?? baseMembers;
  const pools = options.pools ?? basePools;
  mockApiGet.mockImplementation((path: string) => {
    if (path === "/team/quotas") return Promise.resolve({ pools });
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

  it("fetches members and quotas on mount", async () => {
    renderWithProviders(<TeamManagement />);

    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledWith("/team");
      expect(mockApiGet).toHaveBeenCalledWith("/team/quotas");
    });
  });

  it("shows loading state before data arrives", async () => {
    let resolveGet!: (v: unknown) => void;
    const pending = new Promise((r) => {
      resolveGet = r;
    });
    mockApiGet.mockImplementation(() => pending);

    renderWithProviders(<TeamManagement />);

    expect(screen.getByText("加载团队成员...")).toBeInTheDocument();

    resolveGet({ members: [] });
    expect(await screen.findByText("暂无团队成员")).toBeInTheDocument();
  });

  it("renders members with role and status badges", async () => {
    renderWithProviders(<TeamManagement />);

    expect(await screen.findByText("老板")).toBeInTheDocument();
    expect(screen.getByText("bob@company.com")).toBeInTheDocument();
    expect(screen.getByText("alice@company.com")).toBeInTheDocument();
    expect(screen.getByText("拥有者")).toBeInTheDocument();
    expect(screen.getByText("管理员")).toBeInTheDocument();
    expect(screen.getAllByText("正常").length).toBeGreaterThanOrEqual(3);
  });

  it("shows empty state when no members exist", async () => {
    mockRoutes({ members: [], pools: [] });

    renderWithProviders(<TeamManagement />);

    expect(await screen.findByText("暂无团队成员")).toBeInTheDocument();
  });

  it("shows the owner row as non-operable", async () => {
    renderWithProviders(<TeamManagement />);

    await screen.findByText("老板");
    expect(screen.getByText("所有者不可操作")).toBeInTheDocument();

    const ownerRow = screen.getByRole("row", { name: /owner@company\.com/ });
    expect(within(ownerRow).queryByRole("button", { name: /分配额度/ })).not.toBeInTheDocument();
    expect(within(ownerRow).queryByRole("button", { name: /移除成员/ })).not.toBeInTheDocument();
  });

  it("aggregates each member's quota across pools in the list column", async () => {
    renderWithProviders(<TeamManagement />);

    await screen.findByText("Bob");

    // Bob: 2000/800, Alice: 1000+1000=2000 / 200+300=500
    expect(screen.getAllByText("已分配 2,000").length).toBe(2);
    expect(screen.getByText("已用 800")).toBeInTheDocument();
    expect(screen.getByText("已用 500")).toBeInTheDocument();
  });

  it("shows the quota stat strip (pools count / headroom / allocated total)", async () => {
    renderWithProviders(<TeamManagement />);

    await screen.findByText("Bob");

    const headroomLabel = screen.getByText("企业剩余可分配");
    expect(headroomLabel).toBeInTheDocument();
    expect(screen.getByText("11,000")).toBeInTheDocument(); // 7000 + 4000
    expect(screen.getByText("4,000")).toBeInTheDocument(); // allocated total
    expect(screen.getByText("配额池数量")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument(); // pool count
  });

  it("creates a sub-account with role member", async () => {
    const user = userEvent.setup();
    mockApiPost.mockResolvedValue({
      id: "new-id",
      email: "colleague@company.com",
      display_name: "张三",
      role: "member",
    });

    renderWithProviders(<TeamManagement />);
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

    renderWithProviders(<TeamManagement />);
    await screen.findByText("Bob");

    await user.click(screen.getByRole("button", { name: /添加子账号/ }));
    await user.type(screen.getByPlaceholderText("colleague@company.com"), "colleague@company.com");
    await user.type(screen.getByPlaceholderText("张三"), "张三");
    await user.type(screen.getByPlaceholderText("至少 8 位"), "123");

    expect(screen.getByText("密码至少 8 位")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /创建子账号/ })).toBeDisabled();
    expect(mockApiPost).not.toHaveBeenCalled();
  });

  it("allocates quota to a member (additive amount within pool headroom)", async () => {
    const user = userEvent.setup();
    mockApiPost.mockResolvedValue({
      id: "alloc-id",
      pool_id: "pool-1",
      user_id: "bob-id",
      allocated: 2500,
      used: 800,
      remaining: 4500,
    });

    renderWithProviders(<TeamManagement />);
    await screen.findByText("Bob");

    await user.click(within(bobRow()).getByRole("button", { name: /分配额度/ }));

    // Default pool is pool-1 with 7000 remaining (dialog opened)
    expect(screen.getByText("剩余可分配 7,000")).toBeInTheDocument();

    await user.type(screen.getByPlaceholderText("例如 1000"), "500");
    await user.click(screen.getByRole("button", { name: /确认分配/ }));

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith("/team/quotas/allocate", {
        user_id: "bob-id",
        pool_id: "pool-1",
        amount: 500,
      });
    });
  });

  it("rejects an allocation that exceeds pool headroom", async () => {
    const user = userEvent.setup();

    renderWithProviders(<TeamManagement />);
    await screen.findByText("Bob");

    await user.click(within(bobRow()).getByRole("button", { name: /分配额度/ }));
    await user.type(screen.getByPlaceholderText("例如 1000"), "999999");

    expect(screen.getByText("超出企业剩余可分配额度")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /确认分配/ })).toBeDisabled();
    expect(mockApiPost).not.toHaveBeenCalled();
  });

  it("changes a member's role (owner only)", async () => {
    const user = userEvent.setup();
    mockApiPut.mockResolvedValue({ status: "updated" });

    renderWithProviders(<TeamManagement />);
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

    renderWithProviders(<TeamManagement />);
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

    renderWithProviders(<TeamManagement />);
    await screen.findByText("Bob");

    await user.click(within(bobRow()).getByRole("button", { name: /移除成员/ }));

    await waitFor(() => {
      expect(mockApiDelete).toHaveBeenCalledWith("/team/bob-id");
    });
  });

  it("surfaces a members fetch error with a retry button", async () => {
    mockApiGet.mockImplementation((path: string) => {
      if (path === "/team/quotas") return Promise.resolve({ pools: [] });
      return Promise.reject(new Error("Network error"));
    });

    renderWithProviders(<TeamManagement />);

    expect(await screen.findByText("Network error")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });
});
