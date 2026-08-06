import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { AuthProvider } from "../lib/auth";
import Login from "./Login";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

const mockNavigate = vi.fn();
vi.mock("react-router-dom", async () => {
  const actual = await vi.importActual("react-router-dom");
  return { ...actual, useNavigate: () => mockNavigate };
});

function renderLogin() {
  return render(
    <MemoryRouter>
      <AuthProvider>
        <Login />
      </AuthProvider>
    </MemoryRouter>,
  );
}

describe("Login", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFetch.mockResolvedValueOnce({ ok: false, status: 401 });
  });

  it("renders the login form with account and password inputs", () => {
    renderLogin();
    expect(screen.getByPlaceholderText("请输入管理员账号")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("请输入密码")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /登 录/ })).toBeInTheDocument();
  });

  it("submits and navigates to dashboard on success", async () => {
    const user = userEvent.setup();
    mockFetch.mockReset();
    mockFetch
      .mockResolvedValueOnce({ ok: false, status: 401 })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ token: "x" }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ id: "u1", email: "admin@test.com", name: "Admin", role: "admin", status: "active", totp_enabled: false }) });

    renderLogin();
    await user.type(screen.getByPlaceholderText("请输入管理员账号"), "admin@test.com");
    await user.type(screen.getByPlaceholderText("请输入密码"), "password123");
    await user.click(screen.getByRole("button", { name: /登 录/ }));

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith("/dashboard");
    });
  });

  it("displays error when login fails", async () => {
    const user = userEvent.setup();
    mockFetch.mockReset();
    mockFetch
      .mockResolvedValueOnce({ ok: false, status: 401 })
      .mockResolvedValueOnce({ ok: false, status: 401, json: async () => ({ error: "Invalid credentials" }) });

    renderLogin();
    await user.type(screen.getByPlaceholderText("请输入管理员账号"), "admin@test.com");
    await user.type(screen.getByPlaceholderText("请输入密码"), "wrongpass");
    await user.click(screen.getByRole("button", { name: /登 录/ }));

    await waitFor(() => {
      expect(screen.getByText("登录失败，请检查账号和密码")).toBeInTheDocument();
    });
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("prompts for TOTP code when login requires MFA, then logs in", async () => {
    const user = userEvent.setup();
    mockFetch.mockReset();
    // initial fetchMe 401, login returns mfa_required, second fetchMe 401,
    // login with totp succeeds, final fetchMe returns user.
    mockFetch
      .mockResolvedValueOnce({ ok: false, status: 401 }) // mount fetchMe
      .mockResolvedValueOnce({ ok: false, status: 401, json: async () => ({ error: "TOTP code required", mfa_required: "true" }) }) // login 1
      .mockResolvedValueOnce({ ok: true, json: async () => ({ token: "x" }) }) // login 2 (totp)
      .mockResolvedValueOnce({ ok: true, json: async () => ({ id: "u1", email: "admin@test.com", name: "Admin", role: "admin", status: "active", totp_enabled: true }) }); // post-login fetchMe

    renderLogin();
    await user.type(screen.getByPlaceholderText("请输入管理员账号"), "admin@test.com");
    await user.type(screen.getByPlaceholderText("请输入密码"), "password123");
    await user.click(screen.getByRole("button", { name: /登 录/ }));

    // First attempt: MFA required -> code input appears.
    expect(await screen.findByPlaceholderText("输入6位动态验证码")).toBeInTheDocument();

    await user.type(screen.getByPlaceholderText("输入6位动态验证码"), "123456");
    await user.click(screen.getByRole("button", { name: /登 录/ }));

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith("/dashboard");
    });
  });

  it("shows loading state on submit", async () => {
    const user = userEvent.setup();
    mockFetch.mockReset();
    mockFetch.mockImplementation(() => new Promise(() => {}));

    renderLogin();
    await user.type(screen.getByPlaceholderText("请输入管理员账号"), "admin@test.com");
    await user.type(screen.getByPlaceholderText("请输入密码"), "password123");
    await user.click(screen.getByRole("button", { name: /登 录/ }));

    expect(screen.getByText("登录中...")).toBeInTheDocument();
  });
});
