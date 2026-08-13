import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import ConsoleLayout from "./ConsoleLayout";
import { renderWithProviders } from "../test/test-utils";

// A mutable auth stub so each test can exercise a different role combination
// (system admin / personal user / enterprise admin / enterprise member).
const auth = vi.hoisted(() => ({
  user: {
    id: "user-1",
    email: "user@example.com",
    name: "User",
    role: "user",
    status: "active",
    user_type: "enterprise",
    phone: "",
    avatar_url: "",
    tenant_id: "tenant-1",
    tenant_name: "Acme",
    tenant_role: "member",
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

describe("ConsoleLayout 四角色导航", () => {
  beforeEach(() => {
    auth.user.role = "user";
    auth.user.tenant_role = "member";
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("hides 钱包管理 / 团队管理 / 管理控制台 for an enterprise member", () => {
    renderLayout();

    expect(screen.getByText("数据看板")).toBeInTheDocument();
    expect(screen.queryByText("钱包管理")).not.toBeInTheDocument();
    expect(screen.queryByText("团队管理")).not.toBeInTheDocument();
    expect(screen.queryByText("管理控制台")).not.toBeInTheDocument();
  });

  it("shows 钱包管理 and 团队管理 for an enterprise admin (owner)", () => {
    auth.user.tenant_role = "owner";
    renderLayout();

    expect(screen.getByText("钱包管理")).toBeInTheDocument();
    expect(screen.getByText("团队管理")).toBeInTheDocument();
    expect(screen.queryByText("管理控制台")).not.toBeInTheDocument();
  });

  it("shows 钱包管理 but no 团队管理 for a personal user", () => {
    auth.user.tenant_role = "";
    renderLayout();

    expect(screen.getByText("钱包管理")).toBeInTheDocument();
    expect(screen.queryByText("团队管理")).not.toBeInTheDocument();
    expect(screen.queryByText("管理控制台")).not.toBeInTheDocument();
  });

  it("shows 钱包管理 and 管理控制台 for a system admin", () => {
    auth.user.role = "admin";
    auth.user.tenant_role = "owner";
    renderLayout();

    expect(screen.getByText("钱包管理")).toBeInTheDocument();
    expect(screen.getByText("管理控制台")).toBeInTheDocument();
  });
});
