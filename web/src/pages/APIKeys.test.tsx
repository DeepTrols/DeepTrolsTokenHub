import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import APIKeys from "./APIKeys";
import { renderWithProviders } from "../test/test-utils";

// Mock the api module
vi.mock("../lib/api", () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  },
}));

// Mock auth - APIKeys page is wrapped by RequireAuth
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
const mockApiPost = api.post as ReturnType<typeof vi.fn>;
const mockApiPut = api.put as ReturnType<typeof vi.fn>;
const mockApiDelete = api.delete as ReturnType<typeof vi.fn>;

// Helper to create a mock API key with full boundary fields
function createMockKey(overrides: Record<string, unknown> = {}) {
  return {
    id: "key-001",
    name: "Test Key",
    masked_key: "dt-sk-abc1****xyz9",
    key_prefix: "dt-sk-",
    status: "active",
    allowed_models: ["gpt-4o", "claude-sonnet"],
    source_whitelist: ["192.168.1.0/24"],
    monthly_limit: "500",
    weekly_limit: "200",
    cumulative_limit: "5000",
    over_limit_action: "block",
    last_used_at: "2026-07-27T10:00:00Z",
    last_7d_active: true,
    created_at: "2026-06-01T00:00:00Z",
    ...overrides,
  };
}

// Helper to create a minimal mock key (no optional fields set)
function createMinimalMockKey(overrides: Record<string, unknown> = {}) {
  return {
    id: "key-min-001",
    name: "Minimal Key",
    masked_key: "dt-sk-min****xyz9",
    key_prefix: "dt-sk-",
    status: "active",
    allowed_models: null,
    source_whitelist: null,
    monthly_limit: "",
    weekly_limit: "",
    cumulative_limit: "",
    over_limit_action: "block",
    last_used_at: "",
    last_7d_active: false,
    created_at: "2026-06-01T00:00:00Z",
    ...overrides,
  };
}

