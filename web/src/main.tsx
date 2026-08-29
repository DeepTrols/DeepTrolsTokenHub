import React from "react";
import { Toaster } from "./components/ui/sonner";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { QueryClientProvider } from "@tanstack/react-query";
import { AuthProvider } from "./lib/auth";
import { queryClient } from "./lib/query-client";
import { SiteProvider } from "./lib/site";
import "./i18n";
import ErrorBoundary from "./components/ErrorBoundary";
import App from "./App";
import "./styles/index.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ErrorBoundary
      onError={(error, info) => {
        // TODO: Replace with Sentry.captureException / structured logger.
        console.error("[FATAL] Root boundary caught error", {
          message: error.message,
          stack: error.stack,
          componentStack: info.componentStack,
        });
      }}
    >
      <QueryClientProvider client={queryClient}>
        <SiteProvider>
          <BrowserRouter>
            <AuthProvider>
              <App />
              <Toaster />
            </AuthProvider>
          </BrowserRouter>
        </SiteProvider>
      </QueryClientProvider>
    </ErrorBoundary>
  </React.StrictMode>
);
