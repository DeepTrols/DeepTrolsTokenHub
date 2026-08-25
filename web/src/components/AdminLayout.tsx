import React from "react";
import { Outlet, NavLink, useNavigate } from "react-router-dom";
import {
  Box,
  BarChart3,
  LogOut,
  Shield,
  UserCog,
  DollarSign,
  Building2,
  Route,
} from "lucide-react";
import { useAuth } from "../lib/auth";
import RouteErrorBoundary from "./RouteErrorBoundary";

const manageItems = [
  { to: "/admin/models", icon: Box, label: "模型管理", color: "text-[#4F6BED]" },
  { to: "/admin/channels", icon: Shield, label: "渠道管理", color: "text-[#0FA88B]" },
  { to: "/admin/reconciliation", icon: BarChart3, label: "对账管理", color: "text-[#8B6FE8]" },
  { to: "/admin/routing-simulator", icon: Route, label: "路由模拟器", color: "text-[#12A5B0]" },
];

export default function AdminLayout() {
  const navigate = useNavigate();
  const { logout } = useAuth();

  const handleLogout = async () => {
    await logout();
    navigate("/login");
  };

  const linkClass = (isActive: boolean) =>
    `flex items-center gap-[10px] px-[11px] py-[8px] rounded-[11px] text-[14px] font-semibold border transition-all ${
      isActive
        ? "bg-white/80 border-white/95 text-[#4F6BED] shadow-[inset_0_1px_0_rgba(255,255,255,0.95),0_10px_26px_rgba(63,76,128,0.10)]"
        : "border-transparent text-[#5C6472] hover:text-[#161A23] hover:bg-white/40"
    }`;

  return (
    <div className="relative h-screen overflow-hidden">
      <div className="lg-orb w-[520px] h-[460px] bg-[#4F6BED]/20 -top-[170px] -right-[110px]" />
      <div className="lg-orb w-[460px] h-[420px] bg-[#0FA88B]/20 -bottom-[160px] -left-[130px]" />
      <div className="lg-orb w-[320px] h-[300px] bg-[#C9A96A]/14 top-[16%] left-[46%]" />

      <div className="relative z-10 flex h-full gap-5 p-5">
        <aside className="glass w-[220px] shrink-0 rounded-[20px] p-[12px] flex flex-col">
          <div className="px-2.5 pt-1.5 pb-6 border-b border-black/5 mb-5">
            <img src="/brand-logo.png" alt="DEEPTROLS" className="w-[168px] h-auto" />
          </div>

          <nav className="flex flex-col gap-[3px] overflow-y-auto pr-0.5" aria-label="管理导航">
            {manageItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                className={({ isActive }) => linkClass(isActive)}
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

            <NavLink
              to="/admin/tenants"
              className={({ isActive }) => linkClass(isActive)}
            >
              {({ isActive }) => (
                <>
                  <span className={`nav-ic !w-[30px] !h-[30px] !rounded-[9px] ${isActive ? "bg-gradient-to-br from-[#4F6BED] to-[#8B6FE8] text-white border-0 shadow-[0_6px_16px_rgba(79,107,237,0.35)]" : "text-[#0FA88B]"}`}>
                    <Building2 size={15} />
                  </span>
                  企业管理
                </>
              )}
            </NavLink>

            <NavLink
              to="/admin/users"
              className={({ isActive }) => linkClass(isActive)}
            >
              {({ isActive }) => (
                <>
                  <span className={`nav-ic !w-[30px] !h-[30px] !rounded-[9px] ${isActive ? "bg-gradient-to-br from-[#4F6BED] to-[#8B6FE8] text-white border-0 shadow-[0_6px_16px_rgba(79,107,237,0.35)]" : "text-[#4F6BED]"}`}>
                    <UserCog size={15} />
                  </span>
                  个人管理
                </>
              )}
            </NavLink>

            <NavLink
              to="/admin/finance"
              className={({ isActive }) => linkClass(isActive)}
            >
              {({ isActive }) => (
                <>
                  <span className={`nav-ic !w-[30px] !h-[30px] !rounded-[9px] ${isActive ? "bg-gradient-to-br from-[#4F6BED] to-[#8B6FE8] text-white border-0 shadow-[0_6px_16px_rgba(79,107,237,0.35)]" : "text-[#C9A96A]"}`}>
                    <DollarSign size={15} />
                  </span>
                  账务管理
                </>
              )}
            </NavLink>
          </nav>

          <div className="mt-auto pt-3 space-y-[3px]">
            <NavLink
              to="/dashboard"
              className="flex items-center gap-[10px] px-[9px] py-[7px] rounded-[10px] text-[14px] font-semibold text-[#5C6472] hover:text-[#161A23] hover:bg-white/40 transition-all"
            >
              <span className="nav-ic !w-[30px] !h-[30px] !rounded-[9px] text-[#0FA88B]"><Box size={15} /></span>
              用户门户
            </NavLink>
            <button
              onClick={handleLogout}
              className="flex items-center gap-[10px] w-full px-[9px] py-[7px] rounded-[10px] text-[14px] font-semibold text-[#5C6472] hover:text-[#E5484D] hover:bg-white/40 transition-all"
            >
              <span className="nav-ic !w-[30px] !h-[30px] !rounded-[9px] text-[#E5484D]"><LogOut size={15} /></span>
              退出登录
            </button>
          </div>
        </aside>

        <div className="flex-1 min-w-0 flex flex-col gap-4">
          <main className="flex-1 overflow-y-auto pr-1">
            <div className="px-1 pt-3 pb-8">
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
