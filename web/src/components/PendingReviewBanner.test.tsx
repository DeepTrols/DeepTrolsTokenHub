import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { PendingReviewBanner } from "./PendingReviewBanner";

describe("PendingReviewBanner", () => {
  it("renders nothing for a personal user (no tenant status)", () => {
    const { container } = render(<PendingReviewBanner tenantStatus={undefined} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when the tenant is active", () => {
    const { container } = render(<PendingReviewBanner tenantStatus="active" />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when the tenant is suspended", () => {
    const { container } = render(<PendingReviewBanner tenantStatus="suspended" />);
    expect(container).toBeEmptyDOMElement();
  });

  it("shows the pending review banner when the tenant is under review", () => {
    render(<PendingReviewBanner tenantStatus="pending_review" />);
    expect(screen.getByRole("status")).toBeInTheDocument();
    expect(screen.getByText("企业账号审核中")).toBeInTheDocument();
    expect(screen.getByText(/正在等待平台管理员审核/)).toBeInTheDocument();
  });

  it("announces the status to assistive technology", () => {
    render(<PendingReviewBanner tenantStatus="pending_review" />);
    expect(screen.getByRole("status")).toHaveTextContent("企业账号审核中");
  });
});
