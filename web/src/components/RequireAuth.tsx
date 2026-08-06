import React from "react";
import { Navigate } from "react-router-dom";
import { useAuth } from "../lib/auth";

type Props = {
  children: React.ReactNode;
  adminOnly?: boolean;
};

export default function RequireAuth({ children, adminOnly = false }: Props) {
  const { user, isLoading, isAuthenticated } = useAuth();

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
      </div>
    );
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  if (adminOnly && user?.role !== "admin") {
    return <Navigate to="/dashboard" replace />;
  }

  return <>{children}</>;
}
