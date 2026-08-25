import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import UserCenter from "./UserCenter";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

vi.mock("../lib/auth", () => ({
  useAuth: () => ({
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

const wallet = { balance: "100", frozen: "0", available: "100", currency: "CNY", total_charged: "0" };

function mockRoutes() {
  mockApiGet.mockImplementation((path: string) => {
    if (path === "/profile") return Promise.resolve(profile);
    if (path === "/security/login-history") return Promise.resolve({ data: [] });
    if (path === "/wallet") return Promise.resolve(wallet);
    return Promise.resolve({ data: [] });
  });
}

function renderUserCenter() {
  return renderWithProviders(
    <MemoryRouter initialEntries={["/account"]}>
      <UserCenter />
    </MemoryRouter>,
  );
}

describe("UserCenter", () => {
  beforeEach(() => {
    vi.clearAllMocks();
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

  it("renders the account profile content", async () => {
    renderUserCenter();

    expect(await screen.findByText("基本信息")).toBeInTheDocument();
  });
});
