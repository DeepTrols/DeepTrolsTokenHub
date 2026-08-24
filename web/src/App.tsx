import React from "react";
import { Routes, Route, Navigate } from "react-router-dom";
import ConsoleLayout from "./components/ConsoleLayout";
import AdminLayout from "./components/AdminLayout";
import RequireAuth from "./components/RequireAuth";
import Login from "./pages/Login";
import Register from "./pages/Register";
import Dashboard from "./pages/Dashboard";
import APIKeys from "./pages/APIKeys";
import CallLogs from "./pages/CallLogs";
import UsageStats from "./pages/UsageStats";
import Recharge from "./pages/Recharge";
import Bills from "./pages/Bills";
import ModelMarket from "./pages/ModelMarket";
import ModelManagement from "./pages/ModelManagement";
import Playground from "./pages/Playground";
import Tenants from "./pages/Tenants";
import Finance from "./pages/Finance";
import Policies from "./pages/Policies";
import Channels from "./pages/Channels";
import Reconciliation from "./pages/Reconciliation";
import Users from "./pages/Users";
import Costs from "./pages/Costs";
import Profile from "./pages/Profile";
import TeamManagement from "./pages/TeamManagement";
import Docs from "./pages/Docs";

export default function App() {
  return (
    <Routes>
      {/* Public routes */}
      <Route path="/login" element={<Login />} />
      <Route path="/register" element={<Register />} />

      {/* User Console (JWT role=user or admin) */}
      <Route
        path="/"
        element={
          <RequireAuth>
            <ConsoleLayout />
          </RequireAuth>
        }
      >
        <Route index element={<Navigate to="/dashboard" replace />} />
        <Route path="dashboard" element={<Dashboard />} />
        <Route path="api-keys" element={<APIKeys />} />
        <Route path="logs" element={<CallLogs />} />
        <Route path="usage" element={<UsageStats />} />
        <Route path="wallet" element={<Navigate to="/recharge" replace />} />
        <Route path="recharge" element={<Recharge />} />
        <Route path="bills" element={<Bills />} />
        <Route path="models" element={<ModelMarket />} />
        <Route path="playground" element={<Playground />} />
        <Route path="profile" element={<Profile />} />
        <Route path="team" element={
          <RequireAuth tenantAdminOnly>
            <TeamManagement />
          </RequireAuth>
        } />
        <Route path="docs" element={<Docs />} />
      </Route>

      {/* Admin Portal (JWT role=admin only) */}
      <Route
        path="/admin"
        element={
          <RequireAuth adminOnly>
            <AdminLayout />
          </RequireAuth>
        }
      >
        <Route index element={<Navigate to="/admin/models" replace />} />
        <Route path="models" element={<ModelManagement />} />
        <Route path="channels" element={<Channels />} />
        <Route path="tenants" element={<Tenants />} />
        <Route path="finance" element={<Finance />} />
        <Route path="policies" element={<Policies />} />
        <Route path="providers" element={<Navigate to="/admin/channels" replace />} />
        <Route path="reconciliation" element={<Reconciliation />} />
        <Route path="users" element={<Users />} />
        <Route path="costs" element={<Costs />} />
      </Route>
    </Routes>
  );
}
