import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import AuditLogs from "./AuditLogs";

const mockGet = vi.fn();

vi.mock("../lib/hooks/use-api", () => ({
  useAdminQuery: (path: string) => ({
    data: mockGet(path), isLoading: false, isError: false, error: null, refetch: vi.fn(),
  }),
}));

describe("AuditLogs", () => {
  beforeEach(() => mockGet.mockReset());

  it("renders audit entries", async () => {
    mockGet.mockReturnValue({ data: [{
      id: "a1", actor_type: "user", actor_email: "admin@test.local", action: "guardrail_blocked",
      resource_type: "guardrail", resource_id: "p1", new_value: { reason_code: "x" }, reason: "", ip_address: "127.0.0.1", created_at: "2026-08-25T10:00:00Z",
    }] });
    render(<AuditLogs />);
    expect(await screen.findByText("admin@test.local")).toBeInTheDocument();
    expect(screen.getByText("guardrail_blocked")).toBeInTheDocument();
  });
});
