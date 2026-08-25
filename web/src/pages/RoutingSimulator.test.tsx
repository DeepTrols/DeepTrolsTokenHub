import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import RoutingSimulator from "./RoutingSimulator";

const mockPost = vi.fn();

vi.mock("../lib/api", () => ({
  adminApi: {
    post: (...args: unknown[]) => mockPost(...args),
    get: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  },
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() },
}));

describe("RoutingSimulator", () => {
  beforeEach(() => {
    mockPost.mockReset();
  });

  it("renders the form and calls the simulate endpoint", async () => {
    mockPost.mockResolvedValue({
      data: [{
        channel_id: "c1", channel_name: "DeepSeek 主渠道", health_score: 100,
        health_status: "healthy", strategy: "priority_only", sticky_session: false,
        instance_id: "i1", base_url: "https://api.deepseek.com", upstream_model: "deepseek-chat", current_load: 3,
      }],
    });
    render(<RoutingSimulator />);
    await userEvent.click(screen.getByRole("button", { name: /模拟路由/ }));
    await waitFor(() => expect(mockPost).toHaveBeenCalledWith("/routing/simulate", { model: "deepseek-chat", tenant_id: undefined }));
    expect(await screen.findByText("DeepSeek 主渠道")).toBeInTheDocument();
  });

  it("shows the error message on failure", async () => {
    mockPost.mockRejectedValue(new Error("Model not found in catalog"));
    render(<RoutingSimulator />);
    await userEvent.click(screen.getByRole("button", { name: /模拟路由/ }));
    expect(await screen.findByText("Model not found in catalog")).toBeInTheDocument();
  });
});
