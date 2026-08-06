import React, { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useAuth } from "../lib/auth";
import { Cpu } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export default function Login() {
  const navigate = useNavigate();
  const auth = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [totpCode, setTotpCode] = useState("");
  const [mfaRequired, setMfaRequired] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      const result = await auth.login(email, password, totpCode || undefined);
      if (result.mfaRequired) {
        setMfaRequired(true);
        setTotpCode("");
      } else if (result.success) {
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
          <CardDescription>AI 模型聚合平台 · 管理控制台</CardDescription>
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
                placeholder="请输入管理员账号"
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
            {mfaRequired && (
              <div className="space-y-2">
                <Label htmlFor="totp">两步验证码</Label>
                <Input
                  id="totp"
                  type="text"
                  inputMode="numeric"
                  value={totpCode}
                  onChange={(e) => setTotpCode(e.target.value.replace(/\D/g, ""))}
                  maxLength={6}
                  className="tracking-widest text-center"
                  placeholder="输入6位动态验证码"
                  required
                />
              </div>
            )}
            {error && (
              <div className="p-3 bg-destructive/10 border border-destructive/20 rounded-lg text-destructive text-sm">{error}</div>
            )}
            <Button type="submit" disabled={loading} className="w-full" size="lg">
              {loading ? "登录中..." : "登 录"}
            </Button>
          </form>
          <p className="text-center text-xs text-muted-foreground mt-6">
            还没有账号？{" "}
            <Link to="/register" className="text-primary hover:underline">
              立即注册
            </Link>
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
