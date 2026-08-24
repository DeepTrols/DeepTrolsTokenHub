import js from "@eslint/js";
import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";
import globals from "globals";

const vitestGlobals = {
  describe: "readonly",
  it: "readonly",
  test: "readonly",
  expect: "readonly",
  vi: "readonly",
  beforeEach: "readonly",
  afterEach: "readonly",
  beforeAll: "readonly",
  afterAll: "readonly",
};

export default tseslint.config(
  {
    ignores: ["dist/**", "node_modules/**", "coverage/**", "_write-pages.js"],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ["**/*.{ts,tsx}"],
    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.node,
      },
    },
    plugins: {
      "react-hooks": reactHooks,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      // The codebase deliberately uses `any` in several API-boundary types;
      // the backend response shapes are validated at runtime, not compile time.
      "@typescript-eslint/no-explicit-any": "off",
      // react-hooks v7 的这两条新规则误伤本项目的既有模式：
      // - set-state-in-effect：对话框打开/数据到达时同步表单状态是常见写法，
      //   该规则只给性能建议，不构成正确性问题；
      // - purity：会把事件处理器里的 Date.now() 误判为 render 期调用。
      "react-hooks/set-state-in-effect": "off",
      "react-hooks/purity": "off",
    },
  },
  {
    files: ["**/*.test.{ts,tsx}", "**/test/**/*.{ts,tsx}", "src/test/**/*.{ts,tsx}"],
    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.node,
        ...vitestGlobals,
      },
    },
  },
);
