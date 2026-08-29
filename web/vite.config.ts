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
    watch: {
      // Windows 宿主机 bind mount 不会向容器推送 inotify 事件，Vite 默认
      // 监听不到源码变更；改用轮询让 web 容器内的 dev server 热更新生效。
      usePolling: true,
      interval: 1000,
    },
    proxy: {
      "^/api/": { target: proxyTarget, changeOrigin: true },
      "/v1": { target: proxyTarget, changeOrigin: true },
      "/uploads": { target: proxyTarget, changeOrigin: true },
    },
  },
});
