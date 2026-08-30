import React from "react";
import { Outlet, NavLink, useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { setLanguage } from "../i18n";
import {
  Key, Wallet, Receipt, Box, Play, LogOut, LayoutDashboard, Book,
  UserCircle, Radio, Users, Settings, Ticket, Crown,
  type LucideIcon,
} from "lucide-react";
import { useAuth } from "../lib/auth";
import { isAdmin } from "../lib/domain/navigation";
import { SiteBrand } from "./SiteBrand";
import { NoticeBanner } from "./NoticeBanner";
import { SiteFooter } from "./SiteFooter";
import RouteErrorBoundary from "./RouteErrorBoundary";
import { PendingReviewBanner } from "./PendingReviewBanner";

interface NavItem {
  to: string;
  icon: LucideIcon;
  labelKey: string;
  color: string;
}

const userItems: NavItem[] = [
  { to: "/dashboard", icon: LayoutDashboard, labelKey: "nav.dashboard", color: "text-[#4F6BED]" },
  { to: "/api-keys", icon: Key, labelKey: "nav.apiKeys", color: "text-[#0FA88B]" },
  { to: "/recharge", icon: Wallet, labelKey: "nav.recharge", color: "text-[#0FA88B]" },
  { to: "/subscriptions", icon: Crown, labelKey: "nav.subscriptions", color: "text-[#D3A94E]" },
  { to: "/bills", icon: Receipt, labelKey: "nav.bills", color: "text-[#0FA88B]" },
  { to: "/models", icon: Box, labelKey: "nav.models", color: "text-[#D3A94E]" },
  { to: "/playground", icon: Play, labelKey: "nav.playground", color: "text-[#4F6BED]" },
  { to: "/docs", icon: Book, labelKey: "nav.docs", color: "text-[#0FA88B]" },
  { to: "/account", icon: UserCircle, labelKey: "nav.account", color: "text-[#4F6BED]" },
];

const adminItems: NavItem[] = [
  { to: "/admin/channels", icon: Radio, labelKey: "nav.channels", color: "text-[#0FA88B]" },
  { to: "/admin/models", icon: Box, labelKey: "nav.modelMgmt", color: "text-[#4F6BED]" },
  { to: "/admin/redemption", icon: Ticket, labelKey: "nav.redemption", color: "text-[#D3A94E]" },
  { to: "/admin/subscription-plans", icon: Crown, labelKey: "nav.plans", color: "text-[#D3A94E]" },
  { to: "/admin/subscriptions", icon: Receipt, labelKey: "nav.subRecords", color: "text-[#8B6FE8]" },
  { to: "/admin/users", icon: Users, labelKey: "nav.users", color: "text-[#D3A94E]" },
  { to: "/admin/settings", icon: Settings, labelKey: "nav.settings", color: "text-[#8B6FE8]" },
];

function NavLinkItem({ item }: { item: NavItem }) {
  const Icon = item.icon;
  const { t } = useTranslation();
  return (
    <NavLink
      to={item.to}
      className={({ isActive }) =>
        `flex items-center gap-[10px] px-[9px] py-[7px] rounded-[10px] text-[14px] font-semibold border transition-all ${
          isActive
            ? "bg-white/80 border-white/95 text-[#4F6BED] shadow-[inset_0_1px_0_rgba(255,255,255,0.95),0_10px_26px_rgba(63,76,128,0.10)]"
            : "border-transparent text-[#5C6472] hover:text-[#161A23] hover:bg-white/40"
        }`
      }
    >
      {({ isActive }) => (
        <>
          <span className={`nav-ic !w-[30px] !h-[30px] !rounded-[9px] ${isActive ? "bg-gradient-to-br from-[#4F6BED] to-[#8B6FE8] text-white border-0 shadow-[0_6px_16px_rgba(79,107,237,0.35)]" : item.color}`}>
            <Icon size={15} />
          </span>
          {t(item.labelKey)}
        </>
      )}
    </NavLink>
  );
}

export default function ShellLayout() {
  const navigate = useNavigate();
  const { t, i18n } = useTranslation();
  const { user, logout } = useAuth();
  const admin = isAdmin(user);

  const handleLogout = async () => {
    await logout();
    navigate("/login");
  };

  const displayName = user?.name || user?.email?.split("@")[0] || "DeepTrols";
  const avatarChar = (user?.name || user?.email || "深").slice(0, 1).toUpperCase();
  const currentLang = i18n.language?.startsWith("en") ? "en" : "zh-CN";

  return (
    <div className="relative h-screen overflow-hidden">
      <div className="lg-orb w-[520px] h-[460px] bg-[#4F6BED]/20 -top-[170px] -right-[110px]" />
      <div className="lg-orb w-[460px] h-[420px] bg-[#0FA88B]/20 -bottom-[160px] -left-[130px]" />
      <div className="lg-orb w-[320px] h-[300px] bg-[#C9A96A]/14 top-[16%] left-[46%]" />

      <div className="relative z-10 flex h-full gap-5 p-5">
        <aside className="glass w-[220px] shrink-0 rounded-[20px] p-[12px] flex flex-col">
          <div className="px-2.5 pt-1.5 pb-6 border-b border-black/5 mb-5">
            <SiteBrand />
          </div>

          <nav className="flex flex-col gap-[3px] overflow-y-auto pr-0.5" aria-label={t("components.shellNavAria")}>
            {userItems.map((item) => (
              <NavLinkItem key={item.to} item={item} />
            ))}

            {admin && (
              <>
                <div className="px-[10px] pt-4 pb-1 text-[11px] font-semibold tracking-wide text-[#8C93A1]">
                  {t("nav.admin")}
                </div>
                {adminItems.map((item) => (
                  <NavLinkItem key={item.to} item={item} />
                ))}
              </>
            )}
          </nav>

          <div className="mt-auto pt-3">
            <div className="glass-soft flex items-center gap-2.5 rounded-xl p-2">
              <span className="grid w-9 h-9 shrink-0 place-items-center rounded-[10px] bg-gradient-to-br from-[#4F6BED] to-[#8B6FE8] text-white text-[12px] font-bold shadow-[0_6px_18px_rgba(79,107,237,0.35)]">{avatarChar}</span>
              <span className="flex-1 min-w-0">
                <div className="text-[13px] font-semibold truncate">{displayName}</div>
                <div className="text-[11px] text-[#5C6472] truncate">{user?.email || "DeepTrols"} · {t(admin ? "nav.roleAdmin" : "nav.roleUser")}</div>
              </span>
              <button
                onClick={() => (currentLang === "zh-CN" ? setLanguage("en") : setLanguage("zh-CN"))}
                aria-label="Switch language"
                className="grid w-7 h-7 place-items-center rounded-lg text-[#5C6472] hover:text-[#4F6BED] hover:bg-white/70 transition-colors shrink-0 text-[11px] font-bold"
              >
                {currentLang === "zh-CN" ? "EN" : "中"}
              </button>
              <button onClick={handleLogout} aria-label={t("nav.logout")} className="grid w-7 h-7 place-items-center rounded-lg text-[#5C6472] hover:text-[#E5484D] hover:bg-white/70 transition-colors shrink-0">
                <LogOut size={14} />
              </button>
            </div>
          </div>
        </aside>

        <div className="flex-1 min-w-0 flex flex-col gap-4">
          <main className="flex-1 overflow-y-auto pr-1">
            <div className="px-1 pt-3 pb-8 space-y-6">
              <NoticeBanner />
              <PendingReviewBanner tenantStatus={user?.tenant_status} />
              <RouteErrorBoundary>
                <Outlet />
              </RouteErrorBoundary>
              <SiteFooter />
            </div>
          </main>
        </div>
      </div>
    </div>
  );
}
