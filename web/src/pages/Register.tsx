import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useAuth } from "../lib/auth";
import { Cpu } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export default function Register() {
  const navigate = useNavigate();
  const auth = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault(); setError(""); setLoading(true);
    try { await auth.register(email, password, name); navigate("/dashboard"); }
    catch (err) { setError(err instanceof Error ? err.message : "注册失败，请稍后重试"); }
    setLoading(false);
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100">
      <Card className="w-full max-w-md border-gray-100">
        <CardHeader className="text-center pb-2">
          <div className="mx-auto mb-3 inline-flex items-center justify-center w-14 h-14 bg-primary/10 rounded-xl"><Cpu size={28} className="text-primary" /></div>
          <CardTitle className="text-2xl">DeepTrols</CardTitle>
          <CardDescription>AI 模型聚合平台 · 创建账号</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2"><Label htmlFor="name">昵称</Label><Input id="name" value={name} onChange={e => setName(e.target.value)} placeholder="请输入昵称" required /></div>
            <div className="space-y-2"><Label htmlFor="email">邮箱</Label><Input id="email" type="email" value={email} onChange={e => setEmail(e.target.value)} placeholder="请输入邮箱" required /></div>
            <div className="space-y-2"><Label htmlFor="password">密码</Label><Input id="password" type="password" value={password} onChange={e => setPassword(e.target.value)} placeholder="至少8位" required minLength={8} /></div>
            {error && <div className="p-3 bg-destructive/10 border border-destructive/20 rounded-lg text-destructive text-sm">{error}</div>}
            <Button type="submit" disabled={loading} className="w-full" size="lg">{loading ? "注册中..." : "注 册"}</Button>
          </form>
          <p className="text-center text-xs text-muted-foreground mt-6">已有账号？ <Link to="/login" className="text-primary hover:underline">立即登录</Link></p>
        </CardContent>
      </Card>
    </div>
  );
}
