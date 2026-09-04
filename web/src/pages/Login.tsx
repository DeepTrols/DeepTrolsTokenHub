import React, { useEffect, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { Coins, Scale, ShieldCheck, FileCheck2, ArrowRight, KeyRound, Github, MessageCircle, Chrome } from "lucide-react";
import { useAuth } from "../lib/auth";
import { useSiteInfo } from "../lib/site";
import "../i18n";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SiteBrand } from "@/components/SiteBrand";

/** Register-link target (developer/个人 account). */
const REGISTER_TARGET = "/register";

/** 一把密钥即可调用的模型（登录页展示，非模型目录数据源）。以国产模型为主。 */
const MODELS = [
  { name: "DeepSeek", vendor: "深度求索", color: "#D97706" },
  { name: "通义千问", vendor: "阿里云 Qwen", color: "#E85D3F" },
  { name: "智谱 GLM", vendor: "智谱AI", color: "#F78B28" },
  { name: "Kimi", vendor: "月之暗面", color: "#E5484D" },
  { name: "豆包", vendor: "字节跳动", color: "#D9A15D" },
  { name: "混元", vendor: "腾讯云", color: "#E9A23B" },
];

const FEATURES = [
  { icon: Coins, labelKey: "login.featureBilling", color: "#F78B28" },
  { icon: ShieldCheck, labelKey: "login.featureBudget", color: "#D97706" },
  { icon: Scale, labelKey: "login.featureReconcile", color: "#E85D3F" },
  { icon: FileCheck2, labelKey: "login.featureEvidence", color: "#E9A23B" },
];

