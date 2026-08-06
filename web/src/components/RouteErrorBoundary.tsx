import { useLocation } from "react-router-dom";
import type { ReactNode } from "react";
import ErrorBoundary from "./ErrorBoundary";

type RouteErrorBoundaryProps = {
  children: ReactNode;
};

/**
 * ErrorBoundary that auto-resets on every route change.
 *
 * React ErrorBoundary keeps its `hasError` state once a child throws — changing
 * the child (navigating to a different page) does NOT clear it. By keying the
 * boundary with the current pathname, React remounts the boundary whenever the
 * route changes, so a crash on page A is cleared as soon as the user navigates
 * to page B (which also re-renders its own content).
 */
export default function RouteErrorBoundary({ children }: RouteErrorBoundaryProps) {
  const location = useLocation();
  return (
    <ErrorBoundary key={location.pathname}>{children}</ErrorBoundary>
  );
}
