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
    user: { id: "test-user", email: "test@test.com", name: "Test", role: "user", status: "active", totp_enabled: false },
    isLoading: false,
    isAuthenticated: true,
    logout: vi.fn(),
  }),
}));

import { api } from "../lib/api";
const mockApiGet = api.get as ReturnType<typeof vi.fn>;

function seedLogs() {
  return [
    { id: "log-1", model: "gpt-4o", request_id: "req-001", status: "completed", input_tokens: 10, output_tokens: 20, cost: "0.50", created_at: "2026-08-01T00:00:00Z" },
    { id: "log-2", model: "claude-sonnet", request_id: "req-002", status: "failed", input_tokens: 5, output_tokens: 0, cost: "0.10", created_at: "2026-08-01T00:01:00Z" },
  ];
}

describe("CallLogs", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders page title and filter controls", async () => {
    mockApiGet.mockResolvedValue({ data: seedLogs() });

    renderWithProviders(<CallLogs />);

    expect(screen.getByText("调用日志")).toBeInTheDocument();
    expect(await screen.findByPlaceholderText("模型名称")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("请求 ID")).toBeInTheDocument();
  });

  it("shows loading spinner while fetching", () => {
    mockApiGet.mockImplementation(() => new Promise(() => {}));

    renderWithProviders(<CallLogs />);

    expect(screen.getByText("加载调用日志...")).toBeInTheDocument();
  });

  it("displays usage log rows when data loads", async () => {
    mockApiGet.mockResolvedValue({ data: seedLogs() });

    renderWithProviders(<CallLogs />);

    expect(await screen.findByText("gpt-4o")).toBeInTheDocument();
    expect(screen.getByText("claude-sonnet")).toBeInTheDocument();
    expect(screen.getByText("req-001")).toBeInTheDocument();
  });

  it("shows empty message when there are no logs", async () => {
    mockApiGet.mockResolvedValue({ data: [] });

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
    mockApiGet.mockResolvedValue({ data: seedLogs() });

    renderWithProviders(<CallLogs />);

    expect(await screen.findByText("gpt-4o")).toBeInTheDocument();

    await user.type(screen.getByPlaceholderText("模型名称"), "gpt-4o");

    expect(screen.getByText("gpt-4o")).toBeInTheDocument();
    expect(screen.queryByText("claude-sonnet")).not.toBeInTheDocument();
  });
});
