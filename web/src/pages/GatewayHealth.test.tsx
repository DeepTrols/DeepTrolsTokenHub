import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import GatewayHealth from "./GatewayHealth";

const mockGet = vi.fn();

vi.mock("../lib/hooks/use-api", () => ({
  useAdminQuery: () => ({ data: mockGet(), isLoading: false, isError: false, error: null, refetch: vi.fn() }),
}));

describe("GatewayHealth", () => {
  beforeEach(() => mockGet.mockReset());

  it("renders channel health rows", async () => {
    mockGet.mockReturnValue({ data: [{
      channel_id: "c1", channel_name: "主渠道", model_code: "deepseek-chat", pool_type: "shared",
      health_score: 100, health_status: "healthy", channel_status: "active", strategy: "priority_only",
      sticky_session: false, weight: 100, instance_id: "i1", base_url: "https://api.deepseek.com",
      current_load: 3, concurrency_limit: 10, cooldown_until: null,
    }] });
    render(<GatewayHealth />);
    expect(await screen.findByText("主渠道")).toBeInTheDocument();
    expect(screen.getByText("100 · healthy")).toBeInTheDocument();
    expect(screen.getByText("3/10")).toBeInTheDocument();
  });
});
