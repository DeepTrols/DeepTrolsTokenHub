import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import UserCenter from "./UserCenter";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

const auth = vi.hoisted(() => ({
  user: {
    id: "u1",
    email: "user@example.com",
    name: "用户",
    role: "user",
    status: "active",
    user_type: "personal",
    phone: "",
    avatar_url: "",
    tenant_id: "",
    tenant_name: "",
    tenant_role: "",
  },
}));

vi.mock("../lib/auth", () => ({
  useAuth: () => ({
    user: auth.user,
    isLoading: false,
    isAuthenticated: true,
    logout: vi.fn(),
  }),
}));

import { api } from "../lib/api";
const mockApiGet = api.get as ReturnType<typeof vi.fn>;

const profile = {
  user: {
    id: "u1",
    email: "user@example.com",
    name: "用户",
    role: "user",
    status: "active",
    user_type: "personal",
    phone: "",
    avatar_url: "",
    tenant_id: "",
    tenant_name: "",
    tenant_role: "",
  },
  enterprise: null,
};

const members = [
  { id: "owner-id", name: "老板", email: "owner@company.com", role: "owner", status: "active", balance: "0" },
  { id: "bob-id", name: "Bob", email: "bob@company.com", role: "member", status: "active", balance: "10" },
];

const wallet = { balance: "100", frozen: "0", available: "100", currency: "CNY", total_charged: "0" };

function mockRoutes() {
  mockApiGet.mockImplementation((path: string) => {
    if (path === "/profile") return Promise.resolve(profile);
    if (path === "/security/login-history") return Promise.resolve({ data: [] });
    if (path === "/wallet") return Promise.resolve(wallet);
    return Promise.resolve({ members });
  });
}

function renderUserCenter(initialEntries: string[] = ["/account"]) {
  return renderWithProviders(
    <MemoryRouter initialEntries={initialEntries}>
      <UserCenter />
    </MemoryRouter>,
  );
}

describe("UserCenter", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    auth.user.user_type = "personal";
    auth.user.tenant_role = "";
    mockRoutes();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders the page title and description", async () => {
    renderUserCenter();

    expect(screen.getByRole("heading", { name: "用户中心" })).toBeInTheDocument();
    expect(screen.getByText(/整合账户资料与账号体系管理/)).toBeInTheDocument();
    expect(await screen.findByText("个人信息")).toBeInTheDocument();
  });

  it("shows profile content by default and hides the team tab for a personal user", async () => {
    renderUserCenter();

    expect(screen.getByRole("tab", { name: "账户资料" })).toBeInTheDocument();
    expect(await screen.findByText("基本信息")).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "团队管理" })).not.toBeInTheDocument();
  });

  it("shows the team tab only for enterprise admins and switches to it", async () => {
    const user = userEvent.setup();
    auth.user.user_type = "enterprise";
    auth.user.tenant_role = "owner";
    renderUserCenter();

    expect(await screen.findByText("基本信息")).toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "团队管理" }));

    expect(await screen.findByText("添加子账号")).toBeInTheDocument();
    expect(screen.getByText("bob@company.com")).toBeInTheDocument();
  });

  it("hides the team tab for enterprise members", async () => {
    auth.user.user_type = "enterprise";
    auth.user.tenant_role = "member";
    renderUserCenter();

    expect(await screen.findByText("基本信息")).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "团队管理" })).not.toBeInTheDocument();
  });

  it("opens the team tab directly via ?tab=team", async () => {
    auth.user.user_type = "enterprise";
    auth.user.tenant_role = "admin";
    renderUserCenter(["/account?tab=team"]);

    expect(await screen.findByText("添加子账号")).toBeInTheDocument();
    expect(screen.getByText("bob@company.com")).toBeInTheDocument();
  });
});
