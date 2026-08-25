import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import BudgetAdmin from "./BudgetAdmin";

const mockGet = vi.fn();
const mockPost = vi.fn();

vi.mock("../lib/hooks/use-api", () => ({
  useAdminQuery: (path: string) => ({
    data: mockGet(path), isLoading: false, isError: false, error: null,
    refetch: vi.fn(),
  }),
  useAdminMutation: (method: string, path: (v: { id: string }) => string) => ({
    mutateAsync: (v: { id: string }) => mockPost(method, path(v)),
    isPending: false, error: null,
  }),
}));

describe("BudgetAdmin", () => {
  beforeEach(() => {
    mockGet.mockReset();
    mockPost.mockReset();
    mockGet.mockImplementation((path: string) =>
      path === "/budgets"
        ? { data: [{ id: "b1", tenant_name: "Acme", period: "monthly", limit_amount: "1000", spent_amount: "200", status: "active" }] }
        : { data: [{ id: "r1", tenant_name: "Acme", requested_amount: "500", reason: "扩容", status: "pending", created_at: "2026-08-25T10:00:00Z" }] },
    );
  });

  it("renders budgets and pending requests with approve action", async () => {
    render(<BudgetAdmin />);
    const acme = await screen.findAllByText("Acme");
    expect(acme.length).toBeGreaterThanOrEqual(2);
    await userEvent.click(screen.getByRole("button", { name: /通过/ }));
    expect(mockPost).toHaveBeenCalledWith("post", "/budgets/requests/r1/approve");
  });
});
