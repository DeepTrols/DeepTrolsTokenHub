import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Playground from "./Playground";
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

// Mock auth - Playground page is wrapped by RequireAuth
vi.mock("../lib/auth", () => ({
  useAuth: () => ({
    user: { id: "test-user", email: "test@test.com", name: "Test", role: "user", status: "active", totp_enabled: false },
    isLoading: false,
    isAuthenticated: true,
    logout: vi.fn(),
  }),
}));

import { api } from "../lib/api";

const mockApiGet = api.get as ReturnType<typeof vi.fn>;
const mockApiPost = api.post as ReturnType<typeof vi.fn>;

// Stub global fetch for gateway calls (/v1/*)
const originalFetch = globalThis.fetch;

function createFetchMock() {
  return vi.fn();
}

// Real key plaintext the secret endpoint returns. The gateway rejects the key
// ID (UUID) with "Invalid API key", so models/chat requests must use this
// plaintext rather than the selected key's id.
const MOCK_PLAINTEXT = "dt-sk-testsecret1234567890abcdef";

// Set up api.get to return the keys list for /api-keys and the plaintext for
// any /secret call, so the playground's models/chat requests carry a real key.
function mockApiKeys(keys: unknown[]) {
  mockApiGet.mockImplementation((path: string) =>
    path.includes("/secret")
      ? Promise.resolve({ plaintext: MOCK_PLAINTEXT })
      : Promise.resolve({ data: keys }),
  );
}

