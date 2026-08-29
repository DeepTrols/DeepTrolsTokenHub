import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Chat from "./Chat";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  publicApi: { get: vi.fn() },
}));

vi.mock("../lib/site", () => ({
  useSiteInfo: () => ({
    site: {
      site_name: "DeepTrols",
      server_address: "https://api.example.com",
      logo_url: "",
      favicon_url: "",
      footer_text: "",
      notice: "",
      about: "",
      home_page_content: "",
      contact_email: "",
      legal: { user_agreement: "", privacy_policy: "" },
    },
    isLoading: false,
  }),
}));

import { api } from "../lib/api";
const mockApiGet = api.get as ReturnType<typeof vi.fn>;

describe("Chat", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApiGet.mockImplementation((path: string) => {
      if (path === "/chat/presets") {
        return Promise.resolve([{ "Cherry Studio": "https://chat.cherry-ai.com/?api_key={key}" }]);
      }
      if (path === "/api-keys") {
        return Promise.resolve({ data: [{ id: "key-1", name: "Prod", status: "active" }] });
      }
      if (path === "/api-keys/key-1/secret") {
        return Promise.resolve({ plaintext: "sk-plaintext123" });
      }
      return Promise.resolve(null);
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders preset cards from the console endpoint", async () => {
    renderWithProviders(<Chat />);

    expect(await screen.findByText("Cherry Studio")).toBeInTheDocument();
    expect(screen.getByText("网页版")).toBeInTheDocument();
    expect(screen.getAllByText(/自动注入 API Key/).length).toBeGreaterThan(0);
  });

  it("opens a web preset in an iframe with the resolved URL", async () => {
    const user = userEvent.setup();
    renderWithProviders(<Chat />);

    await user.click(await screen.findByText("Cherry Studio"));

    await waitFor(() => {
      const iframe = document.querySelector("iframe") as HTMLIFrameElement | null;
      expect(iframe?.src).toContain("api_key=sk-plaintext123");
    });
  });

  it("shows the empty state when no presets are configured", async () => {
    mockApiGet.mockImplementation((path: string) => {
      if (path === "/chat/presets") return Promise.resolve([]);
      return Promise.resolve(null);
    });

    renderWithProviders(<Chat />);

    expect(await screen.findByText("暂无聊天预设")).toBeInTheDocument();
  });
});
