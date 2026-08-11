import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Providers from "./Providers";
import { renderWithProviders } from "../test/test-utils";

// Mock the api module
vi.mock("../lib/api", () => ({
  adminApi: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  },
  api: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  },
}));

// Mock auth - Providers page is wrapped by RequireAuth (admin)
vi.mock("../lib/auth", () => ({
  useAuth: () => ({
    user: { id: "admin-user", email: "admin@test.com", name: "Admin", role: "admin", status: "active" },
    isLoading: false,
    isAuthenticated: true,
    logout: vi.fn(),
  }),
}));

import { adminApi } from "../lib/api";

const mockGet = adminApi.get as ReturnType<typeof vi.fn>;
const mockPost = adminApi.post as ReturnType<typeof vi.fn>;
const mockPut = adminApi.put as ReturnType<typeof vi.fn>;
const mockDelete = adminApi.delete as ReturnType<typeof vi.fn>;

function createMockProvider(overrides: Record<string, unknown> = {}) {
  return {
    id: "prov-001",
    name: "OpenAI Production",
    provider: "openai",
    base_url: "https://api.openai.com/v1",
    masked_key: "****5678",
    status: "active",
    model_count: 3,
    channel_ids: ["ch-001", "ch-002", "ch-003"],
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-07-27T10:00:00Z",
    ...overrides,
  };
}

