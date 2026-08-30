import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Channels from "./Channels";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

vi.mock("../lib/auth", () => ({
  useAuth: () => ({
    user: { id: "admin", email: "admin@test.com", name: "Admin", role: "admin", status: "active" },
    isLoading: false, isAuthenticated: true, logout: vi.fn(),
  }),
}));

import { adminApi } from "../lib/api";
const mockAdminGet = adminApi.get as ReturnType<typeof vi.fn>;
const mockAdminPost = adminApi.post as ReturnType<typeof vi.fn>;
const mockAdminDelete = adminApi.delete as ReturnType<typeof vi.fn>;

function seedCredentials() {
  return [
    { id: "c1", name: "OpenAI Prod", provider: "openai", base_url: "https://api.openai.com", masked_key: "****1234", status: "active", model_count: 3, channel_ids: ["ch1", "ch2", "ch3"], created_at: "2026-08-01T00:00:00Z", updated_at: "2026-08-01T00:00:00Z" },
    { id: "c2", name: "Anthropic Dev", provider: "anthropic", base_url: "https://api.anthropic.com", masked_key: "****5678", status: "active", model_count: 2, channel_ids: ["ch4", "ch5"], created_at: "2026-08-01T00:00:00Z", updated_at: "2026-08-01T00:00:00Z" },
  ];
}

