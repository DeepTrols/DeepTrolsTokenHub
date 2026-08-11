import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Docs from "./Docs";
import { renderWithProviders } from "../test/test-utils";

// Mock the api module so we control console API responses
vi.mock("../lib/api", () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  },
}));

// Mock auth - Docs page is wrapped by RequireAuth
vi.mock("../lib/auth", () => ({
  useAuth: () => ({
    user: { id: "test-user", email: "test@test.com", name: "Test", role: "user", status: "active" },
    isLoading: false,
    isAuthenticated: true,
    logout: vi.fn(),
  }),
}));

import { api } from "../lib/api";

const mockApiGet = api.get as ReturnType<typeof vi.fn>;

describe("Docs", () => {
  beforeEach(() => {
    vi.clearAllMocks();

    // Default: no API keys
    mockApiGet.mockResolvedValue({ data: [] });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // ============================================================
  // Core rendering tests
  // ============================================================

  it("renders the page title and description", () => {
    // Arrange
    // Act
    renderWithProviders(<Docs />);

    // Assert
    expect(screen.getByText("开发文档")).toBeInTheDocument();
    expect(
      screen.getByText(/集成 DeepTrols AI 模型聚合平台的完整开发指南/)
    ).toBeInTheDocument();
  });

  it("renders all 3 tab navigation items", () => {
    // Arrange
    // Act
    renderWithProviders(<Docs />);

    // Assert
    expect(screen.getByRole("tab", { name: "快速开始" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "API 参考" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "计费说明" })).toBeInTheDocument();
  });

  it("shows quickstart tab content by default", () => {
    // Arrange
    // Act
    renderWithProviders(<Docs />);

    // Assert
    expect(screen.getByText("注册与认证")).toBeInTheDocument();
    expect(screen.getByText("curl 示例")).toBeInTheDocument();
  });

  // ============================================================
  // Tab navigation tests
  // ============================================================

  it("navigates to API Reference tab when clicked", async () => {
    // Arrange
    const user = userEvent.setup();
    renderWithProviders(<Docs />);

    // Act
    await user.click(screen.getByRole("tab", { name: "API 参考" }));

    // Assert
    await waitFor(() => {
      expect(screen.getByText("Chat Completions")).toBeInTheDocument();
    });
    // POST badge and endpoint path are in separate elements, verify both exist
    expect(screen.getByText("POST")).toBeInTheDocument();
    expect(screen.getByText("/v1/chat/completions")).toBeInTheDocument();
  });

  it("navigates to Billing tab when clicked", async () => {
    // Arrange
    const user = userEvent.setup();
    renderWithProviders(<Docs />);

    // Act
    await user.click(screen.getByRole("tab", { name: "计费说明" }));

    // Assert
    await waitFor(() => {
      expect(screen.getByText("计费维度")).toBeInTheDocument();
    });
  });

  it("navigates back to Quickstart tab after switching away", async () => {
    // Arrange
    const user = userEvent.setup();
    renderWithProviders(<Docs />);

    // Act: go to API reference, then back to quickstart
    await user.click(screen.getByRole("tab", { name: "API 参考" }));
    await waitFor(() => {
      expect(screen.getByText("Chat Completions")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("tab", { name: "快速开始" }));

    // Assert: quickstart content visible again
    await waitFor(() => {
      expect(screen.getByText("curl 示例")).toBeInTheDocument();
    });
  });

  // ============================================================
  // Quickstart content tests
  // ============================================================

  it("shows registration step in quickstart", () => {
    // Arrange
    // Act
    renderWithProviders(<Docs />);

    // Assert
    expect(screen.getByText(/注册账号/i)).toBeInTheDocument();
  });

  it("shows API key creation step in quickstart", () => {
    // Arrange
    // Act
    renderWithProviders(<Docs />);

    // Assert
    expect(screen.getByText(/创建 API 密钥/i)).toBeInTheDocument();
  });

  it("shows curl code example", () => {
    // Arrange
    // Act
    renderWithProviders(<Docs />);

    // Assert
    expect(screen.getByText("curl 示例")).toBeInTheDocument();
    // The curl example should reference the chat completions endpoint
    expect(
      screen.getByText(/v1\/chat\/completions/)
    ).toBeInTheDocument();
  });

  it("shows Python code example", () => {
    // Arrange
    // Act
    renderWithProviders(<Docs />);

    // Assert
    expect(screen.getByText("Python 示例")).toBeInTheDocument();
    expect(screen.getByText(/from openai import OpenAI/)).toBeInTheDocument();
  });

  it("shows Node.js code example", () => {
    // Arrange
    // Act
    renderWithProviders(<Docs />);

    // Assert
    expect(screen.getByText("Node.js 示例")).toBeInTheDocument();
    expect(screen.getByText(/import OpenAI from "openai"/)).toBeInTheDocument();
  });

  // ============================================================
  // API Reference content tests
  // ============================================================

  it("shows authentication section in API reference", async () => {
    // Arrange
    const user = userEvent.setup();
    renderWithProviders(<Docs />);

    // Act
    await user.click(screen.getByRole("tab", { name: "API 参考" }));

    // Assert
    await waitFor(() => {
      expect(screen.getByText("认证方式")).toBeInTheDocument();
    });
    expect(
      screen.getByText(/Authorization: Bearer/)
    ).toBeInTheDocument();
  });

  it("shows error codes table in API reference", async () => {
    // Arrange
    const user = userEvent.setup();
    renderWithProviders(<Docs />);

    // Act
    await user.click(screen.getByRole("tab", { name: "API 参考" }));

    // Assert: "错误码" appears as both heading and table column header
    await waitFor(() => {
      const errorCodeElements = screen.getAllByText("错误码");
      expect(errorCodeElements.length).toBeGreaterThanOrEqual(2);
    });
    expect(screen.getByText("401")).toBeInTheDocument();
    expect(screen.getByText("403")).toBeInTheDocument();
    expect(screen.getByText("429")).toBeInTheDocument();
    expect(screen.getByText("500")).toBeInTheDocument();
  });

  it("shows List Models endpoint in API reference", async () => {
    // Arrange
    const user = userEvent.setup();
    renderWithProviders(<Docs />);

    // Act
    await user.click(screen.getByRole("tab", { name: "API 参考" }));

    // Assert
    await waitFor(() => {
      expect(screen.getByText("GET")).toBeInTheDocument();
      expect(screen.getByText("/v1/models")).toBeInTheDocument();
    });
  });

  it("shows request/response examples for chat completions", async () => {
    // Arrange
    const user = userEvent.setup();
    renderWithProviders(<Docs />);

    // Act
    await user.click(screen.getByRole("tab", { name: "API 参考" }));

    // Assert: both Chat Completions and List Models sections have response examples
    await waitFor(() => {
      const requestExamples = screen.getAllByText(/请求示例/);
      const responseExamples = screen.getAllByText(/响应示例/);
      expect(requestExamples.length).toBeGreaterThanOrEqual(1);
      expect(responseExamples.length).toBeGreaterThanOrEqual(1);
    });
  });

  // ============================================================
  // Billing section tests
  // ============================================================

  it("shows pricing dimensions in billing tab", async () => {
    // Arrange
    const user = userEvent.setup();
    renderWithProviders(<Docs />);

    // Act
    await user.click(screen.getByRole("tab", { name: "计费说明" }));

    // Assert
    await waitFor(() => {
      expect(screen.getByText("计费维度")).toBeInTheDocument();
      expect(screen.getByText(/input/)).toBeInTheDocument();
      expect(screen.getByText(/output/)).toBeInTheDocument();
    });
  });

  it("shows wallet balance explanation", async () => {
    // Arrange
    const user = userEvent.setup();
    renderWithProviders(<Docs />);

    // Act
    await user.click(screen.getByRole("tab", { name: "计费说明" }));

    // Assert: "钱包余额" appears as heading and in description text
    const walletElements = screen.getAllByText(/钱包余额/);
    expect(walletElements.length).toBeGreaterThanOrEqual(2);
  });

  it("shows pricing unit explanation (per 1K tokens)", async () => {
    // Arrange
    const user = userEvent.setup();
    renderWithProviders(<Docs />);

    // Act
    await user.click(screen.getByRole("tab", { name: "计费说明" }));

    // Assert: "1K tokens" appears in multiple pricing rows
    const tokenElements = screen.getAllByText(/1K tokens/);
    expect(tokenElements.length).toBeGreaterThanOrEqual(2);
  });

  it("shows cache_read and cache_write pricing dimensions", async () => {
    // Arrange
    const user = userEvent.setup();
    renderWithProviders(<Docs />);

    // Act
    await user.click(screen.getByRole("tab", { name: "计费说明" }));

    // Assert
    expect(screen.getByText(/cache_read/)).toBeInTheDocument();
    expect(screen.getByText(/cache_write/)).toBeInTheDocument();
  });
});
