import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { AuthProvider } from "../lib/auth";
import Register from "./Register";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

const mockNavigate = vi.fn();
vi.mock("react-router-dom", async () => {
  const actual = await vi.importActual("react-router-dom");
  return { ...actual, useNavigate: () => mockNavigate };
});

function renderRegister() {
  return render(
    <MemoryRouter initialEntries={["/register"]}>
      <AuthProvider>
        <Register />
      </AuthProvider>
    </MemoryRouter>,
  );
}

describe("Register", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFetch.mockResolvedValueOnce({ ok: false, status: 401 });
  });

  it("renders the personal registration form by default", async () => {
    renderRegister();
    expect(screen.getByRole("img", { name: "DEEPTROLS" })).toBeInTheDocument();
    expect(screen.getByText("AI 模型聚合平台 · 创建账号")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("请输入昵称")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("请输入邮箱")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("至少8位")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /注 册/i })).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("请输入公司名称")).not.toBeInTheDocument();
  });

  it("switches to the enterprise registration form", async () => {
    const user = userEvent.setup();
    renderRegister();

    await user.click(screen.getByRole("button", { name: "企业" }));

    expect(screen.getByText("企业 AI 模型聚合平台 · 创建账号")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("请输入公司名称")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("请输入联系人姓名")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("请输入邮箱")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("至少8位")).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("请输入昵称")).not.toBeInTheDocument();
  });

  it("clears previously typed values when switching account type", async () => {
    const user = userEvent.setup();
    renderRegister();

    await user.type(screen.getByPlaceholderText("请输入昵称"), "Test User");
    await user.type(screen.getByPlaceholderText("请输入邮箱"), "test@example.com");

    await user.click(screen.getByRole("button", { name: "企业" }));
    await user.click(screen.getByRole("button", { name: "个人" }));

    expect(screen.getByPlaceholderText("请输入昵称")).toHaveValue("");
    expect(screen.getByPlaceholderText("请输入邮箱")).toHaveValue("");
  });

  it("submits personal registration and navigates to dashboard on success", async () => {
    const user = userEvent.setup();
    mockFetch.mockReset();
    mockFetch
      .mockResolvedValueOnce({ ok: false, status: 401 })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ token: "x" }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ id: "u1", email: "test@example.com", name: "Test User", role: "user", status: "active" }) });

    renderRegister();
    await user.type(screen.getByPlaceholderText("请输入昵称"), "Test User");
    await user.type(screen.getByPlaceholderText("请输入邮箱"), "test@example.com");
    await user.type(screen.getByPlaceholderText("至少8位"), "password123");
    await user.click(screen.getByRole("button", { name: /注 册/i }));

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith("/dashboard");
    });
  });

  it("submits enterprise registration with company + contact fields", async () => {
    const user = userEvent.setup();
    mockFetch.mockReset();
    mockFetch
      .mockResolvedValueOnce({ ok: false, status: 401 })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ token: "x" }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ id: "u2", email: "acme@example.com", name: "Acme 科技", role: "user", status: "active", user_type: "enterprise" }) });

    renderRegister();
    await user.click(screen.getByRole("button", { name: "企业" }));
    await user.type(screen.getByPlaceholderText("请输入公司名称"), "Acme 科技");
    await user.type(screen.getByPlaceholderText("请输入联系人姓名"), "张三");
    await user.type(screen.getByPlaceholderText("请输入邮箱"), "acme@example.com");
    await user.type(screen.getByPlaceholderText("至少8位"), "password123");
    await user.click(screen.getByRole("button", { name: /注 册/i }));

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith("/dashboard");
    });

    expect(mockFetch).toHaveBeenNthCalledWith(
      2,
      "/api/console/auth/register/enterprise",
      expect.objectContaining({
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          company_name: "Acme 科技",
          contact_name: "张三",
          email: "acme@example.com",
          password: "password123",
        }),
      }),
    );
  });

  it("displays server error when enterprise registration fails", async () => {
    const user = userEvent.setup();
    mockFetch.mockReset();
    mockFetch
      .mockResolvedValueOnce({ ok: false, status: 401 })
      .mockResolvedValueOnce({ ok: false, status: 409, json: async () => ({ error: "Email already registered" }) });

    renderRegister();
    await user.click(screen.getByRole("button", { name: "企业" }));
    await user.type(screen.getByPlaceholderText("请输入公司名称"), "Acme 科技");
    await user.type(screen.getByPlaceholderText("请输入联系人姓名"), "张三");
    await user.type(screen.getByPlaceholderText("请输入邮箱"), "acme@example.com");
    await user.type(screen.getByPlaceholderText("至少8位"), "password123");
    await user.click(screen.getByRole("button", { name: /注 册/i }));

    await waitFor(() => {
      expect(screen.getByText("Email already registered")).toBeInTheDocument();
    });
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("displays error when personal registration fails", async () => {
    const user = userEvent.setup();
    mockFetch.mockReset();
    mockFetch
      .mockResolvedValueOnce({ ok: false, status: 401 })
      .mockRejectedValueOnce(new Error("Email already registered"));

    renderRegister();
    await user.type(screen.getByPlaceholderText("请输入昵称"), "Test User");
    await user.type(screen.getByPlaceholderText("请输入邮箱"), "existing@example.com");
    await user.type(screen.getByPlaceholderText("至少8位"), "password123");
    await user.click(screen.getByRole("button", { name: /注 册/i }));

    await waitFor(() => {
      expect(screen.getByText("Email already registered")).toBeInTheDocument();
    });
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("displays server error and does not navigate when personal registration returns 409", async () => {
    const user = userEvent.setup();
    mockFetch.mockReset();
    mockFetch
      .mockResolvedValueOnce({ ok: false, status: 401 })
      .mockResolvedValueOnce({ ok: false, status: 409, json: async () => ({ error: "Email already registered" }) });

    renderRegister();
    await user.type(screen.getByPlaceholderText("请输入昵称"), "Test User");
    await user.type(screen.getByPlaceholderText("请输入邮箱"), "existing@example.com");
    await user.type(screen.getByPlaceholderText("至少8位"), "password123");
    await user.click(screen.getByRole("button", { name: /注 册/i }));

    await waitFor(() => {
      expect(screen.getByText("Email already registered")).toBeInTheDocument();
    });
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("shows generic error for non-Error rejections", async () => {
    const user = userEvent.setup();
    mockFetch.mockReset();
    mockFetch
      .mockResolvedValueOnce({ ok: false, status: 401 })
      .mockRejectedValueOnce("unknown error");

    renderRegister();
    await user.type(screen.getByPlaceholderText("请输入昵称"), "Test User");
    await user.type(screen.getByPlaceholderText("请输入邮箱"), "test@example.com");
    await user.type(screen.getByPlaceholderText("至少8位"), "password123");
    await user.click(screen.getByRole("button", { name: /注 册/i }));

    await waitFor(() => {
      expect(screen.getByText("注册失败，请稍后重试")).toBeInTheDocument();
    });
  });
});
