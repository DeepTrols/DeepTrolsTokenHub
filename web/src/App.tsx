import React from "react";
import { Routes, Route, Navigate } from "react-router-dom";
import ShellLayout from "./components/ShellLayout";
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
import Channels from "./pages/Channels";
import Reconciliation from "./pages/Reconciliation";
import Users from "./pages/Users";
import AuditLogs from "./pages/AuditLogs";
import GatewayHealth from "./pages/GatewayHealth";
import UserCenter from "./pages/UserCenter";
import Docs from "./pages/Docs";
import UsageHistory from "./pages/UsageHistory";
import AdminSettingsLayout from "./components/AdminSettingsLayout";
import SiteSettingsSection from "./pages/settings/SiteSettingsSection";
import BillingSettingsSection from "./pages/settings/BillingSettingsSection";
import SecuritySettingsSection from "./pages/settings/SecuritySettingsSection";
import OperationsSettingsSection from "./pages/settings/OperationsSettingsSection";
import RequestLimitsSettingsSection from "./pages/settings/RequestLimitsSettingsSection";
import SystemInfoSection from "./pages/settings/SystemInfoSection";
import About from "./pages/About";
import Legal from "./pages/Legal";
import RedemptionCodes from "./pages/RedemptionCodes";
import Rankings from "./pages/Rankings";
import Chat from "./pages/Chat";
import ChatPresetsSection from "./pages/settings/ChatPresetsSection";
import ModelsSettingsSection from "./pages/settings/ModelsSettingsSection";
import Pricing from "./pages/Pricing";
import Subscriptions from "./pages/Subscriptions";
import AdminSubscriptionPlans from "./pages/AdminSubscriptionPlans";
import AdminSubscriptions from "./pages/AdminSubscriptions";

export default function App() {
  return (
    <Routes>
      {/* Public routes */}
      <Route path="/login" element={<Login />} />
      <Route path="/register" element={<Register />} />
      <Route path="/about" element={<About />} />
      <Route path="/rankings" element={<Rankings />} />
      <Route path="/pricing" element={<Pricing />} />
      <Route path="/legal/user-agreement" element={<Legal kind="user_agreement" />} />
      <Route path="/legal/privacy-policy" element={<Legal kind="privacy_policy" />} />

      {/* User Console (JWT role=user or admin) */}
      <Route
        path="/"
        element={
          <RequireAuth>
            <ShellLayout />
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
        <Route path="usage" element={<UsageHistory />} />
        <Route path="chat" element={<Chat />} />
        <Route path="subscriptions" element={<Subscriptions />} />
      </Route>

      {/* Admin Portal (JWT role=admin only) */}
      <Route
        path="/admin"
        element={
          <RequireAuth adminOnly>
            <ShellLayout />
          </RequireAuth>
        }
      >
        <Route index element={<Navigate to="/admin/models" replace />} />
        <Route path="models" element={<ModelManagement />} />
        <Route path="channels" element={<Channels />} />
        <Route path="tenants" element={<Tenants />} />
        <Route path="providers" element={<Navigate to="/admin/channels" replace />} />
        <Route path="reconciliation" element={<Reconciliation />} />
        <Route path="users" element={<Users />} />
        <Route path="audit" element={<AuditLogs />} />
        <Route path="gateway-health" element={<GatewayHealth />} />
        <Route path="redemption" element={<RedemptionCodes />} />
        <Route path="subscription-plans" element={<AdminSubscriptionPlans />} />
        <Route path="subscriptions" element={<AdminSubscriptions />} />
        <Route path="settings" element={<AdminSettingsLayout />}>
          <Route index element={<Navigate to="/admin/settings/site" replace />} />
          <Route path="site" element={<SiteSettingsSection />} />
          <Route path="billing" element={<BillingSettingsSection />} />
          <Route path="security" element={<SecuritySettingsSection />} />
          <Route path="operations" element={<OperationsSettingsSection />} />
          <Route path="content" element={<ChatPresetsSection />} />
          <Route path="models" element={<ModelsSettingsSection />} />
          <Route path="request-limits" element={<RequestLimitsSettingsSection />} />
          <Route path="system-info" element={<SystemInfoSection />} />
        </Route>
      </Route>
    </Routes>
  );
}
