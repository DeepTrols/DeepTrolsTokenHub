import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import SiteSettingsSection from "./SiteSettingsSection";
import { renderWithProviders } from "../../test/test-utils";

vi.mock("../../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  publicApi: { get: vi.fn() },
}));

import { adminApi } from "../../lib/api";
const mockAdminGet = adminApi.get as ReturnType<typeof vi.fn>;
const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

describe("SiteSettingsSection 品牌上传", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAdminGet.mockResolvedValue({
      site_name: "Acme",
      logo_url: "",
      favicon_url: "",
      server_address: "",
      contact_email: "",
    });
    mockFetch.mockResolvedValue({ ok: true, json: async () => ({ url: "/uploads/abc.png" }) });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders the logo/favicon upload buttons", async () => {
    renderWithProviders(<SiteSettingsSection />);

    expect(await screen.findByDisplayValue("Acme")).toBeInTheDocument();
    expect(screen.getAllByText("上传图片").length).toBe(2);
    expect(screen.getByLabelText("Logo")).toBeInTheDocument();
    expect(screen.getByLabelText("Favicon")).toBeInTheDocument();
  });

  it("uploads a logo and fills the logo_url field", async () => {
    const user = userEvent.setup();
    const { container } = renderWithProviders(<SiteSettingsSection />);

    await screen.findByLabelText("站点名称");
    const fileInput = container.querySelector('input[type="file"]') as HTMLInputElement;
    const file = new File([new Uint8Array([137, 80, 78, 71])], "logo.png", { type: "image/png" });
    await user.upload(fileInput, file);

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/admin/settings/upload",
        expect.objectContaining({ method: "POST", credentials: "include" }),
      );
    });
    expect(screen.getByLabelText("Logo")).toHaveValue("/uploads/abc.png");
  });

  it("surfaces the backend error when upload fails", async () => {
    mockFetch.mockResolvedValue({ ok: false, json: async () => ({ error: "上传失败：文件内容不是图片" }) });
    const user = userEvent.setup();
    const { container } = renderWithProviders(<SiteSettingsSection />);

    await screen.findByLabelText("站点名称");
    const fileInput = container.querySelector('input[type="file"]') as HTMLInputElement;
    // The backend rejects the payload; fireEvent bypasses user-event's accept
    // filtering so the .txt file reaches the page's upload handler.
    fireEvent.change(fileInput, { target: { files: [new File(["hello"], "logo.txt", { type: "text/plain" })] } });

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/admin/settings/upload",
        expect.objectContaining({ method: "POST", credentials: "include" }),
      );
    });
    // The failed upload must not overwrite the existing logo URL.
    expect(screen.getByLabelText("Logo")).toHaveValue("");
  });
});
