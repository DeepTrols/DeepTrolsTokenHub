import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import AdminSubscriptions from "./AdminSubscriptions";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  publicApi: { get: vi.fn() },
}));

import { adminApi } from "../lib/api";
const mockAdminGet = adminApi.get as ReturnType<typeof vi.fn>;
const mockAdminPost = adminApi.post as ReturnType<typeof vi.fn>;

describe("AdminSubscriptions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAdminGet.mockResolvedValue({
      data: [
        {
          id: "s1",
          user_email: "dev@example.com",
          plan_name: "Pro 月付",
          price: "10",
          starts_at: "2026-08-01T00:00:00Z",
          expires_at: "2026-09-01T00:00:00Z",
          status: "active",
        },
        {
          id: "s2",
          user_email: "other@example.com",
          plan_name: "年付",
          price: "100",
          starts_at: "2026-01-01T00:00:00Z",
          expires_at: "2026-06-01T00:00:00Z",
          status: "expired",
        },
      ],
    });
    mockAdminPost.mockResolvedValue({ ok: true });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders subscriptions with user emails and statuses", async () => {
    renderWithProviders(<AdminSubscriptions />);

    expect(await screen.findByText("dev@example.com")).toBeInTheDocument();
    expect(screen.getByText("other@example.com")).toBeInTheDocument();
    expect(screen.getByText("生效中")).toBeInTheDocument();
    expect(screen.getByText("已过期")).toBeInTheDocument();
  });

  it("cancels an active subscription", async () => {
    const user = userEvent.setup();
    renderWithProviders(<AdminSubscriptions />);

    await screen.findByText("dev@example.com");
    await user.click(screen.getAllByRole("button", { name: /取消/ })[0]);

    await waitFor(() => {
      expect(mockAdminPost).toHaveBeenCalledWith("/subscriptions/s1/cancel", { id: "s1" });
    });
  });
});
