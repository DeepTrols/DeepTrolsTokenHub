import codecs
c=codecs.open("DEEPTROLS_完整功能清单.md","r","utf-8").read()
c=c.replace("### 2026-07-31","### 2026-08-04 -- 补齐\n\n| 变更 | 之前 | 之后 |\n|------|------|------|\n| 响应缓存 | 无 | done |\n| 租户中间件 | fail-open | fail-closed |\n| 请求字段保护 | missing | done |\n| 配额管理API | readonly | CRUD |\n| APIKey删除 | broken | fixed |\n| 钱包充值 | basic | order+status+method |\n| 用户管理 | partial | full CRUD |\n| Docker | infra only | 5 containers |\n| Dev | none | Air+Vite HMR |\n| 前端UI | Tailwind | shadcn/ui 21 pages |\n\n### 2026-07-31")
c=c.replace("更新日期: 2026-07-31","更新日期: 2026-08-04")
codecs.open("DEEPTROLS_完整功能清单.md","w","utf-8").write(c)
print("ok")
