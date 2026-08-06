import {
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type {
  QueryKey,
  UseMutationOptions,
  UseQueryOptions,
} from "@tanstack/react-query";
import { adminApi, api } from "../api";

/**
 * Generic TanStack Query hooks wrapping the console/admin API clients.
 *
 * Query keys are derived from the endpoint path so a mutation can
 * invalidate the exact query a page reads (e.g. creating an API key
 * invalidates ["console", "/api-keys"]).
 */

function consoleKey(path: string): QueryKey {
  return ["console", path];
}

function adminKey(path: string): QueryKey {
  return ["admin", path];
}

/** useQuery for the console API (JWT-authenticated user endpoints). */
export function useConsoleQuery<T>(
  path: string,
  options?: Omit<UseQueryOptions<T, Error>, "queryKey" | "queryFn">,
) {
  return useQuery<T, Error>({
    queryKey: consoleKey(path),
    queryFn: () => api.get<T>(path),
    ...options,
  });
}

/** useQuery for the admin API (admin-role endpoints). */
export function useAdminQuery<T>(
  path: string,
  options?: Omit<UseQueryOptions<T, Error>, "queryKey" | "queryFn">,
) {
  return useQuery<T, Error>({
    queryKey: adminKey(path),
    queryFn: () => adminApi.get<T>(path),
    ...options,
  });
}

type MutationMethod = "post" | "put" | "delete";

/**
 * Resolves the concrete request path for a mutation. A string is used as-is
 * (static endpoint); a function receives the mutation variables and can build
 * a dynamic path such as `/api-keys/${variables.id}`.
 */
type MutationPath<TVariables> = string | ((variables: TVariables) => string);

function resolvePath<TVariables>(
  path: MutationPath<TVariables>,
  variables: TVariables,
): string {
  return typeof path === "function" ? path(variables) : path;
}

/**
 * Generic console mutation. Every successful mutation invalidates the
 * query for `invalidatePath` (defaults to `path` when it's a static string)
 * so the page's list/read state stays fresh. Pass an explicit
 * `invalidatePath` when the mutation targets a sub-resource (e.g.
 * PUT /api-keys/{id} invalidates /api-keys).
 */
export function useConsoleMutation<TData = unknown, TVariables = unknown>(
  method: MutationMethod,
  path: MutationPath<TVariables>,
  invalidatePath: MutationPath<TVariables> = typeof path === "string" ? path : "",
  options?: UseMutationOptions<TData, Error, TVariables>,
) {
  const { onSuccess: callerOnSuccess, ...restOptions } = options ?? {};
  const queryClient = useQueryClient();
  return useMutation<TData, Error, TVariables>({
    mutationFn: (variables) => {
      const resolved = resolvePath(path, variables);
      const body = variables as unknown;
      if (method === "post") return api.post<TData>(resolved, body);
      if (method === "put") return api.put<TData>(resolved, body);
      return api.delete<TData>(resolved);
    },
    onSuccess: (data, variables, onMutateResult, mutationContext) => {
      const invalidate = resolvePath(invalidatePath, variables);
      if (invalidate) {
        queryClient.invalidateQueries({ queryKey: consoleKey(invalidate) });
      }
      callerOnSuccess?.(data, variables, onMutateResult, mutationContext);
    },
    ...restOptions,
  });
}

/** Generic admin mutation, invalidates `["admin", invalidatePath]` on success. */
export function useAdminMutation<TData = unknown, TVariables = unknown>(
  method: MutationMethod,
  path: MutationPath<TVariables>,
  invalidatePath: MutationPath<TVariables> = typeof path === "string" ? path : "",
  options?: UseMutationOptions<TData, Error, TVariables>,
) {
  const { onSuccess: callerOnSuccess, ...restOptions } = options ?? {};
  const queryClient = useQueryClient();
  return useMutation<TData, Error, TVariables>({
    mutationFn: (variables) => {
      const resolved = resolvePath(path, variables);
      const body = variables as unknown;
      if (method === "post") return adminApi.post<TData>(resolved, body);
      if (method === "put") return adminApi.put<TData>(resolved, body);
      return adminApi.delete<TData>(resolved);
    },
    onSuccess: (data, variables, onMutateResult, mutationContext) => {
      const invalidate = resolvePath(invalidatePath, variables);
      if (invalidate) {
        queryClient.invalidateQueries({ queryKey: adminKey(invalidate) });
      }
      callerOnSuccess?.(data, variables, onMutateResult, mutationContext);
    },
    ...restOptions,
  });
}

export { consoleKey, adminKey };
