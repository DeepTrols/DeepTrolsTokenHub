import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Guardrails from "./Guardrails";

const mockGet = vi.fn();
const mockPost = vi.fn();
const mockDelete = vi.fn();

vi.mock("../lib/hooks/use-api", () => ({
  useAdminQuery: (path: string) => ({ data: mockGet(path), isLoading: false, isError: false, error: null, refetch: vi.fn() }),
  useAdminMutation: () => ({
    mutateAsync: vi.fn(), isPending: false, error: null,
  }),
}));

describe("Guardrails", () => {
  beforeEach(() => {
    mockGet.mockReset();
    mockPost.mockReset();
    mockDelete.mockReset();
  });

  it("renders the policy list from the API", async () => {
    mockGet.mockReturnValue({ data: [{ id: "p1", name: "敏感词拦截", description: "", status: "active",
      detection_items: [{ id: "i1", name: "keyword", detector_type: "pattern", action: "block" }],
      bindings: [{ id: "b1", scope_type: "all_projects", scope_id: "", checkpoint: "before_provider", protocol: "all" }] }] });
    render(<Guardrails />);
    expect(await screen.findByText("敏感词拦截")).toBeInTheDocument();
    expect(screen.getByText("启用")).toBeInTheDocument();
  });

  it("opens the create dialog", async () => {
    mockGet.mockReturnValue({ data: [] });
    render(<Guardrails />);
    await userEvent.click(screen.getByRole("button", { name: /新建策略/ }));
    expect(await screen.findByText("配置检测项（关键词用逗号分隔）与绑定范围")).toBeInTheDocument();
  });
});
