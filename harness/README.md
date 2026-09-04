# 智曜TokenHub Harness

这个目录是项目级验证工程，用来守住当前改造后的关键边界：

- Nuxt 前台仍按 `/ai/` 直接运行。
- 后台 API、后台控制台和 Nuxt 页面都能被本机访问。
- logo、站点名和 favicon 使用 `智曜TokenHub` 的当前资源。
- Nuxt 页面继续使用 TypeScript、Composition API、`<script setup>`、SCSS + Tailwind CSS v4 theme bridge。
- 旧静态目录和旧品牌文案不会重新混进页面层源码。

需要 Node.js 22.13+ 与 pnpm 10.31.0。`harness` 与 `ai-nuxt`、`web` 位于同一
Git 仓库根目录下，后端根目录就是仓库根目录，不再向下查找第二层 `DeepTrolsTokenHub`。
所有路径由当前模块文件位置解析，与调用命令时的工作目录无关。

## 使用方式

```sh
pnpm --dir harness run audit
pnpm --dir harness run smoke
pnpm --dir harness run check
```

也可以从项目根目录运行：

```sh
pnpm harness:audit
pnpm harness:smoke
pnpm harness:check
pnpm harness:typecheck
pnpm harness:test
```

## 命令说明

- `audit`：只做源码与结构审计，不要求本地服务启动。
- `smoke`：检查运行中的本地服务和公开接口。
- `check`：同时执行 `audit` 和 `smoke`，适合改完代码后做一次完整回归。
- `test`：验证单仓库路径、统一脚本、任意工作目录调用和品牌/源码守卫；品牌检查解析 HTML 标题与文本，不把开发资源路径当成页面文案。

运行态检查默认读取：

- 前台：`http://127.0.0.1:4173/ai`
- 后台 API：`http://127.0.0.1:8080`
- 后台控制台：`http://127.0.0.1:3000`

需要修改地址时，复制 `.env.example` 为 `.env` 后调整对应变量。

每次运行会输出终端报告，并写入 `harness/reports/latest.json`，方便后续排查。
