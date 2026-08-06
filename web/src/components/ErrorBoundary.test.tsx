import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ErrorBoundary from "./ErrorBoundary";

/** Child component that throws during render when `shouldThrow` is true. */
function Bomb({ shouldThrow }: { shouldThrow: boolean }) {
  if (shouldThrow) {
    throw new Error("test crash");
  }
  return <div>normal content</div>;
}

describe("ErrorBoundary", () => {
  let consoleErrorSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    consoleErrorSpy.mockRestore();
  });

  it("renders children normally when no error occurs", () => {
    render(
      <ErrorBoundary>
        <Bomb shouldThrow={false} />
      </ErrorBoundary>,
    );
    expect(screen.getByText("normal content")).toBeInTheDocument();
  });

  it("displays fallback UI when a child throws during render", () => {
    render(
      <ErrorBoundary>
        <Bomb shouldThrow={true} />
      </ErrorBoundary>,
    );
    expect(screen.getByText("页面渲染错误")).toBeInTheDocument();
  });

  it("displays the error message in the fallback", () => {
    render(
      <ErrorBoundary>
        <Bomb shouldThrow={true} />
      </ErrorBoundary>,
    );
    expect(screen.getByText("test crash")).toBeInTheDocument();
  });

  it("shows a retry button in the fallback", () => {
    render(
      <ErrorBoundary>
        <Bomb shouldThrow={true} />
      </ErrorBoundary>,
    );
    expect(screen.getByRole("button", { name: /重试/ })).toBeInTheDocument();
  });

  it("uses a custom fallback prop when provided", () => {
    render(
      <ErrorBoundary fallback={<div>custom fallback</div>}>
        <Bomb shouldThrow={true} />
      </ErrorBoundary>,
    );
    expect(screen.getByText("custom fallback")).toBeInTheDocument();
    expect(screen.queryByText("页面渲染错误")).not.toBeInTheDocument();
  });

  it("resets the boundary and re-renders children when retry is clicked", async () => {
    const user = userEvent.setup();
    // Controlled child: throws on first render, then recovers.
    const { rerender } = render(
      <ErrorBoundary>
        <Bomb shouldThrow={true} />
      </ErrorBoundary>,
    );
    expect(screen.getByText("页面渲染错误")).toBeInTheDocument();

    // Recovery via prop change (simulates data fixing itself).
    rerender(
      <ErrorBoundary>
        <Bomb shouldThrow={false} />
      </ErrorBoundary>,
    );

    // Retry button resets boundary state; children render again.
    await user.click(screen.getByRole("button", { name: /重试/ }));
    expect(screen.getByText("normal content")).toBeInTheDocument();
    expect(screen.queryByText("页面渲染错误")).not.toBeInTheDocument();
  });

  it("calls onError callback with error and info when an error is caught", () => {
    const onError = vi.fn();
    render(
      <ErrorBoundary onError={onError}>
        <Bomb shouldThrow={true} />
      </ErrorBoundary>,
    );
    expect(onError).toHaveBeenCalledTimes(1);
    const [err] = onError.mock.calls[0];
    expect(err).toBeInstanceOf(Error);
    expect((err as Error).message).toBe("test crash");
  });

  it("does NOT auto-reset on children key change; requires explicit retry", async () => {
    const user = userEvent.setup();
    const { rerender } = render(
      <ErrorBoundary>
        <Bomb key="a" shouldThrow={true} />
      </ErrorBoundary>,
    );
    expect(screen.getByText("页面渲染错误")).toBeInTheDocument();

    // Changing the child key re-mounts the child, but the boundary keeps its
    // hasError state (React does not auto-reset error boundaries). The retry
    // button is the explicit reset path.
    rerender(
      <ErrorBoundary>
        <Bomb key="b" shouldThrow={false} />
      </ErrorBoundary>,
    );
    expect(screen.getByText("页面渲染错误")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /重试/ }));
    expect(screen.getByText("normal content")).toBeInTheDocument();
  });

  it("does NOT catch errors thrown in event handlers (documented limitation)", async () => {
    const user = userEvent.setup();
    function ClickBomb() {
      return (
        <button
          onClick={() => {
            throw new Error("event handler error");
          }}
        >
          boom
        </button>
      );
    }

    render(
      <ErrorBoundary>
        <ClickBomb />
      </ErrorBoundary>,
    );

    // Event-handler errors are not caught by error boundaries; the error
    // propagates to the browser. Assert the boundary stays normal.
    await user.click(screen.getByText("boom"));
    expect(screen.queryByText("页面渲染错误")).not.toBeInTheDocument();
  });

  it("renders null children safely", () => {
    render(<ErrorBoundary>{null}</ErrorBoundary>);
    // No crash, no fallback.
    expect(screen.queryByText("页面渲染错误")).not.toBeInTheDocument();
  });
});