describe("Providers", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGet.mockResolvedValue({ data: [], total: 0 });
  });

  it("renders the page title", async () => {
    renderWithProviders(<Providers />);
    expect(screen.getByText("Provider 凭证管理")).toBeInTheDocument();
  });

  it("shows empty state when no providers exist", async () => {
    mockGet.mockResolvedValue({ data: [], total: 0 });
    renderWithProviders(<Providers />);
    await waitFor(() => {
      expect(screen.getByText("暂无凭证")).toBeInTheDocument();
    });
  });

  it("displays a list of providers", async () => {
    const providers = [
      createMockProvider({ id: "p1", name: "OpenAI Prod", provider: "openai" }),
      createMockProvider({ id: "p2", name: "Anthropic Dev", provider: "anthropic", base_url: "https://api.anthropic.com" }),
    ];
    mockGet.mockResolvedValue({ data: providers, total: 2 });

    renderWithProviders(<Providers />);

    await waitFor(() => {
      expect(screen.getByText("OpenAI Prod")).toBeInTheDocument();
      expect(screen.getByText("Anthropic Dev")).toBeInTheDocument();
    });
  });

  it("shows masked API keys", async () => {
    mockGet.mockResolvedValue({ data: [createMockProvider({ masked_key: "****abcd" })], total: 1 });

    renderWithProviders(<Providers />);

    await waitFor(() => {
      expect(screen.getByText("****abcd")).toBeInTheDocument();
    });
  });

  it("shows provider status badge", async () => {
    mockGet.mockResolvedValue({ data: [createMockProvider({ status: "active" })], total: 1 });

    renderWithProviders(<Providers />);

    await waitFor(() => {
      expect(screen.getByText("激活")).toBeInTheDocument();
    });
  });

  it("shows inactive status badge", async () => {
    mockGet.mockResolvedValue({ data: [createMockProvider({ id: "p1", status: "inactive" })], total: 1 });

    renderWithProviders(<Providers />);

    await waitFor(() => {
      expect(screen.getByText("已停用")).toBeInTheDocument();
    });
  });

  it("shows the add provider form when clicking add button", async () => {
    renderWithProviders(<Providers />);

    const addButton = screen.getByRole("button", { name: /添加凭证/ });
    await userEvent.click(addButton);

    expect(screen.getByRole("heading", { name: "添加凭证" })).toBeInTheDocument();
    expect(screen.getByPlaceholderText("例如: OpenAI Production")).toBeInTheDocument();
    expect(screen.getByText("创建")).toBeInTheDocument();
  });

  it("fills and submits the create provider form", async () => {
    mockPost.mockResolvedValue({ id: "new-prov", status: "active" });
    mockGet
      .mockResolvedValueOnce({ data: [], total: 0 })
      .mockResolvedValueOnce({ data: [createMockProvider({ id: "new-prov" })], total: 1 });

    renderWithProviders(<Providers />);

    // Click add button
    await userEvent.click(screen.getByRole("button", { name: /添加凭证/ }));

    // Fill form
    await userEvent.type(screen.getByPlaceholderText("例如: OpenAI Production"), "My Provider");
    // Select provider from dropdown（Radix Select：点击触发后选择选项）
    await userEvent.click(screen.getByRole("combobox"));
    await userEvent.click(await screen.findByText("DeepSeek"));
    await userEvent.type(screen.getByPlaceholderText("sk-..."), "sk-test-key");

    // Submit
    await userEvent.click(screen.getByText("创建"));

    await waitFor(() => {
      expect(mockPost).toHaveBeenCalledWith("/providers", {
        name: "My Provider",
        provider: "deepseek",
        base_url: "",
        api_key: "sk-test-key",
      });
    });
  });

  it("shows error when creating provider without API key", async () => {
    mockGet.mockResolvedValue({ data: [], total: 0 });

    renderWithProviders(<Providers />);

    await userEvent.click(screen.getByRole("button", { name: /添加凭证/ }));
    await userEvent.type(screen.getByPlaceholderText("例如: OpenAI Production"), "My Provider");

    // Submit without API key
    await userEvent.click(screen.getByText("创建"));

    await waitFor(() => {
      expect(screen.getByText("API Key 必填")).toBeInTheDocument();
    });
  });

  it("opens edit form with pre-filled values", async () => {
    const provider = createMockProvider({
      id: "p1",
      name: "OpenAI Prod",
      provider: "openai",
      base_url: "https://api.openai.com/v1",
      masked_key: "****5678",
    });
    mockGet.mockResolvedValue({ data: [provider], total: 1 });

    renderWithProviders(<Providers />);

    await waitFor(() => {
      expect(screen.getByText("OpenAI Prod")).toBeInTheDocument();
    });

    // Click edit button (the pencil icon button)
    const editButton = screen.getByTitle("编辑");
    await userEvent.click(editButton);

    // Form should be in edit mode with pre-filled name
    await waitFor(() => {
      expect(screen.getByText("编辑凭证")).toBeInTheDocument();
    });
  });

  it("deletes a provider with confirmation", async () => {
    const provider = createMockProvider({ id: "p1", name: "To Delete" });
    mockGet.mockResolvedValue({ data: [provider], total: 1 });
    mockDelete.mockResolvedValue({ status: "deleted" });

    // Mock window.confirm
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);

    renderWithProviders(<Providers />);

    await waitFor(() => {
      expect(screen.getByText("To Delete")).toBeInTheDocument();
    });

    // Click delete button
    const deleteButton = screen.getByTitle("停用");
    await userEvent.click(deleteButton);

    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalled();
      expect(mockDelete).toHaveBeenCalledWith("/providers/p1");
    });

    confirmSpy.mockRestore();
  });

  it("does not delete when confirmation is cancelled", async () => {
    const provider = createMockProvider({ id: "p1", name: "Keep Me" });
    mockGet.mockResolvedValue({ data: [provider], total: 1 });

    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(false);

    renderWithProviders(<Providers />);

    await waitFor(() => {
      expect(screen.getByText("Keep Me")).toBeInTheDocument();
    });

    const deleteButton = screen.getByTitle("停用");
    await userEvent.click(deleteButton);

    expect(confirmSpy).toHaveBeenCalled();
    expect(mockDelete).not.toHaveBeenCalled();

    confirmSpy.mockRestore();
  });

  it("cancels the form and closes it", async () => {
    mockGet.mockResolvedValue({ data: [], total: 0 });

    renderWithProviders(<Providers />);

    await userEvent.click(screen.getByRole("button", { name: /添加凭证/ }));

    // Form should be visible
    expect(screen.getByText("创建")).toBeInTheDocument();

    // Click cancel
    await userEvent.click(screen.getByText("取消"));

    // Form should be hidden
    await waitFor(() => {
      expect(screen.queryByText("创建")).not.toBeInTheDocument();
    });
  });
});