export default function Login() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const auth = useAuth();
  const { site } = useSiteInfo();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const oauthNotice = searchParams.get("oauth") === "success" ? t("login.oauthSuccess") : "";
  const oauthError = searchParams.get("error") || "";
  const githubEnabled = (site.oauth_providers ?? []).includes("github");
  const wechatEnabled = (site.oauth_providers ?? []).includes("wechat");
  const googleEnabled = (site.oauth_providers ?? []).includes("google");
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

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      const result = await auth.login(email, password);
      if (result.success) {
        navigate("/dashboard");
      } else {
        setError(result.error || t("login.fail"));
      }
    } catch {
      setError(t("login.fail"));
    }
    setLoading(false);
  };

  return (
    <div className="relative min-h-screen overflow-hidden flex items-center justify-center p-6">
      <div className="lg-orb w-[520px] h-[460px] bg-[#F78B28]/22 -top-[170px] -right-[110px]" />
      <div className="lg-orb w-[460px] h-[420px] bg-[#D97706]/20 -bottom-[160px] -left-[130px]" />
      <div className="lg-orb w-[320px] h-[300px] bg-[#D9A15D]/14 top-[16%] left-[46%]" />

      <div className="relative z-10 grid w-full max-w-[1280px] grid-cols-1 lg:grid-cols-[1.08fr_0.92fr] gap-6 items-stretch">
        {/* 左：品牌与产品价值 */}
        <div className="glass rounded-[22px] p-[42px] flex flex-col justify-center overflow-hidden">
          <SiteBrand
            className="flex items-center gap-3"
            imageClassName="h-[58px] w-[48px] shrink-0 rounded-xl object-contain"
            textClassName="text-[29px] font-bold leading-none text-[#161A23]"
          />
          <div className="text-[12px] font-bold tracking-[0.2em] text-[#5C6472] mt-8">{t("login.platformTag")}</div>
          <h1 className="text-[40px] leading-[1.14] font-bold mt-[16px] mb-3">
            {t("login.heroTitleLine1")}
            <br />
            {t("login.heroTitleLine2")}
            <span className="relative text-primary-700">{t("login.heroTitleHighlight")}</span>
            <svg className="absolute left-0 right-0 -bottom-[7px] h-2" viewBox="0 0 160 8" preserveAspectRatio="none" aria-hidden="true">
              <path d="M2 6 C30 1 55 7 80 4 C105 1 130 6 158 3" fill="none" stroke="#F2644B" strokeWidth="3" strokeLinecap="round" />
            </svg>
          </h1>
          <p className="text-[15px] text-[#5C6472] max-w-[520px]">{t("login.heroDesc")}</p>

          {/* 一把密钥 → 所有模型：可视化 */}
          <div className="glass-soft rounded-2xl p-5 mt-7">
            <div className="flex items-center gap-2 mb-4">
              <span className="grid w-6 h-6 place-items-center rounded-lg bg-gradient-to-br from-[#F78B28] to-[#E85D3F] text-white shadow-[0_4px_12px_rgba(247,139,40,0.4)]">
                <KeyRound size={13} strokeWidth={2.5} />
              </span>
              <span className="font-mono text-[13px] font-bold text-[#161A23]">sk-••••••</span>
              <span className="ml-auto text-[11px] font-semibold text-[#5C6472]">{t("login.oneKeyAllModels")}</span>
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
              <span className="w-2.5 h-2.5 rounded-full bg-[#E9A23B]/80" />
              <span className="w-2.5 h-2.5 rounded-full bg-[#1BA878]/80" />
              <span className="ml-2 font-sans text-[11px] font-semibold text-[#5C6472] tracking-wide">{t("login.codeDemoTitle")}</span>
            </div>
            <div className="space-y-1.5 text-[#2A3040]">
              <div>
                <span className="text-[#B94723]">curl</span>{" "}
                <span className="text-primary-700">https://api.opcstore.com/v1/chat/completions</span>
              </div>
              <div className="pl-4">
                <span className="text-[#5C6472]">-H</span> <span className="text-[#9A4D06]">"Authorization: Bearer sk-••••••"</span>
              </div>
              <div className="pl-4">
                <span className="text-[#5C6472]">-d</span> <span className="text-[#D9A15D]">{'{"model": "deepseek-v4-flash"}'}</span>
              </div>
              <div className="pt-2 border-t border-black/5 text-[#5C6472]">{t("login.codeDemoComment")}</div>
              <div>
                <span className="text-[#9A4D06]">deepseek-v4-pro</span>
                <span className="text-[#5C6472]"> · </span>
                <span className="text-[#E5484D]">qwen-max</span>
                <span className="text-[#5C6472]"> · </span>
                <span className="text-primary-700">glm-4</span>
                <span className="text-[#5C6472]"> · </span>
                <span className="text-[#E9A23B]">moonshot-v1-8k</span>
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
              <div key={f.labelKey} className="flex items-center gap-2 text-[12.5px] font-semibold text-[#5C6472]">
                <span className="grid w-7 h-7 place-items-center rounded-[9px] bg-white/80 border border-white/90 shadow-[inset_0_1px_0_rgba(255,255,255,0.9)]" style={{ color: f.color }}>
                  <f.icon size={14} />
                </span>
                {t(f.labelKey)}
              </div>
            ))}
          </div>

          <div className="flex gap-3 flex-wrap mt-7">
            {[
              { k: modelCount ?? "—", v: t("login.statModels"), c: "#F78B28" },
              { k: "4.2M", v: t("login.statCalls"), c: "#D97706" },
              { k: "99.99%", v: t("login.statUptime"), c: "#1BA878" },
              { k: t("login.keyValue"), v: t("login.statKey"), c: "#E85D3F" },
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
            <span className="w-2 h-2 rounded-full bg-[#D97706] shadow-[0_0_8px_#D97706]" />
            ONE KEY · ALL MODELS
          </div>
          <h3 className="font-display text-[25px] font-bold mt-3">{t("login.title")}</h3>
          <p className="text-[13px] text-[#5C6472] mt-1.5 mb-7">{t("login.description")}</p>

          <form onSubmit={handleSubmit} className="space-y-5">
            <div className="space-y-2">
              <Label htmlFor="email" className="text-[12px] font-semibold text-[#5C6472]">{t("login.account")}</Label>
              <Input
                id="email"
                type="text"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder={t("login.accountPlaceholder")}
                className="py-[13px] h-auto text-[14.5px]"
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password" className="text-[12px] font-semibold text-[#5C6472]">{t("login.password")}</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder={t("login.passwordPlaceholder")}
                className="py-[13px] h-auto text-[14.5px]"
                required
              />
            </div>
            {error && (
              <div className="p-3 bg-[#E5484D]/10 border border-[#E5484D]/20 rounded-xl text-[#C4372C] text-sm">{error}</div>
            )}
            {oauthError && (
              <div className="p-3 bg-[#E5484D]/10 border border-[#E5484D]/20 rounded-xl text-[#C4372C] text-sm">
                {t("login.oauthFail")}：{oauthError}
              </div>
            )}
            {oauthNotice && (
              <div className="p-3 bg-[#1BA878]/10 border border-[#1BA878]/20 rounded-xl text-[#0C7A55] text-sm">
                {oauthNotice}
              </div>
            )}
            <Button type="submit" disabled={loading} className="w-full py-[15px] h-auto text-[14.5px]">
              {loading ? t("login.submitting") : t("login.submit")}
            </Button>
            {(githubEnabled || wechatEnabled) && (
              <div className="flex items-center gap-3">
                <span className="flex-1 h-px bg-black/10" />
                <span className="text-[11px] text-[#8C93A1]">{t("login.or")}</span>
                <span className="flex-1 h-px bg-black/10" />
              </div>
            )}
          </form>
          {githubEnabled && (
            <a
              href="/api/oauth/github/authorize"
              className="mt-3 flex w-full items-center justify-center gap-2 rounded-xl border border-black/10 bg-white/70 py-[13px] text-[14px] font-semibold text-[#161A23] hover:bg-white/95 transition-colors"
            >
              <Github size={17} />
              {t("login.github")}
            </a>
          )}
          {wechatEnabled && (
            <a
              href="/api/oauth/wechat/authorize"
              className="mt-3 flex w-full items-center justify-center gap-2 rounded-xl border border-[#07C160]/30 bg-[#07C160]/10 py-[13px] text-[14px] font-semibold text-[#0a8f46] hover:bg-[#07C160]/15 transition-colors"
            >
              <MessageCircle size={17} />
              {t("login.wechat")}
            </a>
          )}
          {googleEnabled && (
            <a
              href="/api/oauth/google/authorize"
              className="mt-3 flex w-full items-center justify-center gap-2 rounded-xl border border-black/10 bg-white/70 py-[13px] text-[14px] font-semibold text-[#4285F4] hover:bg-white/95 transition-colors"
            >
              <Chrome size={17} />
              {t("login.google")}
            </a>
          )}
          <div className="flex items-center justify-between mt-5 text-[13px] text-[#5C6472]">
            <span>{t("login.register")} <Link to={REGISTER_TARGET} className="text-primary-700 font-semibold hover:underline">{t("login.registerNow")}</Link></span>
          </div>

          <div className="mt-auto pt-8">
            <div className="rounded-2xl bg-gradient-to-br from-[#F78B28]/10 via-transparent to-[#D97706]/10 border border-white/90 p-4">
              <div className="flex items-start gap-3">
                <span className="grid w-8 h-8 shrink-0 place-items-center rounded-[10px] bg-white/90 border border-white shadow-[inset_0_1px_0_rgba(255,255,255,0.95)] text-primary-700">
                  <ArrowRight size={15} strokeWidth={2.4} />
                </span>
                <div>
                  <div className="text-[13px] font-bold text-[#161A23]">{t("login.quickStart")}</div>
                  <div className="text-[12px] leading-relaxed text-[#5C6472] mt-0.5">{t("login.quickStartDesc")}</div>
                </div>
              </div>
            </div>
            <div className="text-center text-[11px] text-[#8B93A3] mt-4">
              {t("login.agreement")} ·{" "}
              <Link to="/rankings" className="text-primary-700 font-semibold hover:underline">
                {t("login.rankings")}
              </Link>
              {" "}·{" "}
              <Link to="/pricing" className="text-primary-700 font-semibold hover:underline">
                {t("login.pricing")}
              </Link>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
