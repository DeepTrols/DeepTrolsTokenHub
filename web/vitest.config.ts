import path from "path";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    exclude: ["e2e/**", "node_modules/**", "dist/**"],
    coverage: {
      all: true,
      include: ["src/**/*.{ts,tsx}"],
      exclude: ["src/**/*.test.tsx", "src/test/**", "src/main.tsx", "src/vite-env.d.ts"],
    },
  },
});
