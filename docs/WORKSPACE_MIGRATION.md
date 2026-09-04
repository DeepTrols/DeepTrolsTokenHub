# 单仓库迁移验收记录

日期：2026-09-04。目标仓库：`DeepTrols/DeepTrolsTokenHub`，分支：`main`。

## 迁移结果

- `ai-nuxt/`、`harness/` 和统一 `package.json` 已移入仓库根目录。
- 外层 README 已合并进仓库原 README，原有后端说明未被覆盖。
- `package.json` 保留统一安装、启动、构建、类型检查和回归测试入口。
- `ai-nuxt` 不再是嵌套 Git 仓库，GitHub 拉取可直接取得其完整源码。
- 原 Nuxt Git 元数据保存在本机 `.git/local-backups/ai-nuxt-20260904.git`，不上传。
- 之前未跟踪的 `docs/tasks/PROJECT_RECOMMENDATIONS_AND_DELIVERY_PLAN.md` 一并纳入版本管理，并更新同仓交付说明。
- Go 业务代码、数据库迁移、React 页面代码、Nuxt 页面路由与样式未修改。

Nuxt 原仓库提交为 `b465bbf`。迁移前 Git tree 与迁移后 `ai-nuxt/` 子树均为
`a5b7989d7a4e1af38962606e1e897686017209c9`，包括源码、图片、配置和锁文件，内容逐文件一致。

## 工程修正

- 根命令移除旧的第二层 `DeepTrolsTokenHub/` 路径；安装 Nuxt 后自动重新生成类型缓存。
- Harness 根路径按模块文件定位，不受调用命令所在目录影响。
- 品牌检查通过 parse5 解析 HTML 文本，避免将开发资源路径中的仓库名误判为旧品牌，也不将独立文本节点拼成新词。
- `backend:test` 复用现有测试数据库守卫，并只扫描自有 Go 源码目录，排除前端依赖内的 Go 示例。
- 添加 Nuxt/harness CI 作业；Git 与 Docker 构建上下文排除依赖、构建缓存、环境密钥文件和生成报告。

## 验收结果

| 检查 | 结果 |
| --- | --- |
| 根目录 Nuxt 类型检查与生产构建 | PASS |
| 根目录后台控制台生产构建 | PASS |
| 后台前端回归测试 | 41 个文件，268 项通过 |
| Harness 类型检查与回归 | PASS，8 项通过 |
| 源码审计与运行态检查 | 16 项通过，2 项提醒，0 项失败 |
| 自有 Go 包 `go vet` / `go build` | PASS |
| 未配置测试数据库时 `backend:test` | 正确失败，未启动数据库测试 |
| 仅由 Git 暂存源码导出的独立临时副本 | 三个 JavaScript 工程可按锁文件安装；Nuxt、后台构建及 harness 检查通过 |
| 干净 Nuxt 生产服务 | 6 个页面、3 个公开接口、logo 内容校验通过；控制中心保持跳转登录 |
| 浏览器检查 | 首页、模型列表切换、详情弹窗、厂商图标、后台登录页正常显示 |
| Git 提交边界 | 无 gitlink、node_modules、Nuxt 产物或 harness 生成报告 |

干净安装首次等待 npm 在线审计响应超过四分钟；仅对临时验收命令设置
`npm_config_fetch_timeout=15000 npm_config_fetch_retries=0` 后重试完成。
项目默认 npm 配置未变，此记录不代表依赖漏洞审计已通过。

## 保留限制

- `web` 保留原 npm lock；Nuxt/harness 使用 pnpm lock。没有借目录合并升级前台或后台依赖。
- Go 数据库集成测试未在本轮重跑：当前命令环境没有独立 `TEST_DATABASE_URL`。已验证现有 API 的 health、readyz 与公开配置，未改动运行数据库。
- 原前端 lint 问题、构建包体积及依赖弃用提醒不在本次迁移修复范围；本记录不表示远程 CI 全绿或已完成生产部署。
- Nuxt 原有本地模型数据快照继续保留；单仓库归并不等于完成前后端业务数据接入。

启动、生产进程与回滚步骤见 [DEPLOYMENT.md](DEPLOYMENT.md) 的「单仓库前端与 Harness」章节。
