import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import SecuritySettingsSection from "./SecuritySettingsSection";
import { renderWithProviders } from "../../test/test-utils";

vi.mock("../../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  publicApi: { get: vi.fn() },
}));

import { adminApi } from "../../lib/api";
const mockAdminGet = adminApi.get as ReturnType<typeof vi.fn>;
const mockAdminPut = adminApi.put as ReturnType<typeof vi.fn>;

describe("SecuritySettingsSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAdminGet.mockResolvedValue({
      register_enabled: "true",
      oauth_github_enabled: "true",
      oauth_github_client_id: "Iv1.abc",
      oauth_github_client_secret: "secret",
      oauth_google_enabled: "true",
      oauth_google_client_id: "goog.apps.googleusercontent.com",
      oauth_google_client_secret: "gsecret",
      oauth_redirect_base_url: "https://console.example.com",
    });
    mockAdminPut.mockResolvedValue({ ok: true });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders the OAuth card with the callback URL hint", async () => {
    renderWithProviders(<SecuritySettingsSection />);

    expect(await screen.findByText("OAuth 登录（GitHub）")).toBeInTheDocument();
    expect(await screen.findByText(/https:\/\/console.example.com\/api\/oauth\/github\/callback/)).toBeInTheDocument();
    expect(screen.getAllByLabelText("Client ID")[0]).toHaveValue("Iv1.abc");
    expect(screen.getAllByLabelText("Client ID")[1]).toHaveValue("goog.apps.googleusercontent.com");
  });

  it("saves the OAuth configuration", async () => {
    const user = userEvent.setup();
    renderWithProviders(<SecuritySettingsSection />);

    await screen.findByText("OAuth 登录（GitHub）");
    await user.click(screen.getAllByRole("button", { name: /保存 OAuth 配置/ })[0]);

    await waitFor(() => {
      expect(mockAdminPut).toHaveBeenCalledWith("/settings/site", {
        oauth_github_enabled: "true",
        oauth_github_client_id: "Iv1.abc",
        oauth_github_client_secret: "secret",
        oauth_redirect_base_url: "https://console.example.com",
      });
    });
  });

  it("toggles OAuth off before saving", async () => {
    const user = userEvent.setup();
    renderWithProviders(<SecuritySettingsSection />);

    await screen.findByText("OAuth 登录（GitHub）");
    await user.click(screen.getAllByRole("switch")[0]);
    await user.click(screen.getAllByRole("button", { name: /保存 OAuth 配置/ })[0]);

    await waitFor(() => {
      expect(mockAdminPut).toHaveBeenCalledWith(
        "/settings/site",
        expect.objectContaining({ oauth_github_enabled: "false" }),
      );
    });
  });

  it("renders the Google OAuth card with its callback URL hint", async () => {
    renderWithProviders(<SecuritySettingsSection />);

    expect(await screen.findByText("OAuth 登录（Google）")).toBeInTheDocument();
    expect(await screen.findByText(/https:\/\/console.example.com\/api\/oauth\/google\/callback/)).toBeInTheDocument();
  });

  it("saves the Google OAuth configuration", async () => {
    const user = userEvent.setup();
    renderWithProviders(<SecuritySettingsSection />);

    await screen.findByText("OAuth 登录（Google）");
    await user.click(screen.getAllByRole("button", { name: /保存 OAuth 配置/ })[1]);

    await waitFor(() => {
      expect(mockAdminPut).toHaveBeenCalledWith("/settings/site", {
        oauth_google_enabled: "true",
        oauth_google_client_id: "goog.apps.googleusercontent.com",
        oauth_google_client_secret: "gsecret",
        oauth_redirect_base_url: "https://console.example.com",
      });
    });
  });
});
