import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import type { ReactElement, ReactNode } from "react";

/**
 * Creates a fresh QueryClient for tests: no retries (fail fast),
 * gcTime of 0 to prevent cache leaking between tests.
 */
export function createTestQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
    },
  });
}

interface RenderWithProvidersOptions {
  queryClient?: QueryClient;
}

/**
 * Renders a component wrapped in a fresh QueryClientProvider.
 * Use this instead of `render` in any test that exercises a
 * component using useConsoleQuery / useAdminQuery / mutations.
 */
export function renderWithProviders(
  ui: ReactElement,
  options: RenderWithProvidersOptions = {},
) {
  const queryClient = options.queryClient ?? createTestQueryClient();

  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  }

  return {
    queryClient,
    ...render(ui, { wrapper: Wrapper }),
  };
}
