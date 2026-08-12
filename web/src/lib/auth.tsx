import React, { createContext, useContext, useState, useEffect, useCallback } from "react";

export interface AuthUser {
  id: string;
  email: string;
  name: string;
  role: string;
  status: string;
  user_type: string;
  phone: string;
  avatar_url: string;
  tenant_id: string;
  tenant_name: string;
  tenant_role: string;
  /** Tenant lifecycle status (pending_review/active/suspended/...). Empty for personal users. */
  tenant_status?: string;
}

/** Payload for self-service enterprise registration. */
export interface EnterpriseRegisterInput {
  companyName: string;
  contactName: string;
  email: string;
  password: string;
}

/** Result of a login attempt. */
export interface LoginResult {
  success: boolean;
  error: string | null;
}

interface AuthContextValue {
  user: AuthUser | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  login: (email: string, password: string) => Promise<LoginResult>;
  register: (email: string, password: string, name: string) => Promise<void>;
  registerEnterprise: (input: EnterpriseRegisterInput) => Promise<void>;
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

  const login = useCallback(async (email: string, password: string): Promise<LoginResult> => {
    let res: Response;
    try {
      res = await fetch("/api/console/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password }),
        credentials: "include",
      });
    } catch {
      return { success: false, error: "网络错误，请稍后重试" };
    }

    if (res.status === 401) {
      return { success: false, error: "登录失败，请检查账号和密码" };
    }

    if (!res.ok) {
      return { success: false, error: "登录失败，请稍后重试" };
    }

    await fetchMe();
    return { success: true, error: null };
  }, [fetchMe]);

  const register = useCallback(async (email: string, password: string, name: string) => {
    const res = await fetch("/api/console/auth/register", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password, name }),
      credentials: "include",
    });

    if (!res.ok) {
      let message = "注册失败，请稍后重试";
      try {
        const data = (await res.json()) as { error?: string };
        if (data.error) message = data.error;
      } catch {
        // keep the generic fallback when the body is not JSON
      }
      throw new Error(message);
    }

    await fetchMe();
  }, [fetchMe]);

  const registerEnterprise = useCallback(async (input: EnterpriseRegisterInput) => {
    const res = await fetch("/api/console/auth/register/enterprise", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        company_name: input.companyName,
        contact_name: input.contactName,
        email: input.email,
        password: input.password,
      }),
      credentials: "include",
    });

    if (!res.ok) {
      let message = "企业注册失败，请稍后重试";
      try {
        const data = (await res.json()) as { error?: string };
        if (data.error) message = data.error;
      } catch {
        // keep the generic fallback when the body is not JSON
      }
      throw new Error(message);
    }

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
    login, register, registerEnterprise, logout,
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
