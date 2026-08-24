import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import APIKeys, { gmt8DateTime } from "./APIKeys";
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
const mockApiPost = api.post as ReturnType<typeof vi.fn>;
const mockApiPut = api.put as ReturnType<typeof vi.fn>;
const mockApiDelete = api.delete as ReturnType<typeof vi.fn>;

function createMockKey(overrides: Record<string, unknown> = {}) {
  return {
    id: "key-001",
    name: "sso_default",
    masked_key: "dt-sk-aw6wG****neDQ",
    key_prefix: "dt-sk-",
    status: "active",
    allowed_models: ["deepseek-v4-flash"],
    source_whitelist: [],
    monthly_limit: "",
    weekly_limit: "",
    cumulative_limit: "",
    over_limit_action: "block",
    last_used_at: "2026-08-20T03:55:13Z",
    last_7d_active: true,
    created_at: "2026-08-20T03:55:13Z",
    ...overrides,
  };
}

function mockListKeys(keys: unknown[]) {
  mockApiGet.mockImplementation((path: string) => {
    if (path === "/api-keys") return Promise.resolve({ data: keys });
    if (path === "/api-keys/key-001/secret") return Promise.resolve({ plaintext: "dt-sk-plain-secret-1234" });
    return Promise.resolve({});
  });
}

describe("APIKeys（API keys）", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders the page title and security notice", async () => {
    mockListKeys([]);

    renderWithProviders(<APIKeys />);

    expect(screen.getByText("API keys")).toBeInTheDocument();
    expect(
      screen.getByText(/列表内是你的全部 API key，API key 请妥善保存/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/不要与他人共享你的 API key/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/自动禁用我们发现已公开泄露的 API key/),
    ).toBeInTheDocument();
  });

  it("fetches API keys on mount and renders table headers", async () => {
    mockListKeys([createMockKey()]);

    renderWithProviders(<APIKeys />);

    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledWith("/api-keys");
    });
    await screen.findByText("sso_default");
    for (const header of ["名称", "key", "创建日期", "最新使用日期", "操作"]) {
      expect(screen.getByText(header)).toBeInTheDocument();
    }
  });

  it("renders one row with masked key, dates and link actions", async () => {
    mockListKeys([createMockKey()]);

    renderWithProviders(<APIKeys />);

    expect(await screen.findByText("sso_default")).toBeInTheDocument();
    expect(screen.getByText("dt-sk-aw6wG****neDQ")).toBeInTheDocument();
    // 2026-08-20T03:55:13Z → GMT+8 2026-08-20 11:55:13
    // 创建日期与最新使用日期在 mock 中相同，因此出现两次
    expect(screen.getAllByText("2026-08-20 11:55:13")).toHaveLength(2);
    expect(screen.getByText("查看key")).toBeInTheDocument();
    expect(screen.getByText("编辑")).toBeInTheDocument();
    expect(screen.getByText("删除")).toBeInTheDocument();
  });

  it('shows "从未使用" when the key has never been used', async () => {
    mockListKeys([createMockKey({ last_used_at: "" })]);

    renderWithProviders(<APIKeys />);

    expect(await screen.findByText("sso_default")).toBeInTheDocument();
    expect(screen.getByText("从未使用")).toBeInTheDocument();
  });

  it("shows empty state when there are no keys", async () => {
    mockListKeys([]);

    renderWithProviders(<APIKeys />);

    expect(await screen.findByText("暂无 API key")).toBeInTheDocument();
  });

  it("creates a key through the create dialog", async () => {
    const user = userEvent.setup();
    mockListKeys([]);
    mockApiPost.mockResolvedValueOnce({ id: "new-1", plaintext: "dt-sk-new-secret", warning: "只显示一次" });

    renderWithProviders(<APIKeys />);

    await user.click(screen.getByRole("button", { name: /创建密钥/ }));
    await user.type(screen.getByPlaceholderText("例如：生产环境、测试环境"), "Production Key");
    await user.click(screen.getByRole("button", { name: /确认创建/ }));

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith("/api-keys", {
        name: "Production Key",
        over_limit_action: "block",
      });
    });
    expect(await screen.findByText("dt-sk-new-secret")).toBeInTheDocument();
  });

  it("reveals the plaintext when 查看key is clicked", async () => {
    const user = userEvent.setup();
    mockListKeys([createMockKey()]);

    renderWithProviders(<APIKeys />);

    await user.click(await screen.findByText("查看key"));

    expect(await screen.findByText("dt-sk-plain-secret-1234")).toBeInTheDocument();
    expect(mockApiGet).toHaveBeenCalledWith("/api-keys/key-001/secret");
  });

  it("edits a key through the edit dialog", async () => {
    const user = userEvent.setup();
    mockListKeys([
      createMockKey({
        allowed_models: ["deepseek-v4-flash", "gpt-4o"],
        monthly_limit: "500",
        weekly_limit: "200",
        cumulative_limit: "5000",
        over_limit_action: "warn",
      }),
    ]);
    mockApiPut.mockResolvedValue({ status: "updated", id: "key-001" });

    renderWithProviders(<APIKeys />);

    await user.click(await screen.findByText("编辑"));
    const nameInput = screen.getByPlaceholderText("例如：生产环境、测试环境");
    await user.clear(nameInput);
    await user.type(nameInput, "Renamed Key");
    await user.click(screen.getByRole("button", { name: /保存更改/ }));

    await waitFor(() => {
      expect(mockApiPut).toHaveBeenCalledWith("/api-keys/key-001", {
        id: "key-001",
        name: "Renamed Key",
        allowed_models: ["deepseek-v4-flash", "gpt-4o"],
        source_whitelist: [],
        monthly_limit: "500",
        weekly_limit: "200",
        cumulative_limit: "5000",
        over_limit_action: "warn",
        status: "active",
      });
    });
  });

  it("deletes a key after confirmation", async () => {
    const user = userEvent.setup();
    vi.spyOn(window, "confirm").mockReturnValue(true);
    mockListKeys([createMockKey()]);
    mockApiDelete.mockResolvedValue({ status: "deleted", id: "key-001" });

    renderWithProviders(<APIKeys />);

    await user.click(await screen.findByText("删除"));

    await waitFor(() => {
      expect(mockApiDelete).toHaveBeenCalledWith("/api-keys/key-001");
    });
  });

  it("shows an error state with retry when the list fails", async () => {
    mockApiGet.mockRejectedValue(new Error("network down"));

    renderWithProviders(<APIKeys />);

    expect(await screen.findByText("加载失败")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });
});

describe("gmt8DateTime", () => {
  it("formats an instant as GMT+8 date time", () => {
    expect(gmt8DateTime("2026-08-20T03:55:13Z")).toBe("2026-08-20 11:55:13");
    expect(gmt8DateTime("2026-08-20T17:30:00Z")).toBe("2026-08-21 01:30:00");
  });

  it("falls back for invalid input", () => {
    expect(gmt8DateTime("not-a-date")).toBe("—");
  });
});
