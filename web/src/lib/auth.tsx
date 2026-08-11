import React, { createContext, useContext, useState, useEffect, useCallback } from "react";

export interface AuthUser {
  id: string;
  email: string;
  name: string;
  role: string;
  status: string;
  totp_enabled: boolean;
  user_type: string;
  phone: string;
  avatar_url: string;
  tenant_id: string;
  tenant_name: string;
  tenant_role: string;
}

/** Result of a login attempt. When mfaRequired is true the caller should
 * prompt for a TOTP code and call login again with totpCode set. */
export interface LoginResult {
  success: boolean;
  mfaRequired: boolean;
  error: string | null;
}

interface AuthContextValue {
  user: AuthUser | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  login: (email: string, password: string, totpCode?: string) => Promise<LoginResult>;
  register: (email: string, password: string, name: string) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  const fetchMe = useCallback(async () => {
    try {
      const res = await fetch("/api/console/me", { credentials: "include" });
      if (!res.ok) {
        setUser(null);
        return;
      }
      const data = (await res.json()) as AuthUser;
      setUser(data);
    } catch {
      setUser(null);
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    fetchMe().then(() => {
      if (!cancelled) setIsLoading(false);
    });
    return () => { cancelled = true; };
  }, [fetchMe]);

  const login = useCallback(async (email: string, password: string, totpCode?: string): Promise<LoginResult> => {
    let res: Response;
    try {
      res = await fetch("/api/console/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password, totp_code: totpCode }),
        credentials: "include",
      });
    } catch {
      return { success: false, mfaRequired: false, error: "网络错误，请稍后重试" };
    }

    if (res.status === 401) {
      const body = (await res.json().catch(() => ({}))) as { mfa_required?: string };
      if (body.mfa_required === "true") {
        return { success: false, mfaRequired: true, error: "请输入两步验证码" };
      }
      return { success: false, mfaRequired: false, error: "登录失败，请检查账号和密码" };
    }

    if (!res.ok) {
      return { success: false, mfaRequired: false, error: "登录失败，请稍后重试" };
    }

    await fetchMe();
    return { success: true, mfaRequired: false, error: null };
  }, [fetchMe]);

  const register = useCallback(async (email: string, password: string, name: string) => {
    await fetch("/api/console/auth/register", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password, name }),
      credentials: "include",
    });
    await fetchMe();
  }, [fetchMe]);

  const logout = useCallback(async () => {
    await fetch("/api/console/auth/logout", {
      method: "POST",
      credentials: "include",
    });
    setUser(null);
  }, []);

  const value: AuthContextValue = {
    user, isLoading,
    isAuthenticated: user !== null && !isLoading,
    login, register, logout,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (ctx === null) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return ctx;
}
