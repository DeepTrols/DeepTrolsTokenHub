import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useAuth } from "../lib/auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export default function Register() {
  const navigate = useNavigate();
  const auth = useAuth();

  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      await auth.register(email, password, name);
      navigate("/dashboard");
    } catch (err) {
      setError(err instanceof Error ? err.message : "注册失败，请稍后重试");
    }
    setLoading(false);
  };

  return (
    <div className="relative min-h-screen overflow-hidden flex items-center justify-center p-6">
      <div className="lg-orb w-[520px] h-[460px] bg-[#4F6BED]/22 -top-[170px] -right-[110px]" />
      <div className="lg-orb w-[460px] h-[420px] bg-[#0FA88B]/20 -bottom-[160px] -left-[130px]" />
      <div className="lg-orb w-[320px] h-[300px] bg-[#C9A96A]/14 top-[16%] left-[46%]" />

      <div className="relative z-10 glass rounded-[22px] w-full max-w-md p-10">
        <div className="text-center pb-2">
          <img src="/brand-logo.png" alt="DEEPTROLS" className="mx-auto mb-5 w-[196px] h-auto" />
          <p className="text-[13px] text-[#5C6472] mt-1">AI 模型聚合平台 · 创建账号</p>
        </div>
        <div>
          <form onSubmit={handleSubmit} className="space-y-4 mt-6">
            <div className="space-y-2">
              <Label htmlFor="name" className="text-[12px] font-semibold text-[#5C6472]">昵称</Label>
              <Input id="name" value={name} onChange={e => setName(e.target.value)} placeholder="请输入昵称" required />
            </div>
            <div className="space-y-2">
              <Label htmlFor="email" className="text-[12px] font-semibold text-[#5C6472]">邮箱</Label>
              <Input id="email" type="email" value={email} onChange={e => setEmail(e.target.value)} placeholder="请输入邮箱" required />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password" className="text-[12px] font-semibold text-[#5C6472]">密码</Label>
              <Input id="password" type="password" value={password} onChange={e => setPassword(e.target.value)} placeholder="至少8位" required minLength={8} />
            </div>
            {error && <div className="p-3 bg-[#E5484D]/10 border border-[#E5484D]/20 rounded-xl text-[#C4372C] text-sm">{error}</div>}
            <Button type="submit" disabled={loading} className="w-full" size="lg">{loading ? "注册中..." : "注 册"}</Button>
          </form>
          <p className="text-center text-xs text-[#5C6472] mt-6">已有账号？ <Link to="/login" className="text-[#4F6BED] font-semibold hover:underline">立即登录</Link></p>
        </div>
      </div>
    </div>
  );
}
