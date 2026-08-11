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
    <MemoryRouter>
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

  it("renders the registration form with all fields", async () => {
    renderRegister();
    expect(screen.getByText("DeepTrols")).toBeInTheDocument();
    expect(screen.getByText("AI 模型聚合平台 · 创建账号")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("请输入昵称")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("请输入邮箱")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("请输入密码（至少8位）")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /注 册/i })).toBeInTheDocument();
  });

  it("submits and navigates to dashboard on success", async () => {
    const user = userEvent.setup();
    mockFetch.mockReset();
    mockFetch
      .mockResolvedValueOnce({ ok: false, status: 401 })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ token: "x" }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ id: "u1", email: "test@example.com", name: "Test User", role: "user", status: "active" }) });

    renderRegister();
    await user.type(screen.getByPlaceholderText("请输入昵称"), "Test User");
    await user.type(screen.getByPlaceholderText("请输入邮箱"), "test@example.com");
    await user.type(screen.getByPlaceholderText("请输入密码（至少8位）"), "password123");
    await user.click(screen.getByRole("button", { name: /注 册/i }));

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith("/dashboard");
    });
  });

  it("displays error when registration fails", async () => {
    const user = userEvent.setup();
    mockFetch.mockReset();
    mockFetch
      .mockResolvedValueOnce({ ok: false, status: 401 })
      .mockRejectedValueOnce(new Error("Email already registered"));

    renderRegister();
    await user.type(screen.getByPlaceholderText("请输入昵称"), "Test User");
    await user.type(screen.getByPlaceholderText("请输入邮箱"), "existing@example.com");
    await user.type(screen.getByPlaceholderText("请输入密码（至少8位）"), "password123");
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
    await user.type(screen.getByPlaceholderText("请输入密码（至少8位）"), "password123");
    await user.click(screen.getByRole("button", { name: /注 册/i }));

    await waitFor(() => {
      expect(screen.getByText("注册失败，请稍后重试")).toBeInTheDocument();
    });
  });
});
