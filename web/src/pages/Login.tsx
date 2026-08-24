import React, { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Coins, Scale, ShieldCheck, FileCheck2, ArrowRight, KeyRound } from "lucide-react";
import { useAuth } from "../lib/auth";
import BrandLogo from "../components/BrandLogo";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

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

/** 一把密钥即可调用的模型（登录页展示，非模型目录数据源）。以国产模型为主。 */
const MODELS = [
  { name: "DeepSeek", vendor: "深度求索", color: "#0FA88B" },
  { name: "通义千问", vendor: "阿里云 Qwen", color: "#8B6FE8" },
  { name: "智谱 GLM", vendor: "智谱AI", color: "#4F6BED" },
  { name: "Kimi", vendor: "月之暗面", color: "#E5484D" },
  { name: "豆包", vendor: "字节跳动", color: "#C9A96A" },
  { name: "混元", vendor: "腾讯云", color: "#D3A94E" },
];

const FEATURES = [
  { icon: Coins, label: "统一计费", color: "#4F6BED" },
  { icon: ShieldCheck, label: "预算预留", color: "#0FA88B" },
  { icon: Scale, label: "用量对账", color: "#8B6FE8" },
  { icon: FileCheck2, label: "证据链", color: "#D3A94E" },
];

export default function Login() {
  const navigate = useNavigate();
  const auth = useAuth();
  const [accountType, setAccountType] = useState<AccountType>("personal");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  // Real active-model count from the public stats endpoint, so the landing
  // stat matches the actual catalog instead of a hardcoded number.
  const [modelCount, setModelCount] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetch("/api/public/stats")
      .then((res) => (res.ok ? res.json() : null))
      .then((data) => {
        if (!cancelled && data && typeof data.models === "number") {
          setModelCount(String(data.models));
        }
      })
      .catch(() => {
        /* keep the placeholder when stats are unavailable */
      });
    return () => {
      cancelled = true;
    };
  }, []);

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
    <div className="relative min-h-screen overflow-hidden flex items-center justify-center p-6">
      <div className="lg-orb w-[520px] h-[460px] bg-[#4F6BED]/22 -top-[170px] -right-[110px]" />
      <div className="lg-orb w-[460px] h-[420px] bg-[#0FA88B]/20 -bottom-[160px] -left-[130px]" />
      <div className="lg-orb w-[320px] h-[300px] bg-[#C9A96A]/14 top-[16%] left-[46%]" />

      <div className="relative z-10 grid w-full max-w-[1280px] grid-cols-1 lg:grid-cols-[1.08fr_0.92fr] gap-6 items-stretch">
        {/* 左：品牌与产品价值 */}
        <div className="glass rounded-[22px] p-[42px] flex flex-col justify-center overflow-hidden">
          <BrandLogo className="w-[210px]" />
          <div className="text-[12px] font-bold tracking-[0.2em] text-[#5C6472] mt-8">AI TOKEN 聚合与计费平台</div>
          <h1 className="text-[40px] leading-[1.14] font-bold mt-[16px] mb-3">
            一个 API Key，<br />调用<span className="relative text-[#4F6BED]">所有模型</span>
            <svg className="absolute left-0 right-0 -bottom-[7px] h-2" viewBox="0 0 160 8" preserveAspectRatio="none" aria-hidden="true">
              <path d="M2 6 C30 1 55 7 80 4 C105 1 130 6 158 3" fill="none" stroke="#F2644B" strokeWidth="3" strokeLinecap="round" />
            </svg>
          </h1>
          <p className="text-[15px] text-[#5C6472] max-w-[520px]">一个 API Key，统一接入 DeepSeek · 通义千问 · 智谱 · Kimi 等国产主流模型，切换模型不改密钥。计费、风控、对账，一屏看清。</p>

          {/* 一把密钥 → 所有模型：可视化 */}
          <div className="glass-soft rounded-2xl p-5 mt-7">
            <div className="flex items-center gap-2 mb-4">
              <span className="grid w-6 h-6 place-items-center rounded-lg bg-gradient-to-br from-[#4F6BED] to-[#8B6FE8] text-white shadow-[0_4px_12px_rgba(79,107,237,0.4)]">
                <KeyRound size={13} strokeWidth={2.5} />
              </span>
              <span className="font-mono text-[13px] font-bold text-[#161A23]">sk-dt-••••••</span>
              <span className="ml-auto text-[11px] font-semibold text-[#5C6472]">一个密钥 · N 个模型</span>
            </div>
            <div className="flex items-center gap-[10px]">
              {MODELS.map((m, i) => (
                <React.Fragment key={m.name}>
                  {i > 0 && <span className="text-[#B9BEC9] font-mono text-[13px]">→</span>}
                  <div className="flex-1 min-w-0 text-center rounded-xl bg-white/80 border border-white/95 px-2 py-2.5 shadow-[inset_0_1px_0_rgba(255,255,255,0.95)] transition-transform hover:-translate-y-0.5">
                    <div className="mx-auto w-5 h-5 rounded-full mb-1.5" style={{ backgroundColor: m.color, boxShadow: `0 0 10px ${m.color}66` }} />
                    <div className="text-[11.5px] font-bold text-[#161A23] truncate">{m.name}</div>
                    <div className="text-[9.5px] text-[#8B93A3] truncate">{m.vendor}</div>
                  </div>
                </React.Fragment>
              ))}
            </div>
          </div>

          {/* 一把密钥调用所有模型：代码示例 */}
          <div className="glass-soft rounded-2xl p-4 font-mono text-[12.5px] leading-relaxed mt-7">
            <div className="flex items-center gap-2 mb-3">
              <span className="w-2.5 h-2.5 rounded-full bg-[#E5484D]/80" />
              <span className="w-2.5 h-2.5 rounded-full bg-[#D3A94E]/80" />
              <span className="w-2.5 h-2.5 rounded-full bg-[#1BA878]/80" />
              <span className="ml-2 font-sans text-[11px] font-semibold text-[#5C6472] tracking-wide">一把密钥 · 全部模型</span>
            </div>
            <div className="space-y-1.5 text-[#2A3040]">
              <div>
                <span className="text-[#8B6FE8]">curl</span>{" "}
                <span className="text-[#4F6BED]">https://api.deeptrols.ai/v1/chat/completions</span>
              </div>
              <div className="pl-4">
                <span className="text-[#5C6472]">-H</span> <span className="text-[#0FA88B]">"Authorization: Bearer sk-dt-••••••"</span>
              </div>
              <div className="pl-4">
                <span className="text-[#5C6472]">-d</span> <span className="text-[#C9A96A]">{'{"model": "deepseek-chat"}'}</span>
              </div>
              <div className="pt-2 border-t border-black/5 text-[#5C6472]"># 同一把密钥，切换任意模型</div>
              <div>
                <span className="text-[#0FA88B]">deepseek-v3</span>
                <span className="text-[#5C6472]"> · </span>
                <span className="text-[#E5484D]">qwen-max</span>
                <span className="text-[#5C6472]"> · </span>
                <span className="text-[#4F6BED]">glm-4</span>
                <span className="text-[#5C6472]"> · </span>
                <span className="text-[#D3A94E]">moonshot-v1-8k</span>
              </div>
            </div>
          </div>

          {/* 支持的模型 */}
          <div className="flex flex-wrap gap-2 mt-4">
            {MODELS.map((m) => (
              <span key={m.name} className="glass-soft inline-flex items-center gap-1.5 rounded-full px-3 py-1.5 text-[12px] font-semibold text-[#161A23]">
                <span className="w-1.5 h-1.5 rounded-full" style={{ backgroundColor: m.color, boxShadow: `0 0 7px ${m.color}` }} />
                {m.name}
              </span>
            ))}
          </div>

          {/* 平台能力 */}
          <div className="flex flex-wrap gap-x-6 gap-y-2.5 mt-6">
            {FEATURES.map((f) => (
              <div key={f.label} className="flex items-center gap-2 text-[12.5px] font-semibold text-[#5C6472]">
                <span className="grid w-7 h-7 place-items-center rounded-[9px] bg-white/80 border border-white/90 shadow-[inset_0_1px_0_rgba(255,255,255,0.9)]" style={{ color: f.color }}>
                  <f.icon size={14} />
                </span>
                {f.label}
              </div>
            ))}
          </div>

          <div className="flex gap-3 flex-wrap mt-7">
            {[
              { k: modelCount ?? "—", v: "在线模型", c: "#4F6BED" },
              { k: "4.2M", v: "今日调用", c: "#0FA88B" },
              { k: "99.99%", v: "服务可用率", c: "#1BA878" },
              { k: "1 个", v: "密钥搞定", c: "#8B6FE8" },
            ].map((s) => (
              <div key={s.v} className="glass-soft flex items-baseline gap-1.5 rounded-xl px-4 py-2.5">
                <span className="font-mono text-[17px] font-bold" style={{ color: s.c }}>{s.k}</span>
                <span className="text-[11.5px] font-semibold text-[#5C6472]">{s.v}</span>
              </div>
            ))}
          </div>
        </div>

        {/* 右：登录表单 */}
        <div className="glass rounded-[22px] p-[44px] flex flex-col overflow-hidden">
          <div className="flex items-center gap-2 text-[12px] font-bold tracking-[0.18em] text-[#5C6472]">
            <span className="w-2 h-2 rounded-full bg-[#0FA88B] shadow-[0_0_8px_#0FA88B]" />
            ONE KEY · ALL MODELS
          </div>
          <h3 className="font-display text-[25px] font-bold mt-3">登录控制台</h3>
          <p className="text-[13px] text-[#5C6472] mt-1.5 mb-7">{copy.description}</p>

          <div className="inline-flex rounded-xl glass-soft p-1 mb-7" role="group" aria-label="账号类型">
            {(["personal", "enterprise"] as const).map((t) => (
              <button
                key={t}
                type="button"
                onClick={() => setAccountType(t)}
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

          <form onSubmit={handleSubmit} className="space-y-5">
            <div className="space-y-2">
              <Label htmlFor="email" className="text-[12px] font-semibold text-[#5C6472]">账号</Label>
              <Input
                id="email"
                type="text"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder={copy.accountPlaceholder}
                className="py-[13px] h-auto text-[14.5px]"
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password" className="text-[12px] font-semibold text-[#5C6472]">密码</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="请输入密码"
                className="py-[13px] h-auto text-[14.5px]"
                required
              />
            </div>
            {error && (
              <div className="p-3 bg-[#E5484D]/10 border border-[#E5484D]/20 rounded-xl text-[#C4372C] text-sm">{error}</div>
            )}
            <Button type="submit" disabled={loading} className="w-full py-[15px] h-auto text-[14.5px]">
              {loading ? "登录中..." : "登 录"}
            </Button>
          </form>
          <div className="flex items-center justify-between mt-5 text-[13px] text-[#5C6472]">
            <span>{copy.registerText} <Link to={copy.registerTarget} className="text-[#4F6BED] font-semibold hover:underline">立即注册</Link></span>
          </div>

          <div className="mt-auto pt-8">
            <div className="rounded-2xl bg-gradient-to-br from-[#4F6BED]/10 via-transparent to-[#0FA88B]/10 border border-white/90 p-4">
              <div className="flex items-start gap-3">
                <span className="grid w-8 h-8 shrink-0 place-items-center rounded-[10px] bg-white/90 border border-white shadow-[inset_0_1px_0_rgba(255,255,255,0.95)] text-[#4F6BED]">
                  <ArrowRight size={15} strokeWidth={2.4} />
                </span>
                <div>
                  <div className="text-[13px] font-bold text-[#161A23]">三分钟接入</div>
                  <div className="text-[12px] leading-relaxed text-[#5C6472] mt-0.5">生成一把密钥 → 复制一行 curl → 立即调用全部模型。计费与用量实时可见。</div>
                </div>
              </div>
            </div>
            <div className="text-center text-[11px] text-[#8B93A3] mt-4">登录即表示同意《服务条款》与《隐私政策》</div>
          </div>
        </div>
      </div>
    </div>
  );
}
