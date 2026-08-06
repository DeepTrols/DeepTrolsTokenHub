import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen } from "@testing-library/react";
import Channels from "./Channels";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

vi.mock("../lib/auth", () => ({
  useAuth: () => ({
    user: { id: "admin", email: "admin@test.com", name: "Admin", role: "admin", status: "active", totp_enabled: false },
    isLoading: false, isAuthenticated: true, logout: vi.fn(),
  }),
}));

import { adminApi } from "../lib/api";
const mockAdminGet = adminApi.get as ReturnType<typeof vi.fn>;

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
});
