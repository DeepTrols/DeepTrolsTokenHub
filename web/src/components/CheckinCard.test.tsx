import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CheckinCard from "./CheckinCard";
import { renderWithProviders } from "../test/test-utils";

vi.mock("../lib/api", () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
  adminApi: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

import { api } from "../lib/api";
const mockApiGet = api.get as ReturnType<typeof vi.fn>;
const mockApiPost = api.post as ReturnType<typeof vi.fn>;

const status = {
  enabled: true,
  min_quota: "1",
  max_quota: "5",
  checked_in_today: false,
  total_days: 3,
};

describe("CheckinCard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApiGet.mockResolvedValue(status);
    mockApiPost.mockResolvedValue({ ok: true, amount: "3" });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders status, reward range and cumulative days", async () => {
    renderWithProviders(<CheckinCard />);

    expect(await screen.findByText("每日签到")).toBeInTheDocument();
    expect(await screen.findByText("每日随机奖励 1 - 5 元")).toBeInTheDocument();
    expect(screen.getByText("累计 3 天")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /签到领奖励/ })).toBeEnabled();
  });

  it("disables the button when already checked in today", async () => {
    mockApiGet.mockResolvedValue({ ...status, checked_in_today: true });

    renderWithProviders(<CheckinCard />);

    expect(await screen.findByText("今日已签到，明天再来吧")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /已签到/ })).toBeDisabled();
  });

  it("posts /checkin on click and refreshes the status query", async () => {
    const user = userEvent.setup();
    renderWithProviders(<CheckinCard />);

    await screen.findByRole("button", { name: /签到领奖励/ });
    await user.click(screen.getByRole("button", { name: /签到领奖励/ }));

    await waitFor(() => {
      expect(mockApiPost).toHaveBeenCalledWith("/checkin", undefined);
    });
    // Success invalidates /wallet and refetches /checkin/status.
    await waitFor(() => {
      expect(mockApiGet).toHaveBeenCalledWith("/checkin/status");
    });
  });

  it("renders the disabled notice when the feature is off", async () => {
    mockApiGet.mockResolvedValue({ ...status, enabled: false });

    renderWithProviders(<CheckinCard />);

    expect(await screen.findByText("签到功能暂未启用")).toBeInTheDocument();
  });
});
