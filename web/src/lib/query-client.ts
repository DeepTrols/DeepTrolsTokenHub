import { QueryClient } from "@tanstack/react-query";

/**
 * Central QueryClient with sensible defaults for the admin console.
 * Data stays fresh for 30s, garbage-collected after 5 min idle,
 * retries once on failure, and never refetches on window focus.
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      gcTime: 5 * 60 * 1000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});