describe("Channels", () => {
  beforeEach(() => { vi.clearAllMocks(); });
  afterEach(() => { vi.restoreAllMocks(); });

  it("renders page title and add button", async () => {
    mockAdminGet.mockResolvedValue({ data: seedCredentials() });
    renderWithProviders(<Channels />);
    expect(screen.getByText("渠道管理")).toBeInTheDocument();
    expect(await screen.findByText("添加渠道")).toBeInTheDocument();
  });

  it("shows loading spinner", () => {
    mockAdminGet.mockImplementation(() => new Promise(() => {}));
    renderWithProviders(<Channels />);
    expect(screen.getByText("加载渠道...")).toBeInTheDocument();
  });

  it("displays credential cards", async () => {
    mockAdminGet.mockResolvedValue({ data: seedCredentials() });
    renderWithProviders(<Channels />);
    expect(await screen.findByText("OpenAI Prod")).toBeInTheDocument();
    expect(screen.getByText("Anthropic Dev")).toBeInTheDocument();
  });

  it("shows model counts", async () => {
    mockAdminGet.mockResolvedValue({ data: seedCredentials() });
    renderWithProviders(<Channels />);
    expect(await screen.findByText("3 个模型")).toBeInTheDocument();
    expect(screen.getByText("2 个模型")).toBeInTheDocument();
  });

  it("shows test/sync/edit/delete actions and no disable toggle", async () => {
    mockAdminGet.mockResolvedValue({ data: seedCredentials() });
    renderWithProviders(<Channels />);
    expect(await screen.findAllByText("测试")).toHaveLength(2);
    expect(screen.getAllByText("同步模型")).toHaveLength(2);
    expect(screen.getAllByText("编辑")).toHaveLength(2);
    expect(screen.getAllByText("删除")).toHaveLength(2);
    expect(screen.queryByText("停用")).not.toBeInTheDocument();
    expect(screen.queryByText("启用")).not.toBeInTheDocument();
  });

  it("deletes a provider credential after confirmation", async () => {
    const user = userEvent.setup();
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    mockAdminGet.mockResolvedValue({ data: seedCredentials() });
    mockAdminDelete.mockResolvedValue({ status: "deleted" });

    renderWithProviders(<Channels />);
    await user.click((await screen.findAllByText("删除"))[0]);

    await waitFor(() => {
      expect(mockAdminDelete).toHaveBeenCalledWith("/providers/c1");
    });
    confirmSpy.mockRestore();
  });

  it("shows empty state", async () => {
    mockAdminGet.mockResolvedValue({ data: [] });
    renderWithProviders(<Channels />);
    expect(await screen.findByText("暂无渠道")).toBeInTheDocument();
  });

  it("shows error with retry", async () => {
    mockAdminGet.mockRejectedValue(new Error("channels down"));
    renderWithProviders(<Channels />);
    expect(await screen.findByText("channels down")).toBeInTheDocument();
  });

  it("runs a real connectivity probe and reports model count", async () => {
    const user = userEvent.setup();
    mockAdminGet.mockResolvedValue({ data: seedCredentials() });
    mockAdminPost.mockResolvedValue({
      ok: true,
      ms: 128,
      models: 3,
      model_codes: ["deepseek-chat", "deepseek-reasoner"],
      capabilities: { chat: 3 },
    });

    renderWithProviders(<Channels />);

    await user.click((await screen.findAllByText("测试"))[0]);

    await waitFor(() => {
      expect(mockAdminPost).toHaveBeenCalledWith("/providers/c1/test");
    });
    expect(await screen.findByText(/测试通过 · 3 个模型 · 响应时间 128ms/)).toBeInTheDocument();
  });

  it("surfaces the probe failure detail", async () => {
    const user = userEvent.setup();
    mockAdminGet.mockResolvedValue({ data: seedCredentials() });
    mockAdminPost.mockResolvedValue({ ok: false, ms: 512, error: "HTTP 401: invalid api key" });

    renderWithProviders(<Channels />);

    await user.click((await screen.findAllByText("测试"))[0]);

    expect(await screen.findByText(/测试失败 · HTTP 401: invalid api key/)).toBeInTheDocument();
  });

  it("shows a friendly message when the provider has no active instance", async () => {
    const user = userEvent.setup();
    mockAdminGet.mockResolvedValue({ data: seedCredentials() });
    mockAdminPost.mockResolvedValue({ ok: false, ms: 0, error: "no active instance" });

    renderWithProviders(<Channels />);

    await user.click((await screen.findAllByText("测试"))[0]);

    expect(await screen.findByText(/测试失败 · 无可用实例/)).toBeInTheDocument();
  });

  it("sends custom request headers when creating a provider", async () => {
    const user = userEvent.setup();
    mockAdminGet.mockResolvedValue({ data: [] });
    mockAdminPost.mockResolvedValue({ provider: "deepseek", name: "Headers Provider", status: "active" });

    renderWithProviders(<Channels />);

    await user.click(await screen.findByText("添加渠道"));
    await user.type(screen.getByPlaceholderText(/例如: DeepSeek 深度求索 生产环境/), "Headers Provider");
    await user.type(screen.getByPlaceholderText("sk-..."), "sk-deepseek-1234");
    await user.click(screen.getByText("高级配置"));
    await user.type(
      screen.getByPlaceholderText(/X-Gateway-Id: gw-east-1/),
      "X-Gateway-Id: gw-east-1\nX-Tenant: acme",
    );
    await user.click(screen.getByRole("button", { name: /提交/ }));

    await waitFor(() => {
      expect(mockAdminPost).toHaveBeenCalledWith("/providers", expect.objectContaining({
        name: "Headers Provider",
        custom_headers: { "X-Gateway-Id": "gw-east-1", "X-Tenant": "acme" },
      }));
    });
  });

  it("submits weight and max concurrency from the advanced config", async () => {
    const user = userEvent.setup();
    mockAdminGet.mockResolvedValue({ data: [] });
    mockAdminPost.mockResolvedValue({ provider: "deepseek", name: "Weighted Provider", status: "active" });

    renderWithProviders(<Channels />);

    await user.click(await screen.findByText("添加渠道"));
    await user.type(screen.getByPlaceholderText(/例如: DeepSeek 深度求索 生产环境/), "Weighted Provider");
    await user.type(screen.getByPlaceholderText("sk-..."), "sk-deepseek-1234");
    await user.click(screen.getByText("高级配置"));
    const weightInput = screen.getAllByRole("spinbutton")[1];
    await user.clear(weightInput);
    await user.type(weightInput, "1000");
    await user.click(screen.getByRole("button", { name: /提交/ }));

    await waitFor(() => {
      expect(mockAdminPost).toHaveBeenCalledWith("/providers", expect.objectContaining({
        name: "Weighted Provider",
        weight: 1000,
        max_concurrency: 10,
      }));
    });
  });
});
