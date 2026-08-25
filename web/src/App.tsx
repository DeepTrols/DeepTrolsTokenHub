import React from "react";
import { Routes, Route, Navigate } from "react-router-dom";
import ConsoleLayout from "./components/ConsoleLayout";
import AdminLayout from "./components/AdminLayout";
import RequireAuth from "./components/RequireAuth";
import Login from "./pages/Login";
import Register from "./pages/Register";
import Dashboard from "./pages/Dashboard";
import APIKeys from "./pages/APIKeys";
import Recharge from "./pages/Recharge";
import Bills from "./pages/Bills";
import ModelMarket from "./pages/ModelMarket";
import ModelManagement from "./pages/ModelManagement";
import Playground from "./pages/Playground";
import Tenants from "./pages/Tenants";
import Finance from "./pages/Finance";
import Channels from "./pages/Channels";
import Reconciliation from "./pages/Reconciliation";
import Users from "./pages/Users";
import RoutingSimulator from "./pages/RoutingSimulator";
import Guardrails from "./pages/Guardrails";
import BillingSync from "./pages/BillingSync";
import AuditLogs from "./pages/AuditLogs";
import GatewayHealth from "./pages/GatewayHealth";
import UserCenter from "./pages/UserCenter";
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
        <Route path="wallet" element={<Navigate to="/recharge" replace />} />
        <Route path="recharge" element={<Recharge />} />
        <Route path="bills" element={<Bills />} />
        <Route path="models" element={<ModelMarket />} />
        <Route path="playground" element={<Playground />} />
        <Route path="account" element={<UserCenter />} />
        <Route path="profile" element={<Navigate to="/account" replace />} />
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
        <Route path="providers" element={<Navigate to="/admin/channels" replace />} />
        <Route path="reconciliation" element={<Reconciliation />} />
        <Route path="users" element={<Users />} />
        <Route path="routing-simulator" element={<RoutingSimulator />} />
        <Route path="guardrails" element={<Guardrails />} />
        <Route path="billing" element={<BillingSync />} />
        <Route path="audit" element={<AuditLogs />} />
        <Route path="gateway-health" element={<GatewayHealth />} />
      </Route>
    </Routes>
  );
}
