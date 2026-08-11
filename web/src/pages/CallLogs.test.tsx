import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CallLogs from "./CallLogs";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

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

// jsdom has no ResizeObserver; recharts' ResponsiveContainer (rendered in the
// stats panel when data loads) requires it and crashes the test run without a stub.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
(globalThis as { ResizeObserver?: typeof ResizeObserver }).ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver;

// Radix Select calls hasPointerCapture/setPointerCapture on pointer events
// during trigger interaction; jsdom doesn't implement them. Stub on the
// prototype so the two Select tests can open the API key dropdown.
Object.defineProperty(Element.prototype, "hasPointerCapture", { configurable: true, value: () => false });
Object.defineProperty(Element.prototype, "setPointerCapture", { configurable: true, value: () => {} });
Object.defineProperty(Element.prototype, "releasePointerCapture", { configurable: true, value: () => {} });
Object.defineProperty(Element.prototype, "scrollIntoView", { configurable: true, value: () => {} });

function seedLogs() {
  return [
    { id: "log-1", model: "gpt-4o", request_id: "req-001", api_key_id: "key-1", api_key_name: "生产环境", status: "completed", input_tokens: 10, output_tokens: 20, cost: "0.50", created_at: "2026-08-01T00:00:00Z" },
    { id: "log-2", model: "claude-sonnet", request_id: "req-002", api_key_id: "key-2", api_key_name: "开发环境", status: "failed", input_tokens: 5, output_tokens: 0, cost: "0.10", created_at: "2026-08-01T00:01:00Z" },
  ];
}

function seedKeys() {
  return [
    { id: "key-1", name: "生产环境", masked_key: "sk-...abcd", status: "active" },
    { id: "key-2", name: "开发环境", masked_key: "sk-...efgh", status: "active" },
  ];
}

function mockUsage(usageData: unknown, keysData: unknown = []) {
  mockApiGet.mockImplementation((path: string) => {
    if (String(path).startsWith("/api-keys")) {
      return Promise.resolve({ data: keysData });
    }
    return Promise.resolve({ data: usageData });
  });
}

describe("CallLogs", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders page title and filter controls", async () => {
    mockUsage(seedLogs());

    renderWithProviders(<CallLogs />);

    expect(screen.getByText("调用记录")).toBeInTheDocument();
    expect(await screen.findByPlaceholderText("模型名称")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("请求 ID")).toBeInTheDocument();
  });

  it("shows loading spinner while fetching", () => {
    mockApiGet.mockImplementation(() => new Promise(() => {}));

    renderWithProviders(<CallLogs />);

    expect(screen.getByText("加载调用记录...")).toBeInTheDocument();
  });

  it("displays usage log rows when data loads", async () => {
    mockUsage(seedLogs());

    renderWithProviders(<CallLogs />);

    const table = within(await screen.findByRole("table"));
    expect(await table.findByText("gpt-4o")).toBeInTheDocument();
    expect(table.getByText("claude-sonnet")).toBeInTheDocument();
    expect(table.getByText("req-001")).toBeInTheDocument();
  });

  it("shows empty message when there are no logs", async () => {
    mockUsage([]);

    renderWithProviders(<CallLogs />);

    expect(await screen.findByText("暂无调用记录")).toBeInTheDocument();
  });

  it("shows error with retry on fetch failure", async () => {
    mockApiGet.mockRejectedValue(new Error("load failed"));

    renderWithProviders(<CallLogs />);

    expect(await screen.findByText("load failed")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });

  it("filters rows by model name", async () => {
    const user = userEvent.setup();
    mockUsage(seedLogs());

    renderWithProviders(<CallLogs />);

    const table = within(await screen.findByRole("table"));
    expect(await table.findByText("gpt-4o")).toBeInTheDocument();

    await user.type(screen.getByPlaceholderText("模型名称"), "gpt-4o");

    expect(table.getByText("gpt-4o")).toBeInTheDocument();
    expect(table.queryByText("claude-sonnet")).not.toBeInTheDocument();
  });

  it("shows the API key name in the API 密钥 column", async () => {
    mockUsage(seedLogs());

    renderWithProviders(<CallLogs />);

    const table = within(await screen.findByRole("table"));
    expect(await table.findByText("生产环境")).toBeInTheDocument();
    expect(table.getByText("开发环境")).toBeInTheDocument();
  });

  it("fetches and lists API keys in the filter dropdown", async () => {
    const user = userEvent.setup();
    mockUsage(seedLogs(), seedKeys());

    renderWithProviders(<CallLogs />);

    const table = within(await screen.findByRole("table"));
    await table.findByText("gpt-4o");

    const apiKeyTrigger = screen.getByText("全部密钥").closest("button");
    expect(apiKeyTrigger).not.toBeNull();
    await user.click(apiKeyTrigger as HTMLElement);

    expect(await screen.findByRole("option", { name: "生产环境" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "开发环境" })).toBeInTheDocument();
  });

  it("selecting an API key in the filter triggers a refetch with api_key_id", async () => {
    const user = userEvent.setup();
    mockUsage(seedLogs(), seedKeys());

    renderWithProviders(<CallLogs />);

    const table = within(await screen.findByRole("table"));
    await table.findByText("gpt-4o");

    const apiKeyTrigger = screen.getByText("全部密钥").closest("button");
    expect(apiKeyTrigger).not.toBeNull();
    await user.click(apiKeyTrigger as HTMLElement);
    await user.click(await screen.findByRole("option", { name: "生产环境" }));

    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledWith(expect.stringContaining("api_key_id=key-1"));
    });
  });
});
