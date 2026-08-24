import { Component } from "react";
import type { ErrorInfo, ReactNode } from "react";
import { AlertTriangle } from "lucide-react";

/**
 * React Error Boundary.
 *
 * Note: This is the one legitimate exception to the "no class components"
 * rule — React has no function-component equivalent for
 * getDerivedStateFromError/componentDidCatch.
 *
 * Error boundaries do NOT catch: event-handler errors, async errors,
 * errors in the boundary itself, or errors in server rendering.
 */

type ErrorBoundaryProps = {
  children: ReactNode;
  /** Custom fallback UI; overrides the default error card when provided. */
  fallback?: ReactNode;
  /** Called when a child throws during render (e.g. for logging). */
  onError?: (error: Error, info: ErrorInfo) => void;
};

type ErrorBoundaryState = {
  hasError: boolean;
  error: Error | null;
  /** Incremented on retry to force a fresh mount of children. */
  resetKey: number;
};

const initialState: ErrorBoundaryState = {
  hasError: false,
  error: null,
  resetKey: 0,
};

export default class ErrorBoundary extends Component<
  ErrorBoundaryProps,
  ErrorBoundaryState
> {
  state: ErrorBoundaryState = initialState;

  static getDerivedStateFromError(error: Error): Partial<ErrorBoundaryState> {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error("[ErrorBoundary]", error, info.componentStack);
    this.props.onError?.(error, info);
  }

  private handleRetry = (): void => {
    this.setState((prev) => ({
      hasError: false,
      error: null,
      resetKey: prev.resetKey + 1,
    }));
  };

  render(): ReactNode {
    if (this.state.hasError) {
      if (this.props.fallback !== undefined) {
        return this.props.fallback;
      }
      return (
        <div className="flex items-center justify-center min-h-[40vh] p-6">
          <div className="w-full max-w-md glass rounded-2xl border-[#E5484D]/25 p-6">
            <div className="flex items-center gap-3 mb-3">
              <div className="p-2 rounded-xl bg-[#E5484D]/10">
                <AlertTriangle size={20} className="text-[#E5484D]" />
              </div>
              <h3 className="font-display font-semibold text-[#C4372C]">页面渲染错误</h3>
            </div>
            <p className="text-sm text-[#5C6472] mb-4">
              {this.state.error?.message || "发生了意外错误，请重试"}
            </p>
            {import.meta.env.DEV && this.state.error?.stack && (
              <details className="mb-4 text-xs text-[#5C6472]">
                <summary className="cursor-pointer">错误详情</summary>
                <pre className="mt-2 p-2 glass-soft rounded-xl overflow-auto whitespace-pre-wrap">
                  {this.state.error.stack}
                </pre>
              </details>
            )}
            <button
              onClick={this.handleRetry}
              className="px-4 py-2 rounded-xl text-sm font-semibold text-white bg-gradient-to-br from-[#E5484D] to-[#C4372C] shadow-[0_10px_26px_rgba(229,72,77,0.3)] hover:brightness-110"
            >
              重试
            </button>
          </div>
        </div>
      );
    }

    return (
      <div key={this.state.resetKey}>{this.props.children}</div>
    );
  }
}
