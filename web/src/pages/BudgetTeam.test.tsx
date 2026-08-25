import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import BudgetTeam from "./BudgetTeam";

const mockGet = vi.fn();
const mockPost = vi.fn();
const refetch = vi.fn();

vi.mock("../lib/hooks/use-api", () => ({
  useConsoleQuery: () => ({ data: mockGet(), isLoading: false, refetch }),
  useConsoleMutation: () => ({
    mutateAsync: (v: { amount: string; reason: string }) => mockPost(v),
    isPending: false, error: null,
  }),
}));

describe("BudgetTeam", () => {
  beforeEach(() => {
    mockGet.mockReset();
    mockPost.mockReset();
    refetch.mockReset();
    mockGet.mockReturnValue({
      budgets: [{ id: "b1", period: "monthly", limit_amount: "1000", spent_amount: "200", status: "active" }],
      requests: [],
    });
  });

  it("shows the budget and submits an increase request", async () => {
    render(<BudgetTeam />);
    expect(await screen.findByText("1000 CNY")).toBeInTheDocument();
    await userEvent.type(screen.getByPlaceholderText("1000"), "500");
    await userEvent.type(screen.getByPlaceholderText("业务扩容"), "加量");
    await userEvent.click(screen.getByRole("button", { name: /提交申请/ }));
    await waitFor(() => expect(mockPost).toHaveBeenCalledWith({ amount: "500", reason: "加量" }));
  });
});
