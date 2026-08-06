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
import Wallet from "./pages/Wallet";
import ModelMarket from "./pages/ModelMarket";
import ModelManagement from "./pages/ModelManagement";
import Playground from "./pages/Playground";
import Security from "./pages/Security";
import Tenants from "./pages/Tenants";
import Policies from "./pages/Policies";
import Channels from "./pages/Channels";
import QuotaManagement from "./pages/QuotaManagement";
import Reconciliation from "./pages/Reconciliation";
import AuditLogs from "./pages/AuditLogs";
import Users from "./pages/Users";
import Costs from "./pages/Costs";
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
        <Route path="wallet" element={<Wallet />} />
        <Route path="models" element={<ModelMarket />} />
        <Route path="playground" element={<Playground />} />
        <Route path="security" element={<Security />} />
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
        <Route path="policies" element={<Policies />} />
        <Route path="providers" element={<Navigate to="/admin/channels" replace />} />
        <Route path="quotas" element={<QuotaManagement />} />
        <Route path="reconciliation" element={<Reconciliation />} />
        <Route path="audit" element={<AuditLogs />} />
        <Route path="users" element={<Users />} />
        <Route path="costs" element={<Costs />} />
      </Route>
    </Routes>
  );
}
