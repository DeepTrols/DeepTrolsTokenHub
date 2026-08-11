const API_BASE = "/api/console";
const ADMIN_API_BASE = "/api/admin";

async function request<T>(baseURL: string, path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(`${baseURL}${path}`, {
    ...options,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...(options.headers as Record<string, string>),
    },
  });

  if (res.status === 401) {
    window.location.href = "/login";
    throw new Error("Unauthorized");
  }

  if (res.status === 403) {
    throw new Error("Forbidden: Admin access required");
  }

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Request failed" }));
    throw new Error(err.error || "Request failed");
  }

  return res.json();
}

function consoleRequest<T>(path: string, options?: RequestInit): Promise<T> {
  return request<T>(API_BASE, path, options);
}

function adminRequest<T>(path: string, options?: RequestInit): Promise<T> {
  return request<T>(ADMIN_API_BASE, path, options);
}

export const api = {
  get: <T>(path: string) => consoleRequest<T>(path),
  post: <T>(path: string, body?: unknown) =>
    consoleRequest<T>(path, { method: "POST", body: JSON.stringify(body) }),
  put: <T>(path: string, body?: unknown) =>
    consoleRequest<T>(path, { method: "PUT", body: JSON.stringify(body) }),
  delete: <T>(path: string) => consoleRequest<T>(path, { method: "DELETE" }),
};

export const adminApi = {
  get: <T>(path: string) => adminRequest<T>(path),
  post: <T>(path: string, body?: unknown) =>
    adminRequest<T>(path, { method: "POST", body: JSON.stringify(body) }),
  put: <T>(path: string, body?: unknown) =>
    adminRequest<T>(path, { method: "PUT", body: JSON.stringify(body) }),
  delete: <T>(path: string) => adminRequest<T>(path, { method: "DELETE" }),
};

export interface User {
  id: string;
  email: string;
  name: string;
}

export interface APIKeyData {
  id: string;
  name: string;
  masked_key: string;
  key_prefix?: string;
  status: string;
  allowed_models?: string[] | null;
  source_whitelist?: string[] | null;
  monthly_limit?: string;
  weekly_limit?: string;
  cumulative_limit?: string;
  over_limit_action?: string;
  last_used_at?: string;
  last_7d_active?: boolean;
  created_at: string;
}

export interface UsageLog {
  id: string;
  model: string;
  request_id: string;
  api_key_id?: string;
  api_key_name?: string;
  status: string;
  input_tokens: number;
  output_tokens: number;
  cost: string;
  created_at: string;
}

export interface WalletData {
  balance: string;
  frozen: string;
  available: string;
  currency: string;
  total_charged: string;
}

export interface Transaction {
  order_no: string;
  status: string;
  payment_method: string;
  id: string;
  type: string;
  amount: string;
  balance_after: string;
  reference: string;
  created_at: string;
}

export interface ModelData {
  code: string;
  provider: string;
  category: string;
  display_name: string;
  context_window: number;
  pricing: Record<string, string>;
}