describe("Playground", () => {
  let mockFetch: ReturnType<typeof createFetchMock>;

  beforeEach(() => {
    mockFetch = createFetchMock();
    globalThis.fetch = mockFetch;
    vi.clearAllMocks();

    // Default: no API keys
    mockApiGet.mockResolvedValue({ data: [] });
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("renders the page title and description", () => {
    // Arrange
    // Act
    renderWithProviders(<Playground />);

    // Assert
    expect(screen.getByText("在线体验")).toBeInTheDocument();
    expect(
      screen.getByText("使用真实 API Key 在线测试模型调用效果")
    ).toBeInTheDocument();
  });

  it("fetches API keys on mount", async () => {
    // Arrange
    // Act
    renderWithProviders(<Playground />);

    // Assert
    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledWith("/api-keys");
    });
  });

  it("shows helper message when user has no API keys", async () => {
    // Arrange
    mockApiGet.mockResolvedValue({ data: [] });

    // Act
    renderWithProviders(<Playground />);

    // Assert
    await waitFor(() => {
      expect(screen.getByText("请先创建 API 密钥")).toBeInTheDocument();
    });
  });

  it("shows API key dropdown when keys are available", async () => {
    // Arrange
    mockApiGet.mockResolvedValue({
      data: [
        {
          id: "key-1",
          name: "Production Key",
          masked_key: "sk-***abc",
          status: "active",
          created_at: "2025-01-01T00:00:00Z",
        },
        {
          id: "key-2",
          name: "Dev Key",
          masked_key: "sk-***xyz",
          status: "active",
          created_at: "2025-02-01T00:00:00Z",
        },
      ],
    });

    // Act
    renderWithProviders(<Playground />);

    // Assert
    await waitFor(() => {
      expect(screen.getByText("Production Key")).toBeInTheDocument();
    });
    expect(screen.getByText("Dev Key")).toBeInTheDocument();
    expect(
      screen.queryByText("请先创建 API 密钥")
    ).not.toBeInTheDocument();
  });

  it("fetches models from gateway using API key Bearer when a key is selected", async () => {
    // Arrange
    const user = userEvent.setup();
    mockApiKeys([
      {
        id: "key-1",
        name: "Production Key",
        masked_key: "sk-***abc",
        status: "active",
        created_at: "2025-01-01T00:00:00Z",
      },
    ]);

    // Mock the gateway models response
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        object: "list",
        data: [
          { id: "gpt-4o", object: "model", created: 1234567890, owned_by: "openai" },
          { id: "claude-sonnet-4-20250514", object: "model", created: 1234567890, owned_by: "anthropic" },
        ],
      }),
    });

    // Act
    renderWithProviders(<Playground />);

    // Wait for keys to load and select a key
    await waitFor(() => {
      expect(screen.getByText("Production Key")).toBeInTheDocument();
    });

    const select = screen.getByRole("combobox", { name: "选择 API 密钥" });
    await user.selectOptions(select, "key-1");

    // Assert: gateway fetch called with the real API key plaintext as Bearer
    // (never the key id — the gateway rejects the UUID with "Invalid API key").
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith("/v1/models", {
        headers: {
          "Content-Type": "application/json",
          Authorization: "Bearer dt-sk-testsecret1234567890abcdef",
        },
      });
    });
  });

  it("does NOT fetch models when no API key is selected", async () => {
    // Arrange
    mockApiKeys([
      {
        id: "key-1",
        name: "Production Key",
        masked_key: "sk-***abc",
        status: "active",
        created_at: "2025-01-01T00:00:00Z",
      },
    ]);

    // Act
    renderWithProviders(<Playground />);

    // Wait for keys to load (we do NOT select a key)
    await waitFor(() => {
      expect(screen.getByText("Production Key")).toBeInTheDocument();
    });

    // Assert: gateway fetch should NOT have been called (no key selected)
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("sends chat request with selected API key as Bearer and selected model", async () => {
    // Arrange
    const user = userEvent.setup();
    mockApiKeys([
      {
        id: "key-real",
        name: "Real Key",
        masked_key: "sk-***123",
        status: "active",
        created_at: "2025-01-01T00:00:00Z",
      },
    ]);

    // Gateway models response
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        object: "list",
        data: [
          { id: "gpt-4o", object: "model", created: 1234567890, owned_by: "openai" },
        ],
      }),
    });

    // Gateway chat response
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        choices: [{ message: { content: "Hello from AI!" } }],
        usage: { prompt_tokens: 10, completion_tokens: 20, total_tokens: 30 },
      }),
    });

    // Act
    renderWithProviders(<Playground />);

    // Select key
    await waitFor(() => {
      expect(screen.getByText("Real Key")).toBeInTheDocument();
    });
    const keySelect = screen.getByRole("combobox", { name: "选择 API 密钥" });
    await user.selectOptions(keySelect, "key-real");

    // Wait for models to load
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(1); // models call
    });

    // Type prompt and send
    const textarea = screen.getByPlaceholderText("在此输入您的问题或提示词...");
    await user.type(textarea, "Hello AI");
    await user.click(screen.getByRole("button", { name: /发送请求/ }));

    // Assert: chat call uses API key as Bearer
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(2); // models + chat
    });

    const chatCallArgs = mockFetch.mock.calls[1];
    expect(chatCallArgs[0]).toBe("/v1/chat/completions");
    expect(chatCallArgs[1]).toMatchObject({
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer dt-sk-testsecret1234567890abcdef",
      },
    });

    const body = JSON.parse(chatCallArgs[1].body);
    expect(body.model).toBe("gpt-4o");
    expect(body.messages).toEqual([{ role: "user", content: "Hello AI" }]);
  });

  it("displays response and usage after successful chat", async () => {
    // Arrange
    const user = userEvent.setup();
    mockApiKeys([
      {
        id: "key-real",
        name: "Real Key",
        masked_key: "sk-***123",
        status: "active",
        created_at: "2025-01-01T00:00:00Z",
      },
    ]);

    mockFetch
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          object: "list",
          data: [
            { id: "gpt-4o", object: "model", created: 1234567890, owned_by: "openai" },
          ],
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          choices: [{ message: { content: "Hello from AI!" } }],
          usage: { prompt_tokens: 10, completion_tokens: 20, total_tokens: 30 },
        }),
      });

    // Act
    renderWithProviders(<Playground />);

    // Select key
    await waitFor(() => {
      expect(screen.getByText("Real Key")).toBeInTheDocument();
    });
    await user.selectOptions(
      screen.getByRole("combobox", { name: "选择 API 密钥" }),
      "key-real"
    );

    // Type and send
    await user.type(
      screen.getByPlaceholderText("在此输入您的问题或提示词..."),
      "Hello AI"
    );
    await user.click(screen.getByRole("button", { name: /发送请求/ }));

    // Assert: response and usage displayed
    await waitFor(() => {
      expect(screen.getByText("Hello from AI!")).toBeInTheDocument();
    });
    expect(screen.getByText(/30 tokens/)).toBeInTheDocument();
  });

  it("shows error when gateway returns error response", async () => {
    // Arrange
    const user = userEvent.setup();
    mockApiKeys([
      {
        id: "key-bad",
        name: "Bad Key",
        masked_key: "sk-***fail",
        status: "active",
        created_at: "2025-01-01T00:00:00Z",
      },
    ]);

    mockFetch
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          object: "list",
          data: [
            { id: "gpt-4o", object: "model", created: 1234567890, owned_by: "openai" },
          ],
        }),
      })
      .mockResolvedValueOnce({
        ok: false,
        json: async () => ({
          error: { message: "Invalid API key" },
        }),
      });

    // Act
    renderWithProviders(<Playground />);
    await waitFor(() => {
      expect(screen.getByText("Bad Key")).toBeInTheDocument();
    });
    await user.selectOptions(
      screen.getByRole("combobox", { name: "选择 API 密钥" }),
      "key-bad"
    );
    await user.type(
      screen.getByPlaceholderText("在此输入您的问题或提示词..."),
      "test"
    );
    await user.click(screen.getByRole("button", { name: /发送请求/ }));

    // Assert: error message displayed
    await waitFor(() => {
      expect(screen.getByText("Invalid API key")).toBeInTheDocument();
    });
  });

  it("shows error when gateway fetch throws network error", async () => {
    // Arrange
    const user = userEvent.setup();
    mockApiKeys([
      {
        id: "key-net",
        name: "Net Key",
        masked_key: "sk-***net",
        status: "active",
        created_at: "2025-01-01T00:00:00Z",
      },
    ]);

    mockFetch
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          object: "list",
          data: [
            { id: "gpt-4o", object: "model", created: 1234567890, owned_by: "openai" },
          ],
        }),
      })
      .mockRejectedValueOnce(new Error("Network failure"));

    // Act
    renderWithProviders(<Playground />);
    await waitFor(() => {
      expect(screen.getByText("Net Key")).toBeInTheDocument();
    });
    await user.selectOptions(
      screen.getByRole("combobox", { name: "选择 API 密钥" }),
      "key-net"
    );
    await user.type(
      screen.getByPlaceholderText("在此输入您的问题或提示词..."),
      "test"
    );
    await user.click(screen.getByRole("button", { name: /发送请求/ }));

    // Assert: network error message
    await waitFor(() => {
      expect(screen.getByText(/网络错误/)).toBeInTheDocument();
    });
  });

  it("disables send button when prompt is empty", async () => {
    // Arrange
    mockApiKeys([
      {
        id: "key-1",
        name: "Production Key",
        masked_key: "sk-***abc",
        status: "active",
        created_at: "2025-01-01T00:00:00Z",
      },
    ]);

    // Act
    renderWithProviders(<Playground />);

    await waitFor(() => {
      expect(screen.getByText("Production Key")).toBeInTheDocument();
    });

    // Assert: send button should be disabled with empty prompt
    const sendButton = screen.getByRole("button", { name: /发送请求/ });
    expect(sendButton).toBeDisabled();
  });

  it("resets response and error when reset button is clicked", async () => {
    // Arrange
    const user = userEvent.setup();
    mockApiKeys([
      {
        id: "key-reset",
        name: "Reset Key",
        masked_key: "sk-***rst",
        status: "active",
        created_at: "2025-01-01T00:00:00Z",
      },
    ]);

    mockFetch
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          object: "list",
          data: [
            { id: "gpt-4o", object: "model", created: 1234567890, owned_by: "openai" },
          ],
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          choices: [{ message: { content: "Response text" } }],
          usage: { prompt_tokens: 5, completion_tokens: 5, total_tokens: 10 },
        }),
      });

    renderWithProviders(<Playground />);
    await waitFor(() => {
      expect(screen.getByText("Reset Key")).toBeInTheDocument();
    });
    await user.selectOptions(
      screen.getByRole("combobox", { name: "选择 API 密钥" }),
      "key-reset"
    );
    await user.type(
      screen.getByPlaceholderText("在此输入您的问题或提示词..."),
      "test"
    );
    await user.click(screen.getByRole("button", { name: /发送请求/ }));

    await waitFor(() => {
      expect(screen.getByText("Response text")).toBeInTheDocument();
    });

    // Act: click reset
    await user.click(screen.getByRole("button", { name: /重置/ }));

    // Assert: response and usage cleared
    await waitFor(() => {
      expect(screen.queryByText("Response text")).not.toBeInTheDocument();
    });
    expect(
      screen.getByText("在左侧输入提示词并点击发送")
    ).toBeInTheDocument();
  });

  it("handles API keys fetch error gracefully", async () => {
    // Arrange
    mockApiGet.mockRejectedValue(new Error("Network error"));

    // Act
    renderWithProviders(<Playground />);

    // Assert: should show no-key helper, not crash
    await waitFor(() => {
      expect(screen.getByText("请先创建 API 密钥")).toBeInTheDocument();
    });
  });

  it("shows error when gateway models endpoint returns non-ok response", async () => {
    // Arrange
    const user = userEvent.setup();
    mockApiKeys([
      {
        id: "key-err",
        name: "Error Key",
        masked_key: "sk-***err",
        status: "active",
        created_at: "2025-01-01T00:00:00Z",
      },
    ]);

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 401,
      json: async () => ({
        error: { message: "Invalid API key for models" },
      }),
    });

    // Act
    renderWithProviders(<Playground />);

    await waitFor(() => {
      expect(screen.getByText("Error Key")).toBeInTheDocument();
    });
    await user.selectOptions(
      screen.getByRole("combobox", { name: "选择 API 密钥" }),
      "key-err"
    );

    // Assert: error message from models endpoint
    await waitFor(() => {
      expect(
        screen.getByText(/获取模型列表失败.*Invalid API key for models/)
      ).toBeInTheDocument();
    });
  });

  it("shows error when gateway models fetch throws network error", async () => {
    // Arrange
    const user = userEvent.setup();
    mockApiKeys([
      {
        id: "key-net2",
        name: "Net Key 2",
        masked_key: "sk-***net2",
        status: "active",
        created_at: "2025-01-01T00:00:00Z",
      },
    ]);

    mockFetch.mockRejectedValueOnce(new Error("Models network failure"));

    // Act
    renderWithProviders(<Playground />);

    await waitFor(() => {
      expect(screen.getByText("Net Key 2")).toBeInTheDocument();
    });
    await user.selectOptions(
      screen.getByRole("combobox", { name: "选择 API 密钥" }),
      "key-net2"
    );

    // Assert: network error from models endpoint
    await waitFor(() => {
      expect(
        screen.getByText(/获取模型列表失败.*Models network failure/)
      ).toBeInTheDocument();
    });
  });
});
