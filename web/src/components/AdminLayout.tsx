import React from "react";
import { Outlet, NavLink, useNavigate } from "react-router-dom";
import { Box, Wallet, BarChart3, LogOut, Shield, ScrollText, UserCog, DollarSign, Building2, Route } from "lucide-react";
import { useAuth } from "../lib/auth";
import RouteErrorBoundary from "./RouteErrorBoundary";

const navItems = [
  { to: "/admin/models", icon: Box, label: "模型管理" },
  { to: "/admin/channels", icon: Shield, label: "渠道管理" },
  { to: "/admin/quotas", icon: Wallet, label: "配额管理" },
  { to: "/admin/reconciliation", icon: BarChart3, label: "对账管理" },
  { to: "/admin/audit", icon: ScrollText, label: "审计日志" },
  { to: "/admin/tenants", icon: Building2, label: "租户管理" },
  { to: "/admin/policies", icon: Route, label: "策略管理" },
  { to: "/admin/users", icon: UserCog, label: "用户管理" },
  { to: "/admin/costs", icon: DollarSign, label: "成本核算" },
];

export default function AdminLayout() {
  const navigate = useNavigate();
  const { logout } = useAuth();

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
        <nav className="flex-1 p-3 space-y-1">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) =>
                `flex items-center gap-3 px-3 py-2 rounded-lg text-sm transition-colors ${
                  isActive
                    ? "bg-gray-700 text-white font-medium"
                    : "text-gray-300 hover:bg-gray-800 hover:text-white"
                }`
              }
            >
              <item.icon size={18} />
              {item.label}
            </NavLink>
          ))}
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
