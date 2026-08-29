import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ModelsSettingsSection from "./ModelsSettingsSection";
import { renderWithProviders } from "../../test/test-utils";

vi.mock("../../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  publicApi: { get: vi.fn() },
}));

import { adminApi } from "../../lib/api";
const mockAdminGet = adminApi.get as ReturnType<typeof vi.fn>;
const mockAdminPut = adminApi.put as ReturnType<typeof vi.fn>;

describe("ModelsSettingsSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAdminGet.mockResolvedValue({
      models_public_visible: "true",
      new_model_default_status: "active",
    });
    mockAdminPut.mockResolvedValue({ ok: true });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders the model settings and saves changes", async () => {
    const user = userEvent.setup();
    renderWithProviders(<ModelsSettingsSection />);

    expect(await screen.findByText("目录与同步")).toBeInTheDocument();
    await user.click(screen.getByRole("switch"));
    await user.click(screen.getByRole("button", { name: /保存/ }));

    await waitFor(() => {
      expect(mockAdminPut).toHaveBeenCalledWith("/settings/site", {
        models_public_visible: "false",
        new_model_default_status: "active",
      });
    });
  });
});
