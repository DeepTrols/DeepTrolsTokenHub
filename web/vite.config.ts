import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";

const proxyTarget = process.env.PROXY_TARGET || "http://localhost:8080";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 3000,
    proxy: {
      "^/api/": { target: proxyTarget, changeOrigin: true },
      "/v1": { target: proxyTarget, changeOrigin: true },
      "/uploads": { target: proxyTarget, changeOrigin: true },
    },
  },
});
