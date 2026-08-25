import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import ConsoleLayout from "./ConsoleLayout";
import { renderWithProviders } from "../test/test-utils";

const auth = vi.hoisted(() => ({
  user: {
    id: "user-1",
    email: "user@example.com",
    name: "User",
    role: "user",
    status: "active",
    user_type: "personal",
    phone: "",
    avatar_url: "",
    tenant_id: "tenant-1",
    tenant_name: "Acme",
    tenant_role: "owner",
    tenant_status: "active",
  },
  logout: vi.fn(),
}));

vi.mock("../lib/auth", () => ({
  useAuth: () => ({
    user: auth.user,
    isLoading: false,
    isAuthenticated: true,
    logout: auth.logout,
  }),
}));

function renderLayout() {
  return renderWithProviders(
    <MemoryRouter initialEntries={["/dashboard"]}>
      <ConsoleLayout />
    </MemoryRouter>,
  );
}

describe("ConsoleLayout 开发者导航", () => {
  beforeEach(() => {
    auth.user.role = "user";
    auth.user.tenant_role = "owner";
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("shows the developer nav for a regular user", () => {
    renderLayout();

    expect(screen.getByText("用量信息")).toBeInTheDocument();
    expect(screen.getByText("用户中心")).toBeInTheDocument();
    expect(screen.getByText("充值")).toBeInTheDocument();
    expect(screen.getByText("账单")).toBeInTheDocument();
    expect(screen.queryByText("团队管理")).not.toBeInTheDocument();
    expect(screen.queryByText("团队预算")).not.toBeInTheDocument();
    expect(screen.queryByText("管理控制台")).not.toBeInTheDocument();
  });

  it("shows 管理控制台 only for admin", () => {
    auth.user.role = "admin";
    renderLayout();

    expect(screen.getByText("管理控制台")).toBeInTheDocument();
    expect(screen.getByText("充值")).toBeInTheDocument();
  });

  it("links 充值 / 账单 / 用户中心 to their routes", () => {
    renderLayout();

    expect(screen.getByRole("link", { name: "充值" })).toHaveAttribute("href", "/recharge");
    expect(screen.getByRole("link", { name: "账单" })).toHaveAttribute("href", "/bills");
    expect(screen.getByRole("link", { name: "用户中心" })).toHaveAttribute("href", "/account");
  });
});