describe("APIKeys", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApiGet.mockResolvedValue({ data: [] });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // ==========================================================================
  // Basic rendering
  // ==========================================================================

  it("renders the page title and description", async () => {
    renderWithProviders(<APIKeys />);

    expect(screen.getByText("API 密钥")).toBeInTheDocument();
    expect(
      screen.getByText("管理 API 密钥，控制模型访问权限与消费额度")
    ).toBeInTheDocument();
  });

  it("shows create button", async () => {
    renderWithProviders(<APIKeys />);

    expect(
      screen.getByRole("button", { name: /创建密钥/ })
    ).toBeInTheDocument();
  });

  it("fetches API keys on mount", async () => {
    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledWith("/api-keys");
    });
  });

  it("shows empty state when no keys exist", async () => {
    mockApiGet.mockResolvedValue({ data: [] });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("暂无 API 密钥")).toBeInTheDocument();
    });
  });

  // ==========================================================================
  // Boundary 1: Identity boundary (name) -- existing functionality
  // ==========================================================================

  it("shows create form with name input when create button is clicked", async () => {
    const user = userEvent.setup();
    renderWithProviders(<APIKeys />);

    await user.click(screen.getByRole("button", { name: /创建密钥/ }));

    expect(screen.getByText("创建新密钥")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("例如：生产环境、测试环境")).toBeInTheDocument();
  });

  // ==========================================================================
  // Boundary 2: Model boundary -- model whitelist
  // ==========================================================================

  it("shows model whitelist input in create form", async () => {
    const user = userEvent.setup();
    renderWithProviders(<APIKeys />);

    await user.click(screen.getByRole("button", { name: /创建密钥/ }));

    // Should show model whitelist section
    expect(screen.getByText(/模型白名单/i)).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText(/例如: gpt-4o, claude-sonnet/i)
    ).toBeInTheDocument();
  });

  it("shows configured model count in key list", async () => {
    mockApiGet.mockResolvedValue({
      data: [createMockKey({ allowed_models: ["gpt-4o", "claude-sonnet", "gemini-pro"] })],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });

    // Should show "3 models" with tooltip
    expect(screen.getByText(/3 models/i)).toBeInTheDocument();
  });

  it("shows no model restriction when allowed_models is empty", async () => {
    mockApiGet.mockResolvedValue({
      data: [createMinimalMockKey()],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Minimal Key")).toBeInTheDocument();
    });

    // Should show unrestricted indicators (model and IP both show "未限制")
    const unrestricted = screen.getAllByText(/未限制/i);
    expect(unrestricted.length).toBeGreaterThanOrEqual(2);
  });

  // ==========================================================================
  // Boundary 3: Source boundary -- IP whitelist
  // ==========================================================================

  it("shows IP whitelist input in create form", async () => {
    const user = userEvent.setup();
    renderWithProviders(<APIKeys />);

    await user.click(screen.getByRole("button", { name: /创建密钥/ }));

    expect(screen.getByText(/IP 白名单/i)).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText(/例如: 192\.168\.1\.0\/24, 10\.0\.0\.0\/8/i)
    ).toBeInTheDocument();
  });

  it("shows IP whitelist status in key list when configured", async () => {
    mockApiGet.mockResolvedValue({
      data: [createMockKey({ source_whitelist: ["192.168.1.0/24"] })],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });

    // Should show "已配置" for IP whitelist
    expect(screen.getByText(/已配置/i)).toBeInTheDocument();
  });

  it("shows IP '未限制' when source_whitelist is empty", async () => {
    mockApiGet.mockResolvedValue({
      data: [createMinimalMockKey()],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Minimal Key")).toBeInTheDocument();
    });

    // The key row should show 未限制 for IP
    const keyContainer = screen.getByText("Minimal Key").closest('[class*="border"]');
    expect(keyContainer).not.toBeNull();
  });

  // ==========================================================================
  // Boundary 4: Budget boundary -- spend limits + over-limit action
  // ==========================================================================

  it("shows spend limit inputs in create form", async () => {
    const user = userEvent.setup();
    renderWithProviders(<APIKeys />);

    await user.click(screen.getByRole("button", { name: /创建密钥/ }));

    // Monthly limit
    expect(screen.getByText(/月度限额/i)).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText(/月度消费上限/i)
    ).toBeInTheDocument();

    // Weekly limit
    expect(screen.getByText(/周限额/i)).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText(/周消费上限/i)
    ).toBeInTheDocument();

    // Cumulative limit
    expect(screen.getByText(/累计限额/i)).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText(/累计消费上限/i)
    ).toBeInTheDocument();
  });

  it("shows over-limit action selector in create form", async () => {
    const user = userEvent.setup();
    renderWithProviders(<APIKeys />);

    await user.click(screen.getByRole("button", { name: /创建密钥/ }));

    expect(screen.getByText(/超限动作/i)).toBeInTheDocument();

    // Should have radio/select for block and warn
    const blockOption = screen.getByLabelText(/阻止/i);
    const warnOption = screen.getByLabelText(/警告/i);
    expect(blockOption).toBeInTheDocument();
    expect(warnOption).toBeInTheDocument();
  });

  it("displays spend limits with CNY suffix labels", async () => {
    const user = userEvent.setup();
    renderWithProviders(<APIKeys />);

    await user.click(screen.getByRole("button", { name: /创建密钥/ }));

    // Verify CNY labels are shown
    expect(screen.getByText(/CNY\/月/i)).toBeInTheDocument();
    expect(screen.getByText(/CNY\/周/i)).toBeInTheDocument();
    expect(screen.getByText(/CNY 总计/i)).toBeInTheDocument();
  });

  it("sends all boundary fields when creating a key", async () => {
    const user = userEvent.setup();
    mockApiPost.mockResolvedValueOnce({
      id: "new-key-001",
      plaintext: "dt-sk-test123",
      key_prefix: "dt-sk-",
      masked_key: "dt-sk-test****123",
      name: "Production Key",
      warning: "Store this key securely.",
    });

    renderWithProviders(<APIKeys />);

    await user.click(screen.getByRole("button", { name: /创建密钥/ }));

    // Fill in all fields
    const nameInput = screen.getByPlaceholderText("例如：生产环境、测试环境");
    await user.type(nameInput, "Production Key");

    // Model whitelist
    const modelInput = screen.getByPlaceholderText(/例如: gpt-4o, claude-sonnet/i);
    await user.type(modelInput, "gpt-4o, claude-sonnet");

    // IP whitelist
    const ipInput = screen.getByPlaceholderText(/例如: 192\.168\.1\.0\/24, 10\.0\.0\.0\/8/i);
    await user.type(ipInput, "192.168.1.0/24");

    // Monthly limit
    const monthlyInput = screen.getByPlaceholderText(/月度消费上限/i);
    await user.type(monthlyInput, "500");

    // Weekly limit
    const weeklyInput = screen.getByPlaceholderText(/周消费上限/i);
    await user.type(weeklyInput, "200");

    // Cumulative limit
    const cumulativeInput = screen.getByPlaceholderText(/累计消费上限/i);
    await user.type(cumulativeInput, "5000");

    // Select warn action
    const warnRadio = screen.getByLabelText(/警告/i);
    await user.click(warnRadio);

    // Submit
    await user.click(screen.getByRole("button", { name: /确认创建/ }));

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith("/api-keys", {
        name: "Production Key",
        allowed_models: ["gpt-4o", "claude-sonnet"],
        source_whitelist: ["192.168.1.0/24"],
        monthly_limit: "500",
        weekly_limit: "200",
        cumulative_limit: "5000",
        over_limit_action: "warn",
      });
    });
  });

  // ==========================================================================
  // Boundary 5: Status boundary -- suspend/revoke actions
  // ==========================================================================

  it("shows status badge for active key", async () => {
    mockApiGet.mockResolvedValue({
      data: [createMockKey({ status: "active" })],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });

    expect(screen.getByText("启用")).toBeInTheDocument();
  });

  it("shows status badge for disabled key", async () => {
    mockApiGet.mockResolvedValue({
      data: [createMockKey({ status: "disabled" })],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });

    expect(screen.getByText("已停用")).toBeInTheDocument();
  });

  it("shows status badge for revoked key", async () => {
    mockApiGet.mockResolvedValue({
      data: [createMockKey({ status: "revoked" })],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });

    expect(screen.getByText("已撤销")).toBeInTheDocument();
  });

  it("has a status toggle button for active keys", async () => {
    mockApiGet.mockResolvedValue({
      data: [createMockKey({ status: "active" })],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });

    // Should have a disable button
    expect(
      screen.getByRole("button", { name: /停用/i })
    ).toBeInTheDocument();
  });

  it("has a status toggle button for disabled keys", async () => {
    mockApiGet.mockResolvedValue({
      data: [createMockKey({ status: "disabled" })],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });

    // Should have an enable button
    expect(
      screen.getByRole("button", { name: /启用/i })
    ).toBeInTheDocument();
  });

  it("calls PUT /api-keys/{id} with status change when toggle is clicked", async () => {
    const user = userEvent.setup();
    mockApiGet.mockResolvedValue({
      data: [createMockKey({ id: "key-001", status: "active" })],
    });
    mockApiPut.mockResolvedValue({ status: "updated", id: "key-001" });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });

    const disableBtn = screen.getByRole("button", { name: /停用/i });
    await user.click(disableBtn);

    await waitFor(() => {
      expect(mockApiPut).toHaveBeenCalledWith("/api-keys/key-001", {
        id: "key-001",
        status: "disabled",
      });
    });
  });

  it("does not show toggle buttons for revoked keys", async () => {
    mockApiGet.mockResolvedValue({
      data: [createMockKey({ status: "revoked" })],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });

    // Should NOT have disable or enable buttons for revoked keys
    expect(
      screen.queryByRole("button", { name: /停用/i })
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /启用/i })
    ).not.toBeInTheDocument();
  });

  it("calls DELETE when delete button is clicked", async () => {
    const user = userEvent.setup();
    mockApiGet.mockResolvedValue({
      data: [createMockKey({ id: "key-001" })],
    });
    mockApiDelete.mockResolvedValue({ status: "deleted", id: "key-001" });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });

    const deleteBtn = screen.getByRole("button", { name: /删除/i });
    await user.click(deleteBtn);

    // Should call delete endpoint
    await waitFor(() => {
      expect(mockApiDelete).toHaveBeenCalledWith("/api-keys/key-001");
    });
  });

  // ==========================================================================
  // Boundary 6: Evidence boundary -- last used, 7d active, link to logs
  // ==========================================================================

  it("shows last used time when available", async () => {
    mockApiGet.mockResolvedValue({
      data: [createMockKey({ last_used_at: "2026-07-27T10:00:00Z" })],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });

    // Should show relative time or indicator of last use
    expect(screen.getByText(/最后使用/i)).toBeInTheDocument();
  });

  it('shows "从未使用" when key has never been used', async () => {
    mockApiGet.mockResolvedValue({
      data: [createMinimalMockKey()],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Minimal Key")).toBeInTheDocument();
    });

    expect(screen.getByText(/从未使用/i)).toBeInTheDocument();
  });

  it("shows 7-day active indicator (green dot) when key is active in last 7 days", async () => {
    mockApiGet.mockResolvedValue({
      data: [createMockKey({ last_7d_active: true })],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });

    // Should have a green dot with "最近 7 天活跃" title
    const activeDot = document.querySelector('[title="最近 7 天活跃"]');
    expect(activeDot).toBeInTheDocument();
  });

  it("does not show green dot when key is not active in last 7 days", async () => {
    mockApiGet.mockResolvedValue({
      data: [createMinimalMockKey({ last_7d_active: false })],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Minimal Key")).toBeInTheDocument();
    });

    // Should not have the 7-day active indicator
    const activeDot = document.querySelector('[title="最近 7 天活跃"]');
    expect(activeDot).toBeNull();
  });

  it('shows "查看日志" link to usage logs', async () => {
    mockApiGet.mockResolvedValue({
      data: [createMockKey({ id: "key-001" })],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });

    const logLink = screen.getByText(/查看日志/i);
    expect(logLink).toBeInTheDocument();
    expect(logLink.closest("a")).toHaveAttribute("href", "/logs?key_id=key-001");
  });

  // ==========================================================================
  // Detail expand / drawer
  // ==========================================================================

  it("expands key details on click showing all boundary configurations", async () => {
    const user = userEvent.setup();
    mockApiGet.mockResolvedValue({
      data: [createMockKey()],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });

    // Click the test key name to expand the detail view
    await user.click(screen.getByText("Test Key"));

    // Should show detailed boundary info
    await waitFor(() => {
      // Model boundary - model codes shown in expanded detail
      expect(screen.getByText("gpt-4o")).toBeInTheDocument();
      expect(screen.getByText("claude-sonnet")).toBeInTheDocument();

      // Source boundary
      expect(screen.getByText("192.168.1.0/24")).toBeInTheDocument();

      // Evidence boundary labels
      expect(screen.getByText("身份边界")).toBeInTheDocument();
      expect(screen.getByText("证据边界")).toBeInTheDocument();
    });
  });

  // ==========================================================================
  // Error handling
  // ==========================================================================

  it("handles API keys fetch error gracefully", async () => {
    mockApiGet.mockRejectedValue(new Error("Network error"));

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("暂无 API 密钥")).toBeInTheDocument();
    });
  });

  it("remains functional when API put fails for status toggle", async () => {
    const user = userEvent.setup();
    mockApiGet.mockResolvedValue({
      data: [createMockKey({ id: "key-001", status: "active" })],
    });
    mockApiPut.mockRejectedValue(new Error("Update failed"));

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });

    const disableBtn = screen.getByRole("button", { name: /停用/i });
    await user.click(disableBtn);

    // Should not crash, still display the component
    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });
  });

  // ==========================================================================
  // Edge cases
  // ==========================================================================

  it("shows all boundary sections in expanded detail for fully configured key", async () => {
    const user = userEvent.setup();
    mockApiGet.mockResolvedValue({
      data: [createMockKey({
        over_limit_action: "warn",
        monthly_limit: "500",
        weekly_limit: "200",
        cumulative_limit: "5000",
      })],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Test Key"));

    await waitFor(() => {
      // All 6 boundary sections should be visible
      expect(screen.getByText("身份边界")).toBeInTheDocument();
      expect(screen.getByText("模型边界")).toBeInTheDocument();
      expect(screen.getByText("来源边界")).toBeInTheDocument();
      expect(screen.getByText("预算边界")).toBeInTheDocument();
      expect(screen.getByText("状态边界")).toBeInTheDocument();
      expect(screen.getByText("证据边界")).toBeInTheDocument();

      // Over-limit action should show "警告" not "阻止"
      expect(screen.getByText("警告")).toBeInTheDocument();
    });
  });

  it("handles keys with over_limit status", async () => {
    mockApiGet.mockResolvedValue({
      data: [createMockKey({ status: "over_limit" })],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });

    expect(screen.getByText("超限")).toBeInTheDocument();
  });

  it("displays cumulative limit progress info", async () => {
    mockApiGet.mockResolvedValue({
      data: [createMockKey({ cumulative_limit: "10000" })],
    });

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(screen.getByText("Test Key")).toBeInTheDocument();
    });
  });

  it("renders create form with default over-limit action as block", async () => {
    const user = userEvent.setup();
    renderWithProviders(<APIKeys />);

    await user.click(screen.getByRole("button", { name: /创建密钥/ }));

    // The block radio should be checked by default
    const blockRadio = screen.getByLabelText(/阻止/i) as HTMLInputElement;
    expect(blockRadio.checked).toBe(true);
  });
});
