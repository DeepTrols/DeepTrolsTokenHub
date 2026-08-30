import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ProviderSyncDialog } from "./ProviderSyncDialog";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { adminApi } from "../lib/api";
const mockAdminGet = adminApi.get as ReturnType<typeof vi.fn>;
const mockAdminPost = adminApi.post as ReturnType<typeof vi.fn>;

const PREVIEW = {
  models: [
    { upstream: "deepseek-v4-flash", code: "deepseek-v4-flash", model_id: "", status: "new", enabled: false },
    { upstream: "deepseek-v4-pro", code: "deepseek-v4-pro", model_id: "m1", status: "bound", enabled: true },
    { upstream: "legacy-model", code: "legacy-model", model_id: "m2", status: "disabled", enabled: false },
  ],
};

describe("ProviderSyncDialog", () => {
  beforeEach(() => { vi.clearAllMocks(); });
  afterEach(() => { vi.restoreAllMocks(); });

  it("renders model names grouped by status with correct badges", async () => {
    mockAdminGet.mockResolvedValue(PREVIEW);
    renderWithProviders(
      <ProviderSyncDialog open providerId="c1" onClose={vi.fn()} onSynced={vi.fn()} />,
    );

    expect(await screen.findByText("deepseek-v4-flash")).toBeInTheDocument();
    expect(screen.getByText("deepseek-v4-pro")).toBeInTheDocument();
    expect(screen.getByText("legacy-model")).toBeInTheDocument();
    expect(screen.getByText("新增模型 (1)")).toBeInTheDocument();
    expect(screen.getByText("已绑定 (1)")).toBeInTheDocument();
    expect(screen.getByText("已停用 (1)")).toBeInTheDocument();
    expect(screen.getByText("1 个新模型将自动创建（默认价格 0，需后补）")).toBeInTheDocument();
  });

  it("applies sync with selected model_ids and auto_create flag", async () => {
    const user = userEvent.setup();
    mockAdminGet.mockResolvedValue(PREVIEW);
    mockAdminPost.mockResolvedValue({ applied: 2, created: 1, skipped: 0 });

    renderWithProviders(
      <ProviderSyncDialog open providerId="c1" onClose={vi.fn()} onSynced={vi.fn()} />,
    );
    await screen.findByText("deepseek-v4-flash");

    // new + bound are checked by default; disabled is unchecked.
    const applyBtn = screen.getByRole("button", { name: "应用同步" });
    expect(applyBtn).toBeEnabled();
    await user.click(applyBtn);

    await waitFor(() => {
      expect(mockAdminPost).toHaveBeenCalledWith(
        "/channels/c1/models/sync",
        expect.objectContaining({ model_ids: ["deepseek-v4-flash", "deepseek-v4-pro"], auto_create: false }),
      );
    });
  });

  it("disables apply when nothing is selected", async () => {
    const user = userEvent.setup();
    mockAdminGet.mockResolvedValue({
      models: [{ upstream: "deepseek-v4-flash", code: "deepseek-v4-flash", model_id: "", status: "new", enabled: false }],
    });

    renderWithProviders(
      <ProviderSyncDialog open providerId="c1" onClose={vi.fn()} onSynced={vi.fn()} />,
    );
    await screen.findByText("deepseek-v4-flash");

    const checkbox = screen.getAllByRole("checkbox")[0];
    await user.click(checkbox);
    expect(screen.getByRole("button", { name: "应用同步" })).toBeDisabled();
  });
});
