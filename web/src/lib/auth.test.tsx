import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { AuthProvider, useAuth } from "./auth";

// Mock window.fetch
const mockFetch = vi.fn();

beforeEach(() => {
  vi.stubGlobal("fetch", mockFetch);
  vi.clearAllMocks();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function mockFetchResponse(status: number, data: unknown) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(data),
  } as Response);
}

describe("AuthProvider", () => {
  it("fetches /api/console/me on mount", async () => {
    mockFetch.mockResolvedValueOnce(
      mockFetchResponse(200, {
        id: "user-1",
        email: "test@test.com",
        name: "Test User",
        role: "user",
        status: "active",
      }),
    );

    renderHook(() => useAuth(), {
      wrapper: ({ children }) => (
        <MemoryRouter>
          <AuthProvider>{children}</AuthProvider>
        </MemoryRouter>
      ),
    });

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/console/me",
        expect.objectContaining({ credentials: "include" }),
      );
    });
  });

  it("provides user when /me succeeds", async () => {
    mockFetch.mockResolvedValueOnce(
      mockFetchResponse(200, {
        id: "user-1",
        email: "test@test.com",
        name: "Test User",
        role: "user",
        status: "active",
      }),
    );

    const { result } = renderHook(() => useAuth(), {
      wrapper: ({ children }) => (
        <MemoryRouter>
          <AuthProvider>{children}</AuthProvider>
        </MemoryRouter>
      ),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.isAuthenticated).toBe(true);
    expect(result.current.user).toEqual(
      expect.objectContaining({ email: "test@test.com" }),
    );
  });

  it("sets isAuthenticated=false when /me fails with 401", async () => {
    mockFetch.mockResolvedValueOnce(mockFetchResponse(401, {}));

    const { result } = renderHook(() => useAuth(), {
      wrapper: ({ children }) => (
        <MemoryRouter>
          <AuthProvider>{children}</AuthProvider>
        </MemoryRouter>
      ),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.isAuthenticated).toBe(false);
    expect(result.current.user).toBeNull();
  });

  it("provides isLoading=true during fetch", () => {
    mockFetch.mockReturnValueOnce(new Promise(() => {}));

    const { result } = renderHook(() => useAuth(), {
      wrapper: ({ children }) => (
        <MemoryRouter>
          <AuthProvider>{children}</AuthProvider>
        </MemoryRouter>
      ),
    });

    expect(result.current.isLoading).toBe(true);
  });

  it("useAuth throws when used outside AuthProvider", () => {
    expect(() => renderHook(() => useAuth())).toThrow(
      "useAuth must be used within an AuthProvider",
    );
  });

  it("logout calls POST /api/console/auth/logout and clears user", async () => {
    mockFetch.mockResolvedValueOnce(
      mockFetchResponse(200, {
        id: "user-1",
        email: "test@test.com",
        name: "Test User",
        role: "user",
        status: "active",
      }),
    );

    const { result } = renderHook(() => useAuth(), {
      wrapper: ({ children }) => (
        <MemoryRouter>
          <AuthProvider>{children}</AuthProvider>
        </MemoryRouter>
      ),
    });

    // Wait for initial auth to complete
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });
    expect(result.current.isAuthenticated).toBe(true);

    // Now test logout
    mockFetch.mockResolvedValueOnce(
      mockFetchResponse(200, { message: "Logged out" }),
    );

    await result.current.logout();

    await waitFor(() => {
      expect(result.current.isAuthenticated).toBe(false);
      expect(result.current.user).toBeNull();
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/console/auth/logout",
      expect.objectContaining({
        method: "POST",
        credentials: "include",
      }),
    );
  });
});
