import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useAuth } from "../lib/auth";
import "../i18n";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SiteBrand } from "@/components/SiteBrand";

export default function Register() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const auth = useAuth();
  const [searchParams] = useSearchParams();
  const prefillInvite = searchParams.get("invite") || "";

  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [inviteCode, setInviteCode] = useState(prefillInvite);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      await auth.register(email, password, name, inviteCode.trim() || undefined);
      navigate("/dashboard");
    } catch (err) {
      setError(err instanceof Error ? err.message : t("register.fail"));
    }
    setLoading(false);
  };

  return (
    <div className="relative min-h-screen overflow-hidden flex items-center justify-center p-6">
      <div className="lg-orb w-[520px] h-[460px] bg-[#F78B28]/22 -top-[170px] -right-[110px]" />
      <div className="lg-orb w-[460px] h-[420px] bg-[#D97706]/20 -bottom-[160px] -left-[130px]" />
      <div className="lg-orb w-[320px] h-[300px] bg-[#D9A15D]/14 top-[16%] left-[46%]" />

      <div className="relative z-10 glass rounded-[22px] w-full max-w-md p-10">
        <div className="text-center pb-2">
          <SiteBrand
            className="mb-5 flex items-center justify-center gap-2.5"
            imageClassName="h-12 w-10 shrink-0 rounded-xl object-contain"
            textClassName="text-[24px] font-bold leading-none text-[#161A23]"
          />
          <p className="text-[13px] text-[#5C6472] mt-1">{t("register.desc")}</p>
        </div>
        <div>
          <form onSubmit={handleSubmit} className="space-y-4 mt-6">
            <div className="space-y-2">
              <Label htmlFor="name" className="text-[12px] font-semibold text-[#5C6472]">{t("register.name")}</Label>
              <Input id="name" value={name} onChange={e => setName(e.target.value)} placeholder={t("register.namePlaceholder")} required />
            </div>
            <div className="space-y-2">
              <Label htmlFor="email" className="text-[12px] font-semibold text-[#5C6472]">{t("register.email")}</Label>
              <Input id="email" type="email" value={email} onChange={e => setEmail(e.target.value)} placeholder={t("register.emailPlaceholder")} required />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password" className="text-[12px] font-semibold text-[#5C6472]">{t("register.password")}</Label>
              <Input id="password" type="password" value={password} onChange={e => setPassword(e.target.value)} placeholder={t("register.passwordPlaceholder")} required minLength={8} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="invite" className="text-[12px] font-semibold text-[#5C6472]">{t("register.invite")}</Label>
              <Input id="invite" value={inviteCode} onChange={e => setInviteCode(e.target.value)} placeholder="DTPxxxxxxxx" />
              <p className="text-xs text-[#5C6472]">{t("register.inviteHint")}</p>
            </div>
            {error && <div className="p-3 bg-[#E5484D]/10 border border-[#E5484D]/20 rounded-xl text-[#C4372C] text-sm">{error}</div>}
            <Button type="submit" disabled={loading} className="w-full" size="lg">{loading ? t("register.submitting") : t("register.submit")}</Button>
          </form>
          <p className="text-center text-xs text-[#5C6472] mt-6">{t("register.hasAccount")} <Link to="/login" className="text-primary-700 font-semibold hover:underline">{t("register.loginNow")}</Link></p>
        </div>
      </div>
    </div>
  );
}
