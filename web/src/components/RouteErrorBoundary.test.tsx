import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useNavigate } from "react-router-dom";
import RouteErrorBoundary from "./RouteErrorBoundary";

/** Page that throws when `crash` is true. */
function CrashPage({ crash }: { crash: boolean }) {
  if (crash) {
    throw new Error("page crash");
  }
  return <div>page content</div>;
}

/** Nav helper: renders a link that navigates to /b. */
function NavButton() {
  const navigate = useNavigate();
  return <button onClick={() => navigate("/b")}>Go to B</button>;
}

function AppRoutes({ crashA, crashB }: { crashA: boolean; crashB: boolean }) {
  return (
    <Routes>
      <Route
        path="/a"
        element={
          <div>
            <RouteErrorBoundary>
              <CrashPage crash={crashA} />
            </RouteErrorBoundary>
            <NavButton />
          </div>
        }
      />
      <Route
        path="/b"
        element={
          <RouteErrorBoundary>
            <CrashPage crash={crashB} />
          </RouteErrorBoundary>
        }
      />
    </Routes>
  );
}

describe("RouteErrorBoundary", () => {
  let consoleErrorSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    consoleErrorSpy.mockRestore();
  });

  it("renders page content normally when the page does not crash", () => {
    render(
      <MemoryRouter initialEntries={["/a"]}>
        <AppRoutes crashA={false} crashB={false} />
      </MemoryRouter>,
    );
    expect(screen.getByText("page content")).toBeInTheDocument();
  });

  it("auto-resets the boundary when navigating to a different route", async () => {
    const userEventModule = (await import("@testing-library/user-event")).default;
    const user = userEventModule.setup();
    // /a crashes -> fallback shown. Navigate to /b (healthy) -> content renders.
    render(
      <MemoryRouter initialEntries={["/a"]}>
        <AppRoutes crashA={true} crashB={false} />
      </MemoryRouter>,
    );
    expect(screen.getByText("页面渲染错误")).toBeInTheDocument();

    await user.click(screen.getByText("Go to B"));
    expect(screen.getByText("page content")).toBeInTheDocument();
    expect(screen.queryByText("页面渲染错误")).not.toBeInTheDocument();
  });

  it("keeps boundary error state when staying on the same route until retry", async () => {
    const userEventModule = (await import("@testing-library/user-event")).default;
    const user = userEventModule.setup();
    render(
      <MemoryRouter initialEntries={["/a"]}>
        <AppRoutes crashA={true} crashB={false} />
      </MemoryRouter>,
    );
    expect(screen.getByText("页面渲染错误")).toBeInTheDocument();

    // Stay on /a: boundary keeps error; retry re-renders (crashA still true so it throws again).
    await user.click(screen.getByRole("button", { name: /重试/ }));
    expect(screen.getByText("页面渲染错误")).toBeInTheDocument();
  });
});
