import React from "react";
import { NavLink, Outlet } from "react-router-dom";
import { Palette, CreditCard, ShieldCheck, Settings2, Gauge, Info, MessageSquare, Box } from "lucide-react";
import { cn } from "@/lib/utils";
import "../i18n";
import { useTranslation } from "react-i18next";

const sections = [
  { to: "/admin/settings/site", labelKey: "settings.navSite", icon: Palette },
  { to: "/admin/settings/billing", labelKey: "settings.navBilling", icon: CreditCard },
  { to: "/admin/settings/security", labelKey: "settings.navSecurity", icon: ShieldCheck },
  { to: "/admin/settings/operations", labelKey: "settings.navOperations", icon: Settings2 },
  { to: "/admin/settings/content", labelKey: "settings.navContent", icon: MessageSquare },
  { to: "/admin/settings/models", labelKey: "settings.navModels", icon: Box },
  { to: "/admin/settings/request-limits", labelKey: "settings.navLimits", icon: Gauge },
  { to: "/admin/settings/system-info", labelKey: "settings.navSystem", icon: Info },
];

const linkClass = (active: boolean) =>
  cn(
    "flex items-center gap-[10px] px-[9px] py-[7px] rounded-[10px] text-[14px] font-semibold border transition-all",
    active
      ? "bg-white/80 border-white/95 text-[#4F6BED] shadow-[inset_0_1px_0_rgba(255,255,255,0.95),0_10px_26px_rgba(63,76,128,0.10)]"
      : "border-transparent text-[#5C6472] hover:text-[#161A23] hover:bg-white/40",
  );

export default function AdminSettingsLayout() {
  const { t } = useTranslation();
  return (
    <div className="flex gap-4">
      <aside className="glass w-[200px] shrink-0 rounded-[20px] p-[10px] flex flex-col self-start">
        <nav className="flex flex-col gap-[3px]" aria-label={t("settings.navAria")}>
          {sections.map((item) => (
            <NavLink key={item.to} to={item.to} className={({ isActive }) => linkClass(isActive)}>
              {({ isActive }) => (
                <>
                  <span
                    className={cn(
                      "nav-ic !w-[30px] !h-[30px] !rounded-[9px]",
                      isActive
                        ? "bg-gradient-to-br from-[#4F6BED] to-[#8B6FE8] text-white border-0 shadow-[0_6px_16px_rgba(79,107,237,0.35)]"
                        : "text-[#8B6FE8]",
                    )}
                  >
                    <item.icon size={15} />
                  </span>
                  {t(item.labelKey)}
                </>
              )}
            </NavLink>
          ))}
        </nav>
      </aside>
      <div className="flex-1 min-w-0">
        <Outlet />
      </div>
    </div>
  );
}
