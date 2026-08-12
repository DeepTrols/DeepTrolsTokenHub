import React, { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useAuth } from "../lib/auth";
import { Cpu } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

type AccountType = "personal" | "enterprise";

/** Copy + register-link target differ per account type; the login form is shared. */
const ACCOUNT_COPY: Record<AccountType, { description: string; accountPlaceholder: string; registerText: string; registerTarget: string }> = {
  personal: {
    description: "AI 模型聚合平台 · 管理控制台",
    accountPlaceholder: "请输入管理员账号",
    registerText: "还没有账号？",
    registerTarget: "/register?type=personal",
  },
  enterprise: {
    description: "企业 AI 模型聚合平台 · 管理控制台",
    accountPlaceholder: "请输入企业管理员账号",
    registerText: "还没有企业账号？",
    registerTarget: "/register?type=enterprise",
  },
};

export default function Login() {
  const navigate = useNavigate();
  const auth = useAuth();
  const [accountType, setAccountType] = useState<AccountType>("personal");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const copy = ACCOUNT_COPY[accountType];

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      const result = await auth.login(email, password);
      if (result.success) {
        navigate("/dashboard");
      } else {
        setError(result.error || "登录失败，请检查账号和密码");
      }
    } catch {
      setError("登录失败，请检查账号和密码");
    }
    setLoading(false);
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100">
      <Card className="w-full max-w-md border-gray-100">
        <CardHeader className="text-center pb-2">
          <div className="mx-auto mb-3 inline-flex items-center justify-center w-14 h-14 bg-primary/10 rounded-xl">
            <Cpu size={28} className="text-primary" />
          </div>
          <CardTitle className="text-2xl">DeepTrols</CardTitle>
          <CardDescription>{copy.description}</CardDescription>
          <div className="mx-auto mt-4 inline-flex rounded-lg bg-muted p-1" role="group" aria-label="账号类型">
            {(["personal", "enterprise"] as const).map((t) => (
              <button
                key={t}
                type="button"
                onClick={() => setAccountType(t)}
                aria-pressed={accountType === t}
                className={`rounded-md px-4 py-1.5 text-sm font-medium transition-colors ${
                  accountType === t
                    ? "bg-background text-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                {t === "personal" ? "个人" : "企业"}
              </button>
            ))}
          </div>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="email">账号</Label>
              <Input
                id="email"
                type="text"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder={copy.accountPlaceholder}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">密码</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="请输入密码"
                required
              />
            </div>
            {error && (
              <div className="p-3 bg-destructive/10 border border-destructive/20 rounded-lg text-destructive text-sm">{error}</div>
            )}
            <Button type="submit" disabled={loading} className="w-full" size="lg">
              {loading ? "登录中..." : "登 录"}
            </Button>
          </form>
          <p className="text-center text-xs text-muted-foreground mt-6">
            {copy.registerText}{" "}
            <Link to={copy.registerTarget} className="text-primary hover:underline">
              立即注册
            </Link>
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
