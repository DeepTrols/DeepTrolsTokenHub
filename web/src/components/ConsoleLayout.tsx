import React from "react";
import { Outlet, NavLink, useNavigate } from "react-router-dom";
import { Key, BarChart3, Wallet, Box, Play, Shield, LogOut, LayoutDashboard, FileText, Book, Settings, UserCircle, Users, Building2 } from "lucide-react";
import { useAuth } from "../lib/auth";
import RouteErrorBoundary from "./RouteErrorBoundary";

export default function ConsoleLayout() {
  const navigate = useNavigate();
  const { user, logout } = useAuth();
  const isAdmin = user?.role === "admin";
  const isEnterpriseAdmin = user?.tenant_role === "owner" || user?.tenant_role === "admin";

  const navItems = [
    { to: "/dashboard", icon: LayoutDashboard, label: "数据看板" },
    { to: "/api-keys", icon: Key, label: "API 密钥" },
    { to: "/models", icon: Box, label: "模型广场" },
    { to: "/playground", icon: Play, label: "在线体验" },
    { to: "/logs", icon: FileText, label: "调用记录" },
    { to: "/wallet", icon: Wallet, label: "钱包管理" },
    { to: "/profile", icon: UserCircle, label: "个人设置" },
    ...(isEnterpriseAdmin ? [
      { to: "/enterprise", icon: Building2, label: "企业设置" },
      { to: "/team", icon: Users, label: "团队管理" },
    ] : []),
    { to: "/security", icon: Shield, label: "安全设置" },
    { to: "/docs", icon: Book, label: "开发文档" },
  ];

  const handleLogout = async () => {
    await logout();
    navigate("/login");
  };

  return (
    <div className="flex h-screen">
      <aside className="w-64 bg-white border-r border-gray-200 flex flex-col">
        <div className="p-4 border-b border-gray-200">
          <h1 className="text-lg font-bold text-primary-700">DeepTrols</h1>
          <p className="text-xs text-gray-500">AI 模型聚合平台</p>
        </div>
        <nav className="flex-1 p-3 space-y-1">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) =>
                `flex items-center gap-3 px-3 py-2 rounded-lg text-sm transition-colors ${
                  isActive
                    ? "bg-primary-50 text-primary-700 font-medium"
                    : "text-gray-600 hover:bg-gray-100"
                }`
              }
            >
              <item.icon size={18} />
              {item.label}
            </NavLink>
          ))}
        </nav>
        {isAdmin && (
          <div className="p-3 border-t border-gray-200">
            <NavLink
              to="/admin/models"
              className="flex items-center gap-3 px-3 py-2 rounded-lg text-sm text-gray-600 hover:bg-gray-100"
            >
              <Settings size={18} />
              管理控制台
            </NavLink>
          </div>
        )}
        <div className="p-3 border-t border-gray-200">
          <button
            onClick={handleLogout}
            className="flex items-center gap-3 px-3 py-2 rounded-lg text-sm text-gray-600 hover:bg-gray-100 w-full"
          >
            <LogOut size={18} />
            退出登录
          </button>
        </div>
      </aside>
      <main className="flex-1 overflow-auto">
        <div className="p-6">
          <RouteErrorBoundary>
            <Outlet />
          </RouteErrorBoundary>
        </div>
      </main>
    </div>
  );
}
