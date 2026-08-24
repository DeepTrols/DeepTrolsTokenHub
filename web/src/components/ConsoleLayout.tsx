import React from "react";
import { Outlet, NavLink, useNavigate } from "react-router-dom";
import { Key, BarChart3, Wallet, Box, Play, LogOut, LayoutDashboard, FileText, Book, Settings, UserCircle, Users } from "lucide-react";
import { useAuth } from "../lib/auth";
import RouteErrorBoundary from "./RouteErrorBoundary";
import { PendingReviewBanner } from "./PendingReviewBanner";

export default function ConsoleLayout() {
  const navigate = useNavigate();
  const { user, logout } = useAuth();
  const isAdmin = user?.role === "admin";
  const isEnterpriseAdmin = user?.tenant_role === "owner" || user?.tenant_role === "admin";
  // Enterprise members have no self-service wallet: their balance is handed out
  // by the team admin, so the wallet menu is hidden for them (read-only spend).
  const isEnterpriseMember = user?.tenant_role === "member";

  const navItems = [
    { to: "/dashboard", icon: LayoutDashboard, label: "用量信息", color: "text-[#4F6BED]" },
    { to: "/api-keys", icon: Key, label: "API keys", color: "text-[#0FA88B]" },
    { to: "/logs", icon: FileText, label: "调用记录", color: "text-[#D3A94E]" },
    { to: "/usage", icon: BarChart3, label: "用量统计", color: "text-[#8B6FE8]" },
    ...(isEnterpriseMember
      ? []
      : [{ to: "/wallet", icon: Wallet, label: "钱包管理", color: "text-[#0FA88B]" }]),
    { to: "/models", icon: Box, label: "模型广场", color: "text-[#D3A94E]" },
    { to: "/playground", icon: Play, label: "在线体验", color: "text-[#4F6BED]" },
    { to: "/docs", icon: Book, label: "开发文档", color: "text-[#0FA88B]" },
    { to: "/profile", icon: UserCircle, label: "个人设置", color: "text-[#4F6BED]" },
    ...(isEnterpriseAdmin ? [{ to: "/team", icon: Users, label: "团队管理", color: "text-[#8B6FE8]" }] : []),
    ...(isAdmin ? [{ to: "/admin/models", icon: Settings, label: "管理控制台", color: "text-[#D3A94E]" }] : []),
  ];

  const handleLogout = async () => {
    await logout();
    navigate("/login");
  };

  const displayName = user?.name || user?.email?.split("@")[0] || "DeepTrols";
  const avatarChar = (user?.name || user?.email || "深").slice(0, 1).toUpperCase();

  return (
    <div className="relative h-screen overflow-hidden">
      <div className="lg-orb w-[520px] h-[460px] bg-[#4F6BED]/20 -top-[170px] -right-[110px]" />
      <div className="lg-orb w-[460px] h-[420px] bg-[#0FA88B]/20 -bottom-[160px] -left-[130px]" />
      <div className="lg-orb w-[320px] h-[300px] bg-[#C9A96A]/14 top-[16%] left-[46%]" />

      <div className="relative z-10 flex h-full gap-5 p-5">
        <aside className="glass w-[220px] shrink-0 rounded-[20px] p-[12px] flex flex-col">
          <div className="px-2.5 pt-1.5 pb-4 border-b border-black/5 mb-2">
            <img src="/brand-logo.png" alt="DEEPTROLS" className="w-[168px] h-auto" />
          </div>

          <nav className="flex flex-col gap-[3px] overflow-y-auto pr-0.5" aria-label="主导航">
            {navItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                className={({ isActive }) =>
                  `flex items-center gap-[10px] px-[9px] py-[7px] rounded-[10px] text-[13px] font-semibold border transition-all ${
                    isActive
                      ? "bg-white/80 border-white/95 text-[#4F6BED] shadow-[inset_0_1px_0_rgba(255,255,255,0.95),0_10px_26px_rgba(63,76,128,0.10)]"
                      : "border-transparent text-[#5C6472] hover:text-[#161A23] hover:bg-white/40"
                  }`
                }
              >
                {({ isActive }) => (
                  <>
                    <span className={`nav-ic !w-[30px] !h-[30px] !rounded-[9px] ${isActive ? "bg-gradient-to-br from-[#4F6BED] to-[#8B6FE8] text-white border-0 shadow-[0_6px_16px_rgba(79,107,237,0.35)]" : item.color}`}>
                      <item.icon size={15} />
                    </span>
                    {item.label}
                  </>
                )}
              </NavLink>
            ))}
          </nav>

          <div className="mt-auto pt-3">
            <div className="glass-soft flex items-center gap-2.5 rounded-xl p-2">
              <span className="grid w-9 h-9 shrink-0 place-items-center rounded-[10px] bg-gradient-to-br from-[#4F6BED] to-[#8B6FE8] text-white text-[12px] font-bold shadow-[0_6px_18px_rgba(79,107,237,0.35)]">{avatarChar}</span>
              <span className="flex-1 min-w-0">
                <div className="text-[12.5px] font-semibold truncate">{displayName}</div>
                <div className="text-[10.5px] text-[#5C6472]">{(user?.tenant_name || "DeepTrols")} · {isAdmin ? "管理员" : (isEnterpriseAdmin ? "企业管理员" : "成员")}</div>
              </span>
              <button onClick={handleLogout} aria-label="退出登录" className="grid w-7 h-7 place-items-center rounded-lg text-[#5C6472] hover:text-[#E5484D] hover:bg-white/70 transition-colors shrink-0">
                <LogOut size={14} />
              </button>
            </div>
          </div>
        </aside>

        <div className="flex-1 min-w-0 flex flex-col gap-4">
          <main className="flex-1 overflow-y-auto pr-1">
            <div className="px-1 pt-3 pb-8 space-y-6">
              <PendingReviewBanner tenantStatus={user?.tenant_status} />
              <RouteErrorBoundary>
                <Outlet />
              </RouteErrorBoundary>
            </div>
          </main>
        </div>
      </div>
    </div>
  );
}
