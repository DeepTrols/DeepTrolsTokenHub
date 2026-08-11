import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
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

// 生产环境: 2 calls across 2 models. 开发环境: 1 call. 新密钥: no calls.
function seedKeys() {
  return [
    { id: "key-1", name: "生产环境", masked_key: "sk-****abcd", status: "active" },
    { id: "key-2", name: "开发环境", masked_key: "sk-****efgh", status: "active" },
    { id: "key-3", name: "新密钥", masked_key: "sk-****zzzz", status: "active" },
  ];
}

function seedLogs() {
  return [
    { id: "log-1", model: "gpt-4o", request_id: "req-001", api_key_id: "key-1", api_key_name: "生产环境", status: "completed", input_tokens: 10, output_tokens: 20, cost: "0.50", created_at: "2026-08-01T00:00:00Z" },
    { id: "log-2", model: "claude-sonnet", request_id: "req-002", api_key_id: "key-1", api_key_name: "生产环境", status: "failed", input_tokens: 5, output_tokens: 0, cost: "0.10", created_at: "2026-08-01T00:01:00Z" },
    { id: "log-3", model: "deepseek-v3", request_id: "req-003", api_key_id: "key-2", api_key_name: "开发环境", status: "completed", input_tokens: 8, output_tokens: 12, cost: "0.20", created_at: "2026-08-01T00:02:00Z" },
  ];
}

function mockUsage(usageData: unknown, keysData: unknown = seedKeys()) {
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

  it("renders title, subtitle, and time-range control", async () => {
    mockUsage(seedLogs());

    renderWithProviders(<CallLogs />);

    expect(await screen.findByText(/按 API 密钥查看调用量与模型分布/)).toBeInTheDocument();
    expect(screen.getByText("调用记录")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "今天" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "近30天" })).toBeInTheDocument();
  });

  it("shows loading spinner while fetching", () => {
    mockApiGet.mockImplementation(() => new Promise(() => {}));

    renderWithProviders(<CallLogs />);

    expect(screen.getByText("加载调用记录...")).toBeInTheDocument();
  });

  it("groups logs by API key, showing key name and total call count", async () => {
    mockUsage(seedLogs());

    renderWithProviders(<CallLogs />);

    expect(await screen.findByText("生产环境")).toBeInTheDocument();
    expect(screen.getByText("开发环境")).toBeInTheDocument();
    expect(screen.getByText(/2 次调用/)).toBeInTheDocument();
    expect(screen.getByText(/1 次调用/)).toBeInTheDocument();
  });

  it("shows per-key totals for tokens and cost at 2 decimal places", async () => {
    mockUsage(seedLogs());

    renderWithProviders(<CallLogs />);

    // 生产环境: 35 tokens, 0.60 CNY
    expect(await screen.findByText(/35 tokens · 0\.60 CNY/)).toBeInTheDocument();
    // 开发环境: 20 tokens, 0.20 CNY
    expect(screen.getByText(/20 tokens · 0\.20 CNY/)).toBeInTheDocument();
  });

  it("shows a KPI summary of totals across the selected range", async () => {
    mockUsage(seedLogs());

    renderWithProviders(<CallLogs />);

    expect(await screen.findByText("总请求")).toBeInTheDocument();
    expect(screen.getByText("总费用")).toBeInTheDocument();
    // 0.50 + 0.10 + 0.20 = 0.80 CNY
    expect(screen.getByText("0.80 CNY")).toBeInTheDocument();
  });

  it("shows zero-usage API keys greyed out instead of hiding them", async () => {
    mockUsage(seedLogs(), seedKeys());

    renderWithProviders(<CallLogs />);

    expect(await screen.findByText("新密钥")).toBeInTheDocument();
    expect(screen.getByText("暂无调用")).toBeInTheDocument();

    const zeroKeyButton = screen.getByText("新密钥").closest("button");
    expect(zeroKeyButton).toBeDisabled();
  });

  it("expands a key to reveal its per-model breakdown", async () => {
    const user = userEvent.setup();
    mockUsage(seedLogs());

    renderWithProviders(<CallLogs />);

    const header = (await screen.findByText("生产环境")).closest("button");
    expect(header).not.toBeNull();
    await user.click(header as HTMLElement);

    expect(screen.getByText("gpt-4o")).toBeInTheDocument();
    expect(screen.getByText("claude-sonnet")).toBeInTheDocument();
    expect(screen.getByText(/1 次 · 30 tokens · 0\.50 CNY/)).toBeInTheDocument();
    expect(screen.getByText(/1 次 · 5 tokens · 0\.10 CNY/)).toBeInTheDocument();
  });

  it("keeps sibling keys collapsed until expanded", async () => {
    const user = userEvent.setup();
    mockUsage(seedLogs());

    renderWithProviders(<CallLogs />);

    const header = (await screen.findByText("生产环境")).closest("button");
    await user.click(header as HTMLElement);

    // 开发环境's model stays hidden while its key is collapsed.
    expect(screen.queryByText("deepseek-v3")).not.toBeInTheDocument();
  });

  it("re-fetches with from/to when the time range changes", async () => {
    const user = userEvent.setup();
    mockUsage(seedLogs());

    renderWithProviders(<CallLogs />);

    await screen.findByText("生产环境");
    await user.click(screen.getByRole("button", { name: "今天" }));

    await waitFor(() => expect(mockApiGet).toHaveBeenCalledWith(expect.stringContaining("from=")));
  });

  it("shows empty message when there are no keys and no logs", async () => {
    mockUsage([], []);

    renderWithProviders(<CallLogs />);

    expect(await screen.findByText("暂无调用记录")).toBeInTheDocument();
  });

  it("shows error with retry on fetch failure", async () => {
    mockApiGet.mockRejectedValue(new Error("load failed"));

    renderWithProviders(<CallLogs />);

    expect(await screen.findByText("load failed")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });

  it("falls back to a truncated key id when the key has no name", async () => {
    mockUsage(
      [
        { id: "log-9", model: "gpt-4o", request_id: "req-9", api_key_id: "abcdef1234567890", api_key_name: "", status: "completed", input_tokens: 1, output_tokens: 1, cost: "0.01", created_at: "2026-08-01T00:00:00Z" },
      ],
      [],
    );

    renderWithProviders(<CallLogs />);

    expect(await screen.findByText("abcdef12…")).toBeInTheDocument();
  });
});
