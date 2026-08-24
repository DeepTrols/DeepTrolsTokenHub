import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useAuth } from "../lib/auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

type AccountType = "personal" | "enterprise";

/** Copy differs per account type; the toggle also clears any typed state. */
const ACCOUNT_COPY: Record<AccountType, { description: string }> = {
  personal: { description: "AI 模型聚合平台 · 创建账号" },
  enterprise: { description: "企业 AI 模型聚合平台 · 创建账号" },
};

export default function Register() {
  const navigate = useNavigate();
  const auth = useAuth();
  const [searchParams] = useSearchParams();

  const [accountType, setAccountType] = useState<AccountType>(
    searchParams.get("type") === "enterprise" ? "enterprise" : "personal",
  );
  const [name, setName] = useState("");
  const [companyName, setCompanyName] = useState("");
  const [contactName, setContactName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const switchAccountType = (next: AccountType) => {
    setAccountType(next);
    setName("");
    setCompanyName("");
    setContactName("");
    setEmail("");
    setPassword("");
    setError("");
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      if (accountType === "enterprise") {
        await auth.registerEnterprise({ companyName, contactName, email, password });
      } else {
        await auth.register(email, password, name);
      }
      navigate("/dashboard");
    } catch (err) {
      setError(err instanceof Error ? err.message : "注册失败，请稍后重试");
    }
    setLoading(false);
  };

  const copy = ACCOUNT_COPY[accountType];

  return (
    <div className="relative min-h-screen overflow-hidden flex items-center justify-center p-6">
      <div className="lg-orb w-[520px] h-[460px] bg-[#4F6BED]/22 -top-[170px] -right-[110px]" />
      <div className="lg-orb w-[460px] h-[420px] bg-[#0FA88B]/20 -bottom-[160px] -left-[130px]" />
      <div className="lg-orb w-[320px] h-[300px] bg-[#C9A96A]/14 top-[16%] left-[46%]" />

      <div className="relative z-10 glass rounded-[22px] w-full max-w-md p-10">
        <div className="text-center pb-2">
          <img src="/brand-logo.png" alt="DEEPTROLS" className="mx-auto mb-4 w-[180px]" style={{ height: "auto" }} />
          <p className="text-[13px] text-[#5C6472] mt-1">{copy.description}</p>
          <div className="mx-auto mt-5 inline-flex rounded-xl glass-soft p-1" role="group" aria-label="账号类型">
            {(["personal", "enterprise"] as const).map((t) => (
              <button
                key={t}
                type="button"
                onClick={() => switchAccountType(t)}
                aria-pressed={accountType === t}
                className={`rounded-lg px-4 py-1.5 text-sm font-medium transition-all ${
                  accountType === t
                    ? "bg-white/85 text-foreground shadow-[0_4px_14px_rgba(63,76,128,0.12)]"
                    : "text-[#5C6472] hover:text-foreground"
                }`}
              >
                {t === "personal" ? "个人" : "企业"}
              </button>
            ))}
          </div>
        </div>
        <div>
          <form onSubmit={handleSubmit} className="space-y-4 mt-6">
            {accountType === "personal" ? (
              <div className="space-y-2">
                <Label htmlFor="name" className="text-[12px] font-semibold text-[#5C6472]">昵称</Label>
                <Input id="name" value={name} onChange={e => setName(e.target.value)} placeholder="请输入昵称" required />
              </div>
            ) : (
              <>
                <div className="space-y-2">
                  <Label htmlFor="companyName" className="text-[12px] font-semibold text-[#5C6472]">公司名称</Label>
                  <Input id="companyName" value={companyName} onChange={e => setCompanyName(e.target.value)} placeholder="请输入公司名称" required />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="contactName" className="text-[12px] font-semibold text-[#5C6472]">联系人姓名</Label>
                  <Input id="contactName" value={contactName} onChange={e => setContactName(e.target.value)} placeholder="请输入联系人姓名" required />
                </div>
              </>
            )}
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
