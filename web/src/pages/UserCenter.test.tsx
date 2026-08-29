import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
const mockApiDelete = api.delete as ReturnType<typeof vi.fn>;

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
    mockApiDelete.mockResolvedValue({ ok: true });
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

  it("lists sessions and revokes a non-current one", async () => {
    mockApiGet.mockImplementation((path: string) => {
      if (path === "/profile") return Promise.resolve(profile);
      if (path === "/security/login-history") return Promise.resolve({ data: [] });
      if (path === "/sessions") {
        return Promise.resolve({
          data: [
            { id: "s1", ip_address: "10.0.0.1", user_agent: "Chrome/126", created_at: "2026-08-01T00:00:00Z", expires_at: "2026-08-02T00:00:00Z", current: true },
            { id: "s2", ip_address: "10.0.0.2", user_agent: "Firefox/125", created_at: "2026-08-01T01:00:00Z", expires_at: "2026-08-02T01:00:00Z", current: false },
          ],
        });
      }
      return Promise.resolve({ data: [] });
    });
    const user = userEvent.setup();
    renderUserCenter();

    await user.click(await screen.findByRole("tab", { name: "登录记录" }));
    expect(await screen.findByText("登录会话")).toBeInTheDocument();
    expect(screen.getByText("当前会话")).toBeInTheDocument();
    expect(screen.getByText("Chrome/126")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "撤销" }));
    await waitFor(() => {
      expect(mockApiDelete).toHaveBeenCalledWith("/sessions/s2");
    });
  });
});
