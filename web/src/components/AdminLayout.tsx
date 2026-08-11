import React, { useState } from "react";
import { Outlet, NavLink, useLocation, useNavigate } from "react-router-dom";
import {
  Box,
  Wallet,
  BarChart3,
  LogOut,
  Shield,
  ScrollText,
  UserCog,
  DollarSign,
  Building2,
  Route,
  Calculator,
  ChevronDown,
} from "lucide-react";
import { useAuth } from "../lib/auth";
import RouteErrorBoundary from "./RouteErrorBoundary";

const navItems = [
  { to: "/admin/models", icon: Box, label: "模型管理" },
  { to: "/admin/channels", icon: Shield, label: "渠道管理" },
  { to: "/admin/quotas", icon: Wallet, label: "配额管理" },
  { to: "/admin/reconciliation", icon: BarChart3, label: "对账管理" },
  { to: "/admin/audit", icon: ScrollText, label: "审计日志" },
  { to: "/admin/policies", icon: Route, label: "策略管理" },
  { to: "/admin/costs", icon: Calculator, label: "成本核算" },
];

/** 用户管理展开后的子项：企业管理 / 个人管理。 */
const userGroupItems = [
  { to: "/admin/tenants", icon: Building2, label: "企业管理" },
  { to: "/admin/users", icon: UserCog, label: "个人管理" },
];

function linkClass(isActive: boolean) {
  return `flex items-center gap-3 px-3 py-2 rounded-lg text-sm transition-colors ${
    isActive
      ? "bg-gray-700 text-white font-medium"
      : "text-gray-300 hover:bg-gray-800 hover:text-white"
  }`;
}

export default function AdminLayout() {
  const navigate = useNavigate();
  const location = useLocation();
  const { logout } = useAuth();
  const [usersOpen, setUsersOpen] = useState(
    () =>
      location.pathname.startsWith("/admin/tenants") ||
      location.pathname.startsWith("/admin/users"),
  );

  const handleLogout = async () => {
    await logout();
    navigate("/login");
  };

  return (
    <div className="flex h-screen">
      <aside className="w-64 bg-gray-900 border-r border-gray-700 flex flex-col">
        <div className="p-4 border-b border-gray-700">
          <h1 className="text-lg font-bold text-white">DeepTrols</h1>
          <p className="text-xs text-gray-400">管理控制台</p>
        </div>
        <nav className="flex-1 p-3 space-y-1 overflow-y-auto">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) => linkClass(isActive)}
            >
              <item.icon size={18} />
              {item.label}
            </NavLink>
          ))}

          {/* 用户管理：展开显示企业管理 / 个人管理 */}
          <div>
            <button
              type="button"
              onClick={() => setUsersOpen((o) => !o)}
              aria-expanded={usersOpen}
              className="flex items-center gap-3 w-full px-3 py-2 rounded-lg text-sm text-gray-300 hover:bg-gray-800 hover:text-white transition-colors"
            >
              <UserCog size={18} />
              <span className="flex-1 text-left">用户管理</span>
              <ChevronDown
                size={16}
                className={`transition-transform ${usersOpen ? "" : "-rotate-90"}`}
              />
            </button>
            {usersOpen && (
              <div className="mt-1 ml-3 space-y-1 border-l border-gray-700 pl-2">
                {userGroupItems.map((child) => (
                  <NavLink
                    key={child.to}
                    to={child.to}
                    className={({ isActive }) => linkClass(isActive)}
                  >
                    <child.icon size={16} />
                    {child.label}
                  </NavLink>
                ))}
              </div>
            )}
          </div>

          {/* 账务管理：独立入口 */}
          <NavLink to="/admin/finance" className={({ isActive }) => linkClass(isActive)}>
            <DollarSign size={18} />
            账务管理
          </NavLink>
        </nav>
        <div className="p-4 border-t border-gray-700 space-y-2">
          <NavLink
            to="/dashboard"
            className="flex items-center gap-3 px-3 py-2 rounded-lg text-sm text-gray-400 hover:bg-gray-800 hover:text-white transition-colors"
          >
            <Box size={18} />
            用户门户
          </NavLink>
          <button
            onClick={handleLogout}
            className="flex items-center gap-3 px-3 py-2 rounded-lg text-sm text-gray-400 hover:bg-gray-800 hover:text-white w-full transition-colors"
          >
            <LogOut size={18} />
            退出登录
          </button>
        </div>
      </aside>
      <main className="flex-1 overflow-auto bg-gray-50">
        <div className="p-6">
          <RouteErrorBoundary>
            <Outlet />
          </RouteErrorBoundary>
        </div>
      </main>
    </div>
  );
}
