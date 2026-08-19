# AI Token 聚合平台 · 完整架构文档

> **执行层现状（2026-08-19）**：本文是设计决策记录，正文章节仍以 LiteLLM 作为执行层前提展开讨论。
> 代码现状已变更：内置 LiteLLM 已于 2026-08-19 从 docker-compose 移除，网关由渠道实例
> base_url 直连上游（OpenAI 兼容），不再经过 LiteLLM；HMAC 请求签名重新定位为"可选能力，
> 仅建议用于平台回调/webhook 验签"，不适用于 OpenAI 兼容网关入口。正文中"LiteLLM 转发 /
> spend 证据 / HMAC 企业客户签名"等描述属于原设计意图，落地以代码与
> docs/PROJECT_STATUS.md 十九节为准。

## 目录

- **第1篇：开篇：从0到1打造一个AI Token聚合平台**
- 为什么“能调通”离“能收费”很远
- 企业级 Token 聚合平台的四层架构
- 第一层：控制面，不只是 API Key
- 第二层：执行面，LiteLLM 是代理层，不是平台账本
- 第三层：资金面，请求前要锁预算，请求后要能解释
- 第四层：证据面，没有证据包，就不能上线
- 这个专题会写些什么
- 一张检查清单
- **第2篇：网关核心：一条能收费的请求链路**
- 一个 handler 为什么不够
- 一条能收费的请求链路
- 入口收口：先建立账务身份
- 路由决策：模型名只是输入，不是结果
- 执行转发：非流式和流式不是同一件事
- 资金证据：HTTP 返回不代表生命周期结束
- 我们在实现里保留的 5 个不变量
- 1. request_id 不是全局唯一账务身份
- 2. 预算预留必须发生在上游调用前
- 3. 路由结果必须进入证据链
- 4. usage 来源必须显式标记
- 5. 流式错误不能伪装成正常成功
- 常见错误方案
- 错误一：把 OpenAI-compatible 当纯协议转换
- 错误二：request_id 只用于日志
- 错误三：流式中途错误用正常结束收尾
- 错误四：usage 缺失就静默估算
- 错误五：只写 usage log，不写 charge line
- 一条能收费的 AI 网关请求链路检查清单
- 结尾
- **第3篇：架构抉择：NewAPI、LiteLLM 与自研原生 Adapter 的边界**
- 先分清四层责任
- 什么时候可以选 NewAPI
- 什么时候选 LiteLLM
- 第一，AetherSpace 先做平台侧控制
- 第二，AetherSpace 只选择一个平台 channel
- 第三，AetherSpace 把多池路由留在自己手里
- 第四，LiteLLM spend 只是证据，不是客户最终账本
- 什么时候必须自研原生 Adapter
- 例子一：Gemini Native 图片
- 例子二：Doubao Seedance 视频
- 三种选择不是互斥关系
- NewAPI、LiteLLM、自研 Adapter 对照表
- 一个更实用的决策树
- 常见的五个错误方案
- 错误一：把 NewAPI、LiteLLM、自研 Adapter 当互斥选择
- 错误二：把 OpenAI-compatible 当所有 provider 的最终协议
- 错误三：把代理层 spend 当客户账本
- 错误四：新模型一上线就承诺 full native compatibility
- 错误五：自研 Adapter 只写 HTTP client
- 接入新 provider 前的 12 项检查清单
- 结尾
- **第4篇：计费引擎：从用户等级、OEM 到完整扣费链路**
- 01 先拆五层：定价、折扣、配额、钱包、账本
- 02 定价体系：客户价和上游成本是两条线
- 03 折扣体系：用户等级不是直接改模型单价
- 04 租户/OEM：不是加一个 tenant_id 就完事
- 05 完整扣费流程：一次请求从预估到落账
- 预估费用不是最终费用
- 预留是为了并发保护
- 流式请求也要完成计费闭环
- 06 配额、钱包和折扣：顺序不能乱
- 07 账本落点：usage log 和 charge line 都要有
- 08 常见错误方案
- 错误一：只有一张模型单价表
- 错误二：把折扣直接写进模型价格
- 错误三：套餐额度也打折
- 错误四：请求后才检查余额
- 错误五：把上游账单当客户账单
- 错误六：只写一个 cost 字段
- 错误七：OEM 和平台直客共用一套等级折扣
- 09 计费引擎上线前 18 项检查清单
- 10 最后
- **第5篇：OEM体系：设计一个企业级 Token 聚合平台OEM体系**
- 01 先把 OEM 拆成 4 个角色，而不是 1 个 tenant_id
- 02 租户开通不是后台 CRUD，而是状态机
- 03 租户识别：Host 优先，未知 Host 不能自动落到平台
- 04 多租户隔离：显式 tenant scope，而不是 ORM 魔法
- 05 OEM 路由能力：把 Admin、Owner、客户入口分开
- 06 AI 模型选品：平台模型目录不等于租户可售目录
- 07 客户等级：OEM 自己的等级，不复用平台会员等级
- 08 客户管理：封禁、调级、代充值都必须带租户边界
- 09 钱包额度和 AI 配额池必须分开
- 10 API Key 代管：当前隐藏，不半开放
- 11 账本可见性：OEM 要经营报表，不等于看平台全部账
- 12 当前 OEM 功能实现清单
- 13 常见错误方案
- 错误一：把 OEM 做成一套白标前端
- 错误二：公开请求靠参数传 tenant_id
- 错误三：平台模型默认给所有 OEM 可售
- 错误四：平台等级和 OEM 客户等级混用
- 错误五：代充值直接改客户余额
- 错误六：把钱包额度和 AI 配额池叫成一个东西
- 错误七：提前开放 OEM 代管 API Key
- 错误八：OEM 看板直接展示平台上游成本
- 14 最后
- **第6篇：后端架构：Token转发后端架构与LLM渠道高可用设计**
- 先把两类高可用分开
- 为什么当前没有一上来拆微服务
- 当前不是“大单体”，而是模块化单体
- API 和 Worker 是运行角色，不是两个业务服务
- Worker 的工程价值，不是“异步”两个字
- 第一，支持长任务和定时任务
- 第二，定时任务支持 singleton
- 第三，API 和 Worker 之间通过队列抽象解耦
- 第四，异步计费使用 durable outbox
- 为未来拆微服务做了哪些铺垫
- 什么时候才应该真正转微服务
- LLM 渠道高可用，不能靠后端微服务解决
- 转发实例只做转发
- channel 才是平台的流量调度单元
- 共享池、租户独享池和混合池
- 路由不是 if/else，而是 RouteContext + Route Policy
- 默认路由：先租户绑定池，再共享池
- 负载均衡：weighted least-load，不是简单轮询
- 健康监测：Worker 管状态，不塞进请求线程
- 同模型多上游怎么配置
- API / Worker 和渠道架构如何配合
- 常见错误设计
- 第一种错误：业务还没稳定就拆微服务
- 第二种错误：把 Worker 当成另一个服务随便写业务逻辑
- 第三种错误：把转发实例做成业务系统
- 第四种错误：channel 只是 provider key 的别名
- 第五种错误：独享池不可用时自动回共享池
- 第六种错误：用 DB current_load 做实时路由
- 第七种错误：平台层 A 主 B 备跨 channel 重试
- 架构检查清单
- API / Worker：
- 微服务拆分：
- LLM 渠道：
- 结尾
- **第7篇：对账系统：如何让Token中转平台的每一条收费都经得起对账考验**
- 对账难在哪里
- 上游账单不能直接当客户账单
- 跨协议转换后，账怎么对
- Usage 必须先归一化
- 四层对账模型
- L0：客户请求账本
- L1：上游转发证据
- L2：L0 和 L1 的内部对账
- L3：L1 和上游账单的内部对账
- 案例一：平台成功，但转发层证据缺失
- 案例二：上游 504，平台失败但没有扣费
- 案例三：平台失败，但转发层已经成功计费
- 案例四：总量一致，但明细维度不一致
- 修正不能止于接口成功
- 对账任务不能直接改钱包
- 产品经理应该关心哪些对账能力
- 最后
- **第8篇：控制台：API Key 权限、用量控制与调用日志分析**
- API Key 管理的 6 个边界
- 创建 Key：明文只展示一次，边界要一起配置
- Key 状态：激活、停用、撤销、超限必须分清
- 模型白名单：不要让测试 Key 打到生产模型
- 来源白名单：全局默认 + 单 Key 覆盖
- 消费限额：累计、每周、每月和超限策略要分开
- Key 不能孤立：要接上日志、统计和账单
- 调用日志：客户侧账本入口
- 用量统计：采购看趋势，技术看异常
- 钱包账单：资金动作要能追到 AI 用量
- 模型广场要放在 Key 后面
- 模型详情要把价格讲透
- Playground 是上线前验证台
- 文档中心要和控制台互相跳转
- 安全设置不能后补
- 页面按任务拆，对象按事实拆
- 企业级 token 控制台检查清单
- 最后
- **第9篇：接口设计：Token聚合平台的接口设计与实现逻辑**
- 先看结论：16 条路由不是一条转发链
- 一张图看懂整体架构
- 模型出现在目录里，不等于客户一定能调用
- 平台选择 Channel，转发实例处理 Provider 级重试
- 模型支持面要拆成三层
- 第一层：通用目录模型
- 第二层：特殊能力契约
- 第三层：运行期可用性
- 16 个对外接口先看全表
- 三类协议入口如何鉴权
- 逐个拆开 16 个接口的处理流程
- 5.1 GET /v1/models
- 5.2 通用同步推理主链
- 5.3 POST /v1/chat/completions
- 5.4 POST /v1/responses
- 5.5 POST /v1/messages 与 /v1/messages/count_tokens
- 5.5.1 GLM-5.2 经 Anthropic Messages 的 usage 与计费适配
- 5.6 Chat / Responses / Messages / Images 的流式主链
- 5.7 POST /v1/embeddings
- 5.9 POST /v1beta/models/{model}:generateContent
- 5.10 /v1/audio/transcriptions
- 5.11 /v1/audio/speech
- 5.12 Seedance 视频：创建、Callback、Polling、交付与结算
- 任务辅助接口
- Console 与 Playground 没有另造一套网关
- 上线前必须逐项检查的设计边界
- 写接口的安全中间件顺序必须单独验证
- 16 个接口之外的能力需要单独设计
- 一个模型真正发布需要走完哪些步骤
- 判断“模型已支持”的证据顺序
- 写在最后
- **第10篇：案例分析：OpenRouter 凭什么值 13 亿美元**
- 第一阶段：模型市场先验证统一交易
- 时间：2023 年至 2024 年
- 产品判断：模型数量只是供给规模，统一账户、统一接口和统一结算才是聚合平台的最小商业闭环
- 第二阶段：BYOK 把平台从 Token 销售商变成基础设施
- 时间：2024 年 12 月
- 产品判断：当客户可以绕过平台余额，平台仍然有使用价值，说明它出售的已经不只是 Token，而是统一接入、路由和治理能力
- 第三阶段：路由从内部逻辑变成对外产品
- 时间：2025 年 1 月至 3 月
- 产品判断：平台价值从"替客户接入多个模型"，升级为"替客户选择更合适的模型和供应商，并屏蔽失败差异"
- 第四阶段：用量与配置开始服务生产流量
- 时间：2025 年 4 月至 6 月
- 产品判断：完成"能调用"之后，下一步不是继续堆模型，而是让调用可查询、成本可解释、配置可变更
- 第五阶段：同一个模型的不同端点不再被视为等价
- 时间：2025 年 7 月至 2026 年 3 月
- 产品判断：路由不再只是权重和优先级配置，而是由真实请求质量数据驱动的持续决策系统
- 第六阶段：企业客户需要组织级治理
- 时间：2026 年 4 月至 5 月
- 产品判断：当平台服务对象从个人开发者变成企业，账户模型必须升级为组织、项目、成员、Key 和策略之间的关系模型
- 第七阶段：多模态复用同一套治理底座
- 时间：2026 年 4 月至 6 月
- 产品判断：多模态平台真正需要复用的不是 URL 风格，而是认证、能力发现、路由、计量、结算和治理底座
- 第八阶段：从调用模型走向编排智能
- 时间：2026 年 6 月至 7 月
- 产品判断：模型聚合平台开始介入模型选择、任务拆分、结果综合和 Agent 工具发现，平台边界从 Gateway 延伸到 Runtime
- 八阶段演进构成 13 亿美元的产品底座
- OpenRouter 占据的是推理控制点
- 最低迁移成本完成开发者获客
- 双边市场让供给和需求相互增强
- BYOK 降低企业迁移阻力
- 透明定价建立中立信任
- 真实请求数据形成第二层飞轮
- 运营策略把产品数据变成分发渠道
- 热点模型承接搜索需求
- 免费模型承担开发者获客成本
- Stealth Model 制造独家供给
- 应用排行榜让客户帮助平台传播
- 行业报告把平台变成数据来源
- 产品驱动增长完成企业转化
- 13 亿美元买的不是每一项功能
- 头部平台仍然存在边界
- 自建 AI 聚合平台的迭代清单
- 1. 先闭合交易
- 2. 再收口执行
- 3. 补齐证据
- 4. 开放客户资产
- 5. 让配置脱离代码
- 6. 用数据驱动路由
- 7. 再进入企业治理
- 8. 最后扩展运行时
- 9. 建立增长入口
- 10. 公开市场数据
- 13 亿美元最终买的是什么
- **第11篇：附录：Claude Code / Codex 第三方平台 API Key 配置手册**
- 先准备 4 个值
- Mac 配置 Claude Code
- 方式一：临时配置
- 方式二：长期配置到 zsh
- 方式三：写入 Claude Code settings
- 验证 Claude Code
- Windows 配置 Claude Code
- 方式一：临时配置
- 方式二：长期配置到用户环境变量
- 方式三：写入 Claude Code settings
- 验证 Claude Code
- Mac 配置 Codex
- 1. 设置 API Key 环境变量
- 2. 编辑 Codex 配置
- 3. 启动 Codex
- 4. 验证 Codex
- Windows 配置 Codex
- 1. 设置 API Key 环境变量
- 2. 编辑 Codex 配置
- 3. 启动 Codex
- 4. 验证 Codex
- WSL 单独说明
- 常见错误
- 最小检查清单
- 参考资料

---


## 第1篇：开篇：从0到1打造一个AI Token聚合平台

很多团队做 AI Token 聚合平台，第一反应是：
接一个模型代理，暴露 /v1/chat/completions ，把请求转给 OpenAI、Anthropic、
Gemini 或其他兼容服务。
这样做可以很快调通接口，但这还不是一个能收费上线的平台。
真正决定这个系统能不能对外卖，不是“请求能不能转发”，而是下面四件事：
维度 要回答的问题
控制面 谁在调用？属于哪个租户？能用哪些模型？能花多少钱？
执行面 这次请求走哪个上游？失败后能不能 fallback？路由是否可回滚？
资金面 请求前有没有锁住预算？请求后有没有准确落账？账单能不能解释？
证据面 出问题后能不能按 request_id 查清楚？上线前有没有 smoke、canary 和对账证据？
这就是这个专题想讲的主线：
AI Token 聚合平台不是一个反向代理，而是一个围绕模型调用构建的计费、风控、对账和运
营系统。

## 为什么“能调通”离“能收费”很远

一个最小代理只需要做三件事：
```
Client -> Gateway -> Model Provider
收请求，换模型名，转发给上游，再把结果返回给客户。
如果只是内部工具，这可能够用。但一旦它变成收费产品，问题马上变了。
客户会问：
我的 API Key 为什么被扣了这笔钱？
同一个请求重试了两次，为什么产生了两笔费用？
流式请求中途断了，算成功还是失败？
模型返回了 cached tokens、reasoning tokens、image tokens，分别怎么计费？

我是 OEM 客户，为什么流量走了共享上游，而不是我的专属池？
平台自己也要问：
用户钱包余额不足时，能不能保证请求不会先打到上游？
上游账单和平台 usage log 对不上时，以哪个为准？
LiteLLM 的 spend log 能不能直接当客户账单？
新 provider、新模型、新价格上线前，有没有可复现的证据包？
这些问题，单靠反向代理解决不了。
代理解决的是协议转发。收费平台要解决的是责任边界。

## 企业级 Token 聚合平台的四层架构

我更倾向于把 Token 聚合平台拆成四层：
```
┌──────────────────────────────────────────────┐
│ 控制面 Control Plane │
│ API Key / HMAC / 租户隔离 / 模型目录 / 限额 │
└───────────────────┬──────────────────────────┘
│
┌───────────────────▼──────────────────────────┐
│ 执行面 Execution Plane │
│ OpenAI 兼容直连 / Provider Adapter / 路由 / Fallback │
└───────────────────┬──────────────────────────┘
│
┌───────────────────▼──────────────────────────┐
│ 资金面 Money Plane │
│ Usage Log / Charge Line / 钱包 / 配额 / 价格快照│
└───────────────────┬──────────────────────────┘
│
┌───────────────────▼──────────────────────────┐
│ 证据面 Evidence Plane │
│ Raw Usage / Provider Cost / Invoice / Release Evidence │
└──────────────────────────────────────────────┘
这四层缺一层，系统都很难进入公开收费状态。
控制面回答“谁可以调用什么”。
执行面回答“请求实际怎么被完成”。
资金面回答“钱怎么算，钱怎么扣，账怎么解释”。
证据面回答“出了问题怎么复盘，新能力怎么证明可以上线”。

## 第一层：控制面，不只是 API Key

很多人把 API Key 当成一串 token。
在企业级平台里，API Key 是一个预算边界、租户边界和审计边界。
它至少要绑定这些信息：
对象 作用
user_id 谁拥有这把 key
tenant_id 这把 key 属于哪个租户或 OEM
allowed_models 能调用哪些模型
hard limit 总额、周期额度、风险上限
IP whitelist / HMAC 企业客户的调用来源和签名约束
status / expiry 是否启用、是否过期
如果只在请求开始前读一次额度，然后请求结束后无条件累加消费，就会出现并发穿透。
如果 OEM 独立域名和 API Key 的租户不匹配，但网关没有 fail-closed，就会出现入口污
染：某个租户的 key 可以通过另一个租户的品牌域名调用。
这些问题看起来是鉴权细节，本质上是商业边界。
收费 API 的第一条原则是：
调用身份、租户归属、预算额度和入口域名必须在请求开始前收口。

## 第二层：执行面，LiteLLM 是代理层，不是平台账本

LiteLLM 这类代理层很有价值。
它可以帮平台统一对接不同 provider，处理 OpenAI-compatible 协议适配、模型名映射、
上游请求、流式响应和部分 usage 归一化。
但它不应该承载平台的最终账务。
原因很直接：
LiteLLM 适合负责 平台必须自己负责
协议适配 租户隔离
上游模型转发 API Key 预算
provider route 客户钱包
spend evidence 客户账单

LiteLLM 适合负责 平台必须自己负责
上游错误返回 争议处理
LiteLLM spend log 可以作为请求级证据，但不能直接替代客户账本。
因为客户实际支付价格可能包含平台定价、OEM 折扣、套餐额度、缓存 token 价格、图片
价格、汇率快照和毛利策略。
上游花了多少钱，和客户应该被收多少钱，不是一回事。
所以执行面应该保持可替换。
平台可以用 LiteLLM，也可以接原生 provider adapter。但无论执行面怎么换，平台自己的
usage log、charge line、wallet、quota 和 reconcile 都不能丢。

## 第三层：资金面，请求前要锁预算，请求后要能解释

付费中转站最危险的亏损入口，是预算不足但请求已经打到上游。
上游成本一旦发生，平台就已经承担了成本。此时再发现用户余额不足、配额不足、API Key
超限，已经晚了。
所以请求生命周期里必须先做预算 claim：
```
claim idempotency
-> reserve wallet / quota / API key budget
-> call upstream
-> parse usage
-> create usage log
-> create charge lines
-> commit wallet / quota / spend
-> release or compensate
这里面有几个关键点。
第一，幂等要发生在上游调用前。
如果同一个 request_id 并发进来，两条请求都先打上游，最后只有一条成功落账，平台还是
可能承担两次 provider cost。
第二，账单不能只有一个总金额。
文本 token、cached token、reasoning token、图片、音频、视频、service tier、
provider cost，最好都拆成可解释的 charge line。否则客户只看到“扣了 0.37 元”，但不知
道为什么。
第三，金额不能用浮点数。

AI 计费里经常有千 token 单价、缓存单价、美元成本、人民币售价、OEM 折扣和 6 位小数
钱包流水。用 float 做钱，迟早会在对账时付出代价。

## 第四层：证据面，没有证据包，就不能上线

一个收费模型上线，不能只看“本地调用成功”。
至少要证明：
非流式请求能返回正确 usage。
流式请求能拿到 final usage，异常时不会伪装成正常结束。
cache / reasoning / image / audio 等计费维度被正确识别。
TreeCloud usage log 能查到。
客户 charge line 能查到。
LiteLLM raw usage 或 provider evidence 能查到。
route snapshot 能说明当时走的是哪个模型、哪个 provider、哪个价格版本。
这就是发布证据包的意义。
它不是为了让上线流程显得正式，而是为了在三天后、三周后、三个月后还能回答这个问
题：
当时为什么认为这个模型可以收费上线？
如果回答不了，就不应该公开放量。

## 这个专题会写些什么

接下来这个专题会按“从平台骨架到收费上线”的顺序拆。
第一组，讲基础认知和产品边界：
Token 聚合平台为什么不仅仅是反向代理
OpenAI-compatible 只是协议形态，不是商业闭环
直客、OEM、平台和上游 provider 的责任边界
第二组，讲网关控制面：
API Key、HMAC、IP 白名单和租户隔离
OpenAI-compatible 网关请求链路
模型目录、渠道、LiteLLM route 的职责拆分
第三组，讲执行面：
为什么 LiteLLM 适合做执行代理层
多 LiteLLM 实例、共享池、OEM 专属池

fallback、健康分和 provider matrix
第四组，讲计量和计费：
usage log 为什么是客户账本
多维定价：cache、reasoning、image、audio、video
wallet、quota、API Key budget claim
charge line、价格快照、汇率和金额精度
第五组，讲对账和风险：
四层对账：请求内一致性、LiteLLM raw usage、provider official cost、provider
invoice
幂等、流式失败和中途错误语义
为什么对账任务不能直接改客户钱包
第六组，讲上线和运营：
Billing Ops 该看什么
smoke、canary、release evidence 怎么做
从 No-Go 到受控灰度，怎样判断一个能力能不能公开收费
最后，讲 OEM 和未来演进：
OEM 分销不是换 logo，而是租户、入口、模型、价格和结算的一整套隔离
原生图片、视频、多模态 adapter 为什么不是多加几个 endpoint
高 QPS 下为什么要走 ledger-first 和异步聚合

## 一张检查清单

如果你正在做 AI Token 聚合平台，可以先用这张表判断系统处在哪个阶段。
     | 只做代 | 可内 可收费灰 | 可公开放 
问题
                                           | 理   | 测 度  | 量   
 OpenAI-compatible 接口能调通                   | 是   | 是 是  | 是   
 API Key 绑定租户、额度、模型权限                      | 否   | 是 是  | 是   
 请求前预算 claim                               | 否   | 部分 是 | 是   
 usage log 是客户账本                           | 否   | 是 是  | 是   
 charge line 可解释每个收费维度                     | 否   | 部分 是 | 是   
 LiteLLM / provider evidence 可按 request_id | 否   | 部分 是 | 是   
对账

     | 只做代 | 可内 可收费灰 | 可公开放 
问题
                                        | 理   | 测 度  | 量   
 provider official cost / invoice 能闭环   | 否   | 否 部分 | 是   
 新模型上线有 smoke / canary / route snapshot | 否   | 部分 是 | 是   
 OEM 租户入口、模型、账务隔离                       | 否   | 部分 是 | 是   
这张表里，第一行只是开始。
真正难的是后面那些看起来“不性感”的东西：预算、账本、对账、证据、灰度、回滚。
但这些东西，才决定一个 AI Token 聚合平台能不能从 demo 变成业务。
从 API Key 鉴权、租户隔离、模型路由，到 LiteLLM 转发、usage log 和计费落账。
这个系列会持续拆企业级 Token 聚合平台的网关、计费、对账、OEM 和上线门禁。如果你
正在做 AI 网关、模型聚合平台或 AI SaaS 计费系统，可以按这个系列建立自己的设计清
单。
```

---


## 第2篇：网关核心：一条能收费的请求链路

一旦网关要对外卖 Token、接 OEM 租户、限制 API Key 预算、支持流式输出、处理上游失
败和账单争议，每个请求都必须回答 7 个问题：
 问题         | 如果没回答，会发生什么                   
 谁在调用       | API Key 额度、租户隔离和审计都无法闭环       
 属于哪个租户     | OEM 入口、模型权限、账单归属会混乱           
 能不能花这笔钱    | 余额不足仍然打上游，平台承担成本              
 该走哪个模型和上游  | 路由不可解释，失败后无法复盘                
 usage 从哪里来 | 账单争议时说不清 token 口径             
 钱怎么落账      | 客户账单、钱包、配额和 API Key spend 对不上 
 出问题怎么复盘    | request_id 查不到完整证据链           
所以这篇文章想讲清楚一个判断：
一个能收费的 OpenAI-compatible 网关，不是“收 JSON、调上游、回 JSON”的
handler，而是一条必须同时完成身份、预算、路由、执行、计量、落账和证据留存的请求
链路。

## 一个 handler 为什么不够

如果只是内部工具，一个 handler 可能够用。
 它只要把  | /v1/chat/completions |  的请求转出去，再把上游响应原样转回来。模型名写死也 
可以，额度不校验也可以，日志少一点也可以。
但收费平台的风险不在“请求没有转发成功”，而在下面这些地方：
预算不足的请求已经打到了上游。
同一个 request_id 并发重试，产生了多次 provider cost。
流式请求中途断开，客户端看到一半输出，平台不知道该不该收费。
上游没有返回 usage，系统静默估算，却没有标记来源。

客户查账时只能看到一个总金额，看不到 cache、reasoning、输入输出 token 的价格口
径。
OEM 租户通过错误入口调用成功，账单和品牌域名归属不一致。
一个 handler 只能处理 HTTP。
一个收费 AI 网关要处理的是责任。

## 一条能收费的请求链路

在我们的实现里，网关入口不是挂在普通业务 API 下面，而是单独收口 OpenAI-
compatible 相关入口。
它既支持常见的 chat、responses、models 等 OpenAI-compatible 形态，也把
Anthropic、Gemini、多模态、音频等接口放进同一套请求生命周期里。
但无论入口长什么样，核心链路都可以抽象成 13 步：

更准确的拆法是 4 个阶段：
 阶段   | 核心问题       | 请求链路里的职责                 
 入口收口 | 谁在调用，能不能调用 | 鉴权、租户、限流、请求身份            
 路由决策 | 这次请求应该怎么执行 | 模型解析、租户池、共享池、渠道选择        
 执行转发 | 上游调用如何完成   | 请求转换、LiteLLM 转发、非流式和流式处理 
 资金证据 | 怎么收费，怎么解释  | usage、价格、钱包、配额、账单和证据     
下面分开讲。

## 入口收口：先建立账务身份

很多人把鉴权理解成“挡住非法请求”。
在收费 AI 网关里，鉴权更重要的作用是给合法请求建立账务身份。
我们的网关会在进入业务服务前，把调用者解析成一组稳定上下文：
上下文 用途
api_key_id API Key 额度、spend 统计和审计边界
user_id 钱包、配额、客户账单归属
tenant_id OEM 隔离、租户模型权限、入口域名校验
request_id 日志、幂等、账单、对账和客服查询
request_type chat、responses、image、audio 等不同计费表面
这里有几个实现上的细节很关键。
第一，request_id 要在链路一开始稳定下来。
如果客户端传了可用的请求标识，网关可以沿用；否则网关生成一个新的标识，并在响应头
里返回。这样客户、客服、财务和运维都能围绕同一个请求查证据。

第二，鉴权不能只校验 token 存在。
API Key 需要检查状态、硬额度、模型 allowlist、来源 IP 策略，以及和入口租户是否匹
配。企业客户使用 HMAC 时，还要校验时间窗口、nonce 和请求签名，避免重放和篡改。
第三，限流要放在鉴权之后。
因为收费网关不能只按 IP 限流。真正有意义的是按 API Key、用户、租户和 endpoint 限
流。没有稳定身份，RPM、TPM、并发限制都会变成粗粒度防护，无法成为商业规则。
这一阶段的结论是：
鉴权不是为了挡请求，而是为了让后面的预算、路由、账单和审计都能找到同一个责任主
体。

## 路由决策：模型名只是输入，不是结果

用户请求里看到的是 public model。
例如用户调用一个对外开放的模型名，他关心的是“我能不能用这个模型、价格是多少、效果
是不是稳定”。
但在平台内部，这个模型名只是路由输入。
真正执行时，网关至少要经过这几层：
 层级             | 读者看到的含义     | 工程含义                          
 模型目录           | 我能调用什么模型    | 平台能力、状态、定价和功能门禁               
 租户模型           | 当前租户是否开放该模型 | OEM 权限、价格覆盖和 pay-as-you-go 策略 
 路由策略           | 这次请求优先走哪里   | 租户专属池、共享池、fallback 边界         
 Channel        | 哪条执行路径被选中   | 权重、健康分、当前并发和成本                
 Instance       | 哪个执行实例承接请求  | LiteLLM 实例、凭证、route alias     
 Provider Route | 最终上游模型      | provider 协议、模型名和上游能力          
我们的路由是 fail-closed 的。
如果 public model 在平台模型目录里不是 active，或者当前租户不允许使用，网关不会继
续猜一个上游模型去调用。
如果模型可用，路由层会先尝试租户绑定的实例池。比如某个 OEM 有专属 LiteLLM pool，
就优先走它的绑定。没有合适候选时，才进入共享池。
共享池里也不是随机挑一个。

候选 channel 会经过状态、健康分、并发上限和权重过滤。一个 inactive、unhealthy、
pending 或并发已满的 channel，不能因为权重高就继续承接请求。
最终选中的 channel 还会被写入请求证据里，包括 channel、instance、route policy、
provider route 等信息。
这件事非常重要。
因为客户问“这次为什么贵了”“为什么这次失败了”“为什么没有走我的专属上游”时，平台不能
只回答“系统自动路由了”。
平台要能回答：
```
这个 public model 当时映射到了哪个内部模型
当前租户当时有没有权限
路由策略选中了哪个 channel
channel 当时的实例和 provider route 是什么
这条路径对应哪个价格和证据版本
这一阶段的结论是：
模型名不是路由结果，只是路由输入。收费网关必须把路由过程变成可解释证据。

## 执行转发：非流式和流式不是同一件事

到了执行阶段，很多团队会觉得事情终于简单了：
把请求交给 LiteLLM，或者交给某个 provider adapter，然后等响应。
但 OpenAI-compatible 网关里，非流式和流式是两套风险模型。
场景 非流式 流式
响应方式 上游完整响应回来后一次 chunk 持续写给客户端
性返回
usage 获取 通常在响应体中解析 通常在 final chunk 或结束阶段解析
fallback 边 上游未成功前可以重试或 首个 chunk 发出前可以处理，开始输出后不能随意切换
界 切换
客户端断开 请求一般已经有明确响应 客户端可能先断，上游仍在生成
结果
账务证据 记录完整响应 usage 记录 final usage、estimated、disconnect 和 upstre
am error
非流式请求比较直观。

网关拿到完整响应后，解析 usage，恢复对外模型名，计算价格，写入 usage log 和
charge line，再返回 OpenAI-compatible 响应。
流式请求复杂得多。
因为 chunk 一旦写给客户端，这次请求的语义就变了：你不能在输出一半后突然切换到另一
个 provider，把两个 provider 的结果拼成一个回答。
所以我们的流式策略是：
首个 chunk 发出前，如果执行失败，可以按策略处理。
首个 chunk 发出后，不再做 provider 级切换。
尽量要求上游在流末尾返回 usage。
如果上游缺失 usage，可以估算，但必须标记 usage 来源。
如果客户端中途断开，网关仍要尽量把上游流读完，用 final usage 完成账务证据。
最后一点很反直觉，但对收费平台很重要。
客户端断开不代表 provider cost 没发生。
如果网关因为客户端连接断开就直接取消上游请求，很可能拿不到 final usage。客户体验上
看是“我中断了请求”，平台证据上却变成“上游到底生成了多少不知道”。
我们的实现会把“向客户端转发 chunk”和“继续读取上游流用于计量”分开处理。客户端还在，
就继续转发；客户端断开，就停止转发，但尽量继续 drain 上游流，直到拿到 usage 或明确
失败。
这一阶段的结论是：
流式不是“边读边写”这么简单，它改变了 fallback、错误语义和计费证据。

## 资金证据：HTTP 返回不代表生命周期结束

对客户端来说，HTTP 200 或 SSE [DONE] 基本意味着请求结束。
对收费网关来说，这只是生命周期进入资金收口阶段。
一个 AI 请求真正完成，至少要把下面几件事对齐：
```
estimated tokens
-> estimated cost
-> wallet / quota / API key spend reserve
-> upstream execution
-> actual usage parse
-> price calculation
-> usage log
-> charge lines
-> wallet / quota / API key spend commit

-> billing intent state
-> metrics and evidence
这里要特别区分三类东西：
对象 它是什么 它不是什么
usage log 客户账单和请求明细的主记录 不是一个简单访问日志
charge line 费用拆分和价格证据 不是只有总金额的流水
provider evidence 上游成本和对账证据 不是客户最终应付账单
我们的 usage log 不只记录输入输出 token。
它还会记录请求类型、对外模型、上游模型、channel、instance、usage 来源、cache
token、reasoning token、service tier、quota 消耗、钱包费用、上游成本、请求摘要和
响应摘要。
这样做不是为了字段多。
而是为了让平台在账单争议时能回答：
这次请求是哪个 API Key 发起的？
属于哪个用户和租户？
用的是哪个对外模型？
实际走了哪个执行路径？
usage 是上游返回的，还是平台估算的？
用户价格和上游成本分别是多少？
这笔费用是否命中了套餐额度、折扣或钱包扣费？
charge line 的意义也类似。
只写一条“本次扣费 X 元”，后续很难解释缓存 token、reasoning token、图片、音频、视
频、批处理、service tier 等维度。
收费平台应该把费用拆成可解释行，并记录价格来源、价格版本、币种和金额精度。
这一阶段的结论是：
网关返回响应之前要完成执行，返回响应之后还要能解释账。

## 我们在实现里保留的 5 个不变量

把上面的机制落到代码里，最重要的不是某个函数怎么写，而是这些不变量不能被破坏。

### 1. request_id 不是全局唯一账务身份

很多系统会把 request_id 当成唯一键。
这在多租户收费平台里不够。
更稳妥的请求身份应该包含：
```
tenant scope + user_id + api_key_id + request_type + request_id
同一个 request_id 可能被不同客户、不同 API Key、不同请求类型使用。只靠 request_id
做幂等和查询，很容易串账。

### 2. 预算预留必须发生在上游调用前

如果先打上游，再扣钱包或扣配额，平台就会暴露在成本风险里。
正确顺序是先预估，再检查，再预留，再执行。
实际 usage 回来后，再按真实费用提交或补偿。

### 3. 路由结果必须进入证据链

只知道用户调用了哪个 public model 不够。
账单、客服和运营还需要知道最终走了哪个 channel、哪个 instance、哪个 provider
route、哪个价格版本。
否则一次异常只能靠日志猜。

### 4. usage 来源必须显式标记

usage 可能来自上游，也可能来自流末尾，也可能来自平台估算。
估算不是不能用，但不能伪装成上游精确返回。
只要进入客户账单，usage source 就必须可见。

### 5. 流式错误不能伪装成正常成功

流式请求里，客户端断开、上游中断、首 chunk 后失败，都是不同语义。
如果中途错误最后仍然写一个正常结束标记，客户端 SDK、客户账单和平台客服都会被误
导。

## 常见错误方案
### 错误一：把 OpenAI-compatible 当纯协议转换

只处理 request 和 response，不处理预算、账本和证据。

这种系统可以 demo，但不能放心收费上线。

### 错误二：request_id 只用于日志

request_id 如果不参与幂等、账单和对账，同一请求的重试就可能变成多次上游成本。
最后客户觉得只请求了一次，平台账务里却出现多条费用证据。

### 错误三：流式中途错误用正常结束收尾

这会让客户端以为生成成功，也会让账务侧以为可以按完整请求处理。
真正的问题是：输出可能已经截断，usage 也可能不完整。

### 错误四：usage 缺失就静默估算

估算是兜底能力，不是精确证据。
如果不标记 usage source，后续和 provider invoice 对账时一定会说不清。

### 错误五：只写 usage log，不写 charge line

usage log 记录“发生了什么”。
charge line 解释“为什么扣这些钱”。
只有一条总金额，后续遇到 cache、reasoning、多模态和 OEM 折扣时，账单解释成本会
非常高。

## 一条能收费的 AI 网关请求链路检查清单

如果你正在做 AI 网关、模型中转或 Token 聚合平台，可以用这张表做一次自查。
检查项 通过标准
调用身份 每个请求都有稳定的 api_key_id、user_id、tenant_id
租户边界 API Key 租户和入口租户能 fail-closed 校验
请求身份 request_id 可用于日志、幂等、账单和对账
模型解析 public model 必须映射到 active 的平台模型目录
权限控制 租户模型权限、API Key allowlist 和来源策略都能生效
预算控制 请求上游前已经完成钱包、配额和 API Key spend 预留
路由证据 能记录最终 channel、instance、provider route 和 route policy
负载选择 路由会考虑健康分、并发上限和权重，而不是纯随机
Fallback 语义 首 chunk 前和首 chunk 后的行为边界清楚

检查项 通过标准
Usage 来源 upstream、final chunk、estimated 均可区分
计费落账 usage log、charge line、quota、wallet、API Key spend 有一致关系
错误语义 上游错误、客户端断开、超时不会伪装成正常成功
查询闭环 客服或财务能按 request_id 查到完整链路

## 结尾

这篇文章讲的是一条请求从进入网关到完成出账的完整生命周期。
能调通 OpenAI-compatible 接口，只是第一步。
真正能收费的 AI 网关，必须在每个请求里同时完成身份、预算、路由、执行、计量、落账和
证据留存。
什么时候用 LiteLLM 这类执行代理，什么时候需要原生 Adapter，什么时候只做 OpenAI-
compatible Facade。
这个选择会决定平台后续接 OpenAI、Anthropic、Gemini、多模态模型和 OEM 专属上游
时，到底是轻装扩展，还是被 adapter 复杂度拖住。
从0到1打造一个AI Token聚合平台 AI聚合网关之三:NewAPI、LiteLLM 和自研原
生 Adapter 的边界
```

---


## 第3篇：架构抉择：NewAPI、LiteLLM 与自研原生 Adapter 的边界

不是所有模型接入都要自研，也不是所有 AI 网关都适合托给通用代理。关键是先分清网关工具、
执行代理、商业平台和原生能力四种责任。
很多团队接新 provider 时，第一反应通常是：
```
能不能找个现成网关接一下？
能不能都转成 OpenAI-compatible？
能不能先用一个通用代理顶住？
这些问题都对，但顺序错了。
真正应该先问的是：这一层系统到底承担什么责任？
你真正要解决的问题 更像哪种选择
自用转发、快速聚合、多渠道面板 NewAPI
平台已有账本，只缺 provider 执行代理 LiteLLM
要保留 provider 原生协议、usage 和任务状态 自研原生 Adapter
要做客户、钱包、OEM、对账、审计 自研平台控制面，不能交给代理层
Provider Adapter 选错，通常不是因为你低估了上游接口，而是因为你高估了 OpenAI-
compatible 能抹平的东西。

## 先分清四层责任

在企业级 Token 聚合平台里，模型接入不是一个简单的 HTTP 转发问题。至少有四层责任：
```
商业平台控制面
用户 / 租户 / OEM / API Key / 钱包 / 账本 / 对账 / 审计

执行代理层
LiteLLM / NewAPI / 其他通用网关

Provider Adapter
OpenAI / Anthropic / Gemini / Doubao / GLM / OpenRouter


Provider Native API
usage / cache / reasoning / image / video task / callback / invoice evidence
NewAPI 更接近通用 AI 网关和管理面板。它适合快速统一模型入口、管理渠道、处理
token、额度、日志和充值等通用网关能力。
LiteLLM 更适合放在多 provider 执行代理层。它的强项是 provider coverage、OpenAI-
compatible 转换、route alias、streaming、虚拟 key、spend logs 和 provider
fallback。
自研原生 Adapter 则适合处理 provider 原生协议、异步任务、usage 证据、媒体交付和安
全边界。
分账、争议时用哪一份账本说话，这些都属于平台责任。
一句话：工具可以替你转发请求，但不能替你承担客户账本和商业责任。

## 什么时候可以选 NewAPI

NewAPI 的优势不是“它能替代所有平台”，而是它能把“先接起来”这件事做得很快。
如果你的目标是自用网关、团队内部代理、轻量中转站 MVP，或者快速搭一个多模型 API 聚
合面板，NewAPI 是很现实的选择。它已经围绕渠道、模型、token、额度、日志、充值和
多协议兼容做了大量通用能力。
适合 NewAPI 的场景 为什么适合
个人或小团队自建 AI 网关 快速部署，已有通用管理面板
内部多模型统一出口 不需要复杂 OEM 和财务闭环
轻量中转站 MVP 先验证需求，不急着做自有账本
协议覆盖要求广，但商业责任较轻 通用网关比从零写适配更快
但如果你的目标是做一个真正对外收费的 AI Token 平台，问题就变了。
不建议直接用 NewAPI 承担的场景 主要风险
闭源商业化二开 需要提前评估开源协议和持续维护成本
OEM 分销和多级结算 通用网关不是围绕分销账本设计
客户账本、发票、退款、毛利保护要求高 quota 倍率不足以解释复杂商业账
每次请求都要 charge line、FX、provider evidence 需要自有账本和对账系统
AI Token 要和代理 IP、订单、钱包统一售卖 需要产品、订单、钱包和运营控制面

我们在做 AetherSpace 和 NewAPI 的定位对比时，结论不是“NewAPI 不行”。更准确的结
论是：
NewAPI 更像快速搭建通用 AI 网关；AetherSpace 要做的是经营 AI Token 生意的平台。
前者解决“能不能接”。后者还要解决“能不能卖、能不能扣、能不能分账、能不能对账、能不
能审计”。
所以，NewAPI 适合把“能接入”做快。如果你要经营客户、账本和分销，它更适合作参考或
阶段性工具，而不是平台核心控制面。

## 什么时候选 LiteLLM

LiteLLM 适合的场景，是平台已经有自己的用户系统、API Key、租户、钱包和账本，只缺一
个稳定的 provider 执行代理。
execution layer。
这句话听起来抽象，落到实现上就是几件非常具体的事。

### 第一，AetherSpace 先做平台侧控制

请求进来后，先完成 API Key 鉴权、租户校验、模型解析、预算预检、钱包或配额预留、请
求身份锁定。这些动作都发生在平台 Gateway 里，不能依赖执行代理补救。

### 第二，AetherSpace 只选择一个平台 channel

当前实现里，路由不是在 Gateway 内部对多个 provider 逐个 retry。平台会根据租户绑定
池、共享池、route policy、健康状态、并发负载和权重选择一个 channel。选中后，本次
请求就沿着这个 channel 执行。
Provider 级别的 retry、fallback 和 timeout 策略，放在 LiteLLM 的 route 边界下处理。
这样可以避免平台 Gateway 和执行代理两边都在重试，最后账本、幂等和证据全部变乱。

### 第三，AetherSpace 把多池路由留在自己手里

代码实现里，路由大体是两级：
```
先看租户绑定实例池
找到可用 channel -> 按健康、并发、权重、route policy 选择
找不到 -> 回落到共享池
共享池
同样按健康、并发、权重、route policy 选择

这让平台可以表达 OEM 专属池、共享池、A/B 上游、租户固定路由、灰度策略等商业路由
规则。LiteLLM 不需要理解租户和 OEM，它只负责被选中之后怎么把请求打到 provider。

### 第四，LiteLLM spend 只是证据，不是客户最终账本

LiteLLM 可以提供 spend log 和 provider 执行信息，但客户最终应付金额必须回到平台
usage log、charge line、钱包流水和对账流程。原因很简单：客户价格、折扣、余额、套
餐、OEM 结算和上游成本不是同一张账。
适合 LiteLLM 的场景 为什么适合
平台已有 API Key、租户、钱包和账本 LiteLLM 只承担执行，不抢平台控制面
主要入口是 OpenAI-compatible 可以降低 provider 适配成本
需要多 provider route alias 对外模型名和上游模型名可以解耦
需要上游 A/B、实例冗余、fallback 适合放在执行故障域里
需要 raw spend evidence 可作为平台对账证据之一
我们在 GPT-5.4 双实例、双上游配置里采用的思路也是这个：平台侧把 LiteLLM 实例和上
游 A/B 拆开建模，AetherSpace 负责 route policy 和证据，LiteLLM 负责 provider
route 和执行。
所以 LiteLLM 最适合做“平台控制面之下的执行代理”，而不是替代平台账本。

## 什么时候必须自研原生 Adapter

OpenAI-compatible 可以统一很多 chat 形态，但不能统一所有 provider 的原生能力。
一旦 provider 原生字段会影响计费、风控、异步任务、素材交付或客户 SDK 体验，就应该
自研 Adapter。
必须自研的信号 例子 为什么不能只靠通用代理
原生 usage 口径影 Gemini usageMetadata 、cache、though 账单需要保留 provider 原生
响计费 ts、image token 证据
请求不是同步响应 视频生成任务、查询、取消、callback 需要任务状态机和最终结算
响应不是 OpenAI Gemini candidates.parts.inlineData 客户 SDK 需要原生响应
结构
provider feature grounding、imageSize、region、data r 未定价、未建模能力要 fail-cl
必须门禁 esidency osed
需要素材转存和安 视频、图片、文件下载 涉及存储、防 SSRF、过期授
全处理 权

 必须自研的信号  | 例子                     |     | 为什么不能只靠通用代理      
 上游凭证和回调必 | callback、credential_id |     | 不允许客户覆盖上游 key 或内 
 须隔离      |                        |     | 部 callback       

## 例子一：Gemini Native 图片

一开始，Gemini 可以通过 LiteLLM 的 OpenAI-compatible Chat facade 接入。对于普通
文本和简单对话，这样做成本低，也方便统一入口。
但到了图片生成，情况变了。
Gemini  的   generateContent 、 usageMetadata 、 imageSize 、 aspectRatio 、
和 Gemini 原生错误形态，都会影响客户体验、计费口径和上线门禁。
inlineData
所以 AetherSpace 没有只把它当成普通  /v1/images/generations  转发，而是建立了
Gemini 原生 Gateway contract：
 实现点               |                 |     | 设计意图                     
 使用 Gemini 原生      | generateContent |  入口 | 保留 provider native shape 
 保留  usageMetadata |  和原始响应          |     | 用作 usage evidence 和对账材   
料
 按实际有效图片数计  | image_output_units |     | 计费维度和文本 token 分开 
对  imageSize 、grounding、cache、Live API 等能力做 featu 未定价或未建模能力 fail-closed
re gate
 必要时再映射成 OpenAI Images facade |     |     | 兼顾客户通用 SDK，但不丢原生 
证据
代码实现上，Gemini 请求会先经过模型解析、feature gate、图片尺寸校验、预算预检和路
由选择。执行时可以仍然通过 LiteLLM 传输，但请求路径、响应解析、usage evidence 和
最终计费口径都由 AetherSpace Gateway 掌握。
这就是“LiteLLM 可以参与执行，但不能替代原生语义”的典型场景。

## 例子二：Doubao Seedance 视频

视频生成比图片更能说明为什么需要自研 Adapter。
它不是一次 HTTP 返回结果，而是一个异步任务生命周期：
```

创建任务
-> 等待 / 运行
-> callback 或轮询
-> 成功 / 失败 / 取消 / 过期
-> 素材交付
-> 最终结算
-> provider evidence 留存
这个过程里，通用代理能帮你打请求，但不能替你决定任务状态机、callback 安全、素材交
付和最终扣费。
实现点 设计意图
从平台加密配置解析上游凭证 客户请求不能携带或覆盖 provid
er key
拒绝 channel metadata 中的明文上游密钥 防止配置泄漏和错误发布
校验上游 base URL 只能是允许的安全来源 防止被请求参数改成任意转发
创建请求前清理 api_key 、 headers 、 callback_url 、 base_ 客户不能覆盖平台安全边界
url 等字段
由平台覆盖 provider model 和 callback 路由和回调归平台控制
映射 provider task 状态 把上游状态转成平台可结算状态
保留 provider 原始响应和 request evidence 用于异步对账和争议处理
创建任务前做预算预留 防止任务创建成功后钱包或配额
兜不住
这已经不是“写一个 HTTP client”了。
真正的 Adapter 要处理凭证隔离、错误映射、usage evidence、feature gate、幂等、状
态机、计费和发布证据。
当 provider 的原生语义决定账单、状态和证据时，自研 Adapter 不是重复造轮子，而是在
保护平台责任边界。

## 三种选择不是互斥关系

NewAPI、LiteLLM、自研 Adapter 不一定三选一。它们可以出现在不同层。
层 责任 可用工具
商业平台控制面 用户、租户、钱包、OEM、账本、审计 自研平台

 层     | 责任                                             |     |     | 可用工具    
 ----- | ---------------------------------------------- | --- | --- | ------- 
 执行代理层 | 多 provider 执行、route alias、fallback、spend evide |     |     | LiteLLM 
nce
 通用网关参考或轻量阶 | 快速聚合、面板、额度、日志 |     |     | NewAPI 
段
 原生能力接入 | Gemini 图片、Doubao 视频、provider native usage |     |     | 自研 Adapte 
r
平台控制面自研，LiteLLM 做主要执行代理；对于 Gemini Native 图片、Doubao
Seedance 视频这类 provider 原生语义很强的场景，再自研 Adapter。

## NewAPI、LiteLLM、自研 Adapter 对照表

这张表可以作为选型时的快速判断：
 维度    | NewAPI       | LiteLLM                 | 自研原生 Adapter           
 ----- | ------------ | ----------------------- | ---------------------- | --- 
 核心定位  | 通用 AI 网关与管理  | 多 provider 执行代理         | 特定 provider 原生能力接      
       | 面板           |                         | 入                      
 最适合阶段 | 快速搭建、内部工     | 平台已有控制面后接多上游            | 商业化支持复杂 provider       
       | 具、MVP        |                         | 能力                     
 协议重点  | 广覆盖、统一入口     | OpenAI-compatible 与 pro | 保留 provider native sha 
       |              | vider route             | pe                     
 账本责任  | 通用 quota 和倍率 | spend evidence，不做最终     | 提供原生 evidence，账本       
       |              | 账本                      | 仍归平台                   
路由责任 通用渠道管理 provider route、fallback、 provider-specific  endpoi
           |           | 虚拟 key        | nt 和 task         
 TreeCloud | 对标、参考、轻量场 | 当前执行面主力       | 多模态和异步 provider 的 
 关系        | 景         |               | 必要补充              
 最大风险      | 商业平台责任下沉  | 把 spend 当客户账本 | 研发和维护成本更高         

## 一个更实用的决策树

接入新 provider 或新模型时，可以按这个顺序问：
```
接一个新 provider / 新模型

-- 只是内部统一转发或轻量中转？
      -> 可以先选 NewAPI


-- 已经有自己的用户、钱包、API Key、账本？
 -> 执行层优先选 LiteLLM

-- provider 能力能被 OpenAI-compatible 准确表达？
 -> LiteLLM / OpenAI-compatible facade

-- provider 原生 usage / cache / media / task 会影响计费？
 -> 自研原生 Adapter

-- 请求需要异步任务、callback、素材转存？
 -> 必须自研 Adapter + 状态机

-- 客户要用 provider 官方 SDK 形态？
-> 提供 native facade，或明确 Beta / Unsupported
这个决策树的核心不是“哪个工具更强”，而是“哪一层负责哪件事”。

## 常见的五个错误方案
### 错误一：把 NewAPI、LiteLLM、自研 Adapter 当互斥选择

实际上它们可以出现在不同层。NewAPI 可以做参考或轻量阶段工具，LiteLLM 可以做执行
代理，自研 Adapter 可以处理 Gemini、Doubao 这类原生语义很强的 provider。

### 错误二：把 OpenAI-compatible 当所有 provider 的最终协议

Chat 可以统一很多东西，但 cache TTL、Gemini usageMetadata 、视频任务、callback
和 provider invoice 不能被统一掉。

### 错误三：把代理层 spend 当客户账本

LiteLLM spend 和 provider evidence 是对账证据，不是客户最终应付账单。客户账单必
须由平台 usage log、charge line、钱包流水和结算规则生成。

### 错误四：新模型一上线就承诺 full native compatibility

更稳妥的做法是按 GA / Beta / Unsupported 管理。OpenAI-compatible Chat 可以先
scoped launch，但不能顺手宣称完整兼容 OpenAI、Claude、Gemini、GLM 各家原生
API。

### 错误五：自研 Adapter 只写 HTTP client

HTTP client 只是最外层。Adapter 真正的成本在凭证隔离、错误映射、usage evidence、
feature gate、幂等、状态机、计费和发布证据。

## 接入新 provider 前的 12 项检查清单

最后给一张可以直接复用的检查清单。

检查项 判断标准
入口协议 是 OpenAI-compatible，还是 provider native
客户 SDK 客户是否要求使用官方 SDK 形态
Usage 口径 usage 是否影响客户账单和 provider 对账
计费维度 是否包含 cache、reasoning、image、audio、video、task
请求生命周期 同步响应还是异步任务
幂等边界 request_id / task_id / callback 是否能闭环
凭证来源 上游凭证是否只能来自平台加密配置
客户可控字段 客户是否能覆盖 base_url、api_key、headers、callback_url
路由策略 是否需要租户池、共享池、A/B 上游、固定用户路由
证据留存 是否能保存 route snapshot、raw usage、provider request id
上线状态 GA / Beta / Unsupported 是否清晰
发布证据 是否有 smoke、canary、provider matrix、价格和账单验证

## 结尾

这篇文章讲的是 Provider Adapter 的选择边界。
如果只是快速统一转发，NewAPI 很合适，能快速上线运营。但是要接受风险,比如已知的
bug及漏洞,只能用它内置的定价方式,用量统计无法上下游对齐等,而企业级别所需关注的往往
是这些.
如果平台已经有自己的账本和控制面，LiteLLM 更适合做执行代理。
如果 provider 原生语义决定了 usage、任务状态、素材交付和对账证据，就必须自研
Adapter。
而要同时处理用户等级、租户A/IO聚E合M网、关折之扣二:体一系条能、收定费价的体请系求，链以路长及什从么预样算预留到最终扣费的完
整流程。
这个系列会持续拆企业级 ToAkednap 聚ter合 的平边台界的网关、计费、对账、路由执行面、OEM 和上线
门禁。如果你正在做 AI 网关或模型聚合平台，可以按这个系列建立自己的设计清单。
链路
台OEM体系 喜欢作者

Token 聚合平台 · 目录 AI聚合平台之六：Token转发后端架构与LLM渠道
高可用设计
AI聚合网关之二:一条能收费的请求链路长什么 AI聚合网关之四：从用户等级、OEM 到完整
样 扣费链路
```

---


## 第4篇：计费引擎：从用户等级、OEM 到完整扣费链路

基本最早大家都在用的都是这个公式：
cost = input_tokens * input_price + output_tokens * output_price
这个公式不是错，而是不够。
如果平台只是团队内部自用，维护一张模型单价表，再把上游返回的 usage 乘一下价格，短
期能跑。
但只要平台开始对外收费，问题会立刻变多：
 简化做法 | 企业级问题 
只维护 input/output 单价 无法表达 cache、reasoning、image、audio、video 等计费维度
 请求结束后才扣费         | 并发请求会透支钱包，平台先承担上游成本   
 折扣直接改模型价格        | 用户等级、OEM 策略和账单解释会混在一起 
 上游 usage 直接当客户账单 | 客户价、上游成本、折扣和结算规则不是一层  
 只写一个 cost 字段     | 财务、客服、对账都无法拆明细        
计费引擎的核心不是算出一个金额，而是让每一次请求都能在请求前控风险、请求后能落
账、出问题能解释。
本文要讲的就是这条完整链路：从模型定价、租户/OEM 选品、用户等级折扣，到配额优
先、钱包实扣、usage log 和 charge line。

## 01 先拆五层：定价、折扣、配额、钱包、账本

AI Token 聚合平台的计费，不应该从“单价表”开始看。
更合理的拆法，是先把一次请求背后的五层关系分开：

这五层分别回答不同问题：
层级 回答的问题
定价 这次请求按客户侧价格，折前应该多少钱
折扣 这个用户或 OEM 下游客户，钱包部分能优惠多少
配额 哪些用量由套餐、租户额度或用户额度覆盖
钱包 哪些金额需要现金余额实扣，扣款是否幂等
账本 事后客服、财务、客户和平台凭什么对账
这里最容易出错的是把五层揉成一个字段。
比如把用户等级折扣直接写进模型价格，看起来少了一次计算，但账单里就很难解释：
模型原价是多少？
套餐抵扣了多少？
用户等级优惠了多少？
钱包到底扣了多少？
平台上游成本和毛利是多少？
单价表只能告诉你理论上多少钱；计费引擎要保证这笔钱能被预检、预留、实扣、落账和解
释。

## 02 定价体系：客户价和上游成本是两条线

企业级计费的第一个原则是：客户价不要和上游成本混在一起。
客户价用于客户账单。

上游成本用于毛利、渠道成本核算和对账。
这两条线可能同时来自同一个模型，但它们不是同一个概念。
 价格层       | 作用                | 不能承担什么      
 平台基础价     | 平台直客默认刊例价         | 不表达用户等级折扣   
 租户/OEM 价格 | OEM 选品后的终端客户价     | 不等于上游成本     
 条件计价规则    | 按请求类型、参数、计量单位命中价格 | 不表达钱包余额     
 上游渠道成本    | 估算 provider 成本和毛利 | 不作为客户最终账单   
 价格快照      | 解释这次请求为什么这么算      | 不用于事后随意重算旧账 
在我们的实现里，请求进入网关后会先解析可计费模型。
平台直客走平台模型目录；OEM 或租户客户必须先命中租户侧启用的模型。租户可以对
input、output、cache、图片、音频、TTS 等维度设置自己的客户价，也可以决定是否允许
PAYG。
这一步很关键。
如果某个租户没有启用模型，或者这个模型没有配置可用的 PAYG 价格，请求应该被拒绝，
而不是走一个默认 0 元价格。
计费里最危险的默认值不是“贵一点”，而是“看起来能跑，但实际免费打上游”。
我们的计费引擎会分别计算两条结果：
customer list cost = 客户侧折前价格
upstream cost       = 上游侧成本估算
客户侧价格用于账单和扣费。
上游侧成本用于毛利保护和后续对账。
金额计算还要坚持一个底线：不要用浮点数。
AI 计费经常是很小的单价乘以很多维度，input、output、cache、image、audio 再加
总。如果用浮点数，单次误差看起来很小，但到了 charge line 汇总、财务对账和客户争议
时，就会变成解释成本。
如果上游成本涉及不同币种，还要保存汇率来源和版本。这样旧账不会因为未来汇率变化被
重新解释。
条件计价规则也要独立出来。

很多模型不是只有 input/output 两个价格。例如：
cached input read
cache write
long context
reasoning output
image output units
audio input seconds
TTS characters
video seconds 或 video units
这些维度如果硬塞到模型基础价里，最后会变成一堆不可维护的特殊字段。
更好的做法是：基础模型价负责默认价格，条件计价规则负责更细的 request type、计量单
位和参数条件。

## 03 折扣体系：用户等级不是直接改模型单价

第二个原则是：折扣属于用户权益，不属于模型基础价格。
平台直客读平台用户等级。
OEM 下游客户读 OEM 客户等级。
二者不能混用。
否则很容易出现跨租户权益读取，或者某个 OEM 的客户拿到了平台直客的折扣。
AI PAYG 折扣一般至少有四类：
折扣类型 作用范围 优先级 是否叠加
模型月累计阶梯折扣 指定模型 + 请求类型 + metric 1 不叠加
全局月累计阶梯折扣 所有模型 + 请求类型 + metric 2 不叠加
模型固定折扣 当前等级下某个模型 3 不叠加
等级默认 AI 折扣 当前用户等级所有 AI PAYG 4 不叠加
无折扣 默认兜底 5 不叠加
这里有两个容易踩坑的点。
第一个坑：多个折扣不要层层叠加。
如果模型固定折扣、等级默认折扣、月累计阶梯折扣一起叠，客户看到的价格会越来越难解
释，平台毛利也不可控。

更清晰的规则是：命中优先级最高的一条，生成一份折扣快照。
第二个坑：套餐额度不要二次打折。
如果用户已经用套餐额度覆盖了一部分用量，这部分本质上是预购资源消耗，不应该再吃
PAYG 钱包折扣。
所以折扣应只作用在钱包部分：
list_cost = 折前客户价
wallet_cost_before_discount = 配额覆盖后需要钱包支付的部分
discount_amount = wallet_cost_before_discount * (1 - discount_rate)
final_cost = list_cost - discount_amount
wallet_cost = wallet_cost_before_discount - discount_amount
举个简化例子。
一次请求折前客户价是 100 元，其中套餐额度覆盖了 60 元，只剩 40 元需要钱包支付。
如果当前等级折扣率是 0.75，那么：
discount_amount = 40 * (1 - 0.75) = 10
final_cost = 100 - 10 = 90
wallet_cost = 40 - 10 = 30
账单里可以清楚解释：
原价 100
套餐抵扣 60
钱包部分等级优惠 10
钱包实扣 30
本次最终客户成本 90
这比“直接把模型单价改成 75 折”更复杂，但它能解释清楚，也能避免套餐和钱包权益互相污
染。

## 04 租户/OEM：不是加一个 tenant_id 就完事

OEM 不是简单给用户加一个租户字段。
它至少会改变四件事：
对象 看到什么价格 平台需要保存什么
平台直客 平台模型价 + 平台用户等级折扣 平台等级、钱包、请求级账本

 对象        | 看到什么价格                 | 平台需要保存什么           
 OEM  下    | 游 客 租户模型价 + OEM 客户等级折扣 | 租户上下文、OEM 等级、租户价格快 
 户         |                        | 照                  
 OEM Owner | 下游消耗、配额池、资金汇总、结算视      | 租户额度、充值记录、结算策略     
图
 平台 Admin | 客户账、上游成本、毛利和对账证据 | 明细账本、汇率、渠道成本、上游证据 
一个模型在平台上可用，不代表它在某个 OEM 下也可用。
OEM 需要自己的模型选品。
它可以启用某些模型，关闭某些模型；可以允许 PAYG，也可以只允许套餐额度；可以设置自
己的终端客户价，也可以做客户等级体系。
这也是为什么 LiteLLM 这类路由层不应该负责理解 OEM 价格。
LiteLLM 更适合处理 provider 接入、路由、重试和协议适配。
租户可计费模型、OEM 等级折扣、配额策略和钱包扣费，应该在平台网关和计费引擎里完
成。
否则上游路由系统会被业务价格体系污染，而业务系统也拿不到完整账本。

## 05 完整扣费流程：一次请求从预估到落账

下面这张图，是一次 AI 请求在计费链路里的主流程。
这条链路里有三个关键点。

### 预估费用不是最终费用

请求前必须预估费用。
因为平台不能等请求打完上游后才发现用户没钱。
但预估费用不能直接当最终费用。
输入 token、最大输出 token、图片数量、音频秒数、视频单位，都可能和最终 usage 有差
异。
所以请求前做的是预算预检和预算预留；请求后才基于最终 usage 重算客户价和上游成本。

### 预留是为了并发保护

余额检查本身不够。
如果一个用户同时发起 20 个请求，每个请求在开始时都看到钱包余额足够，但都不做预留，
那么最终很可能透支。
所以请求前要做钱包预留和 API Key spend 预占。
这一步不是扣款，而是锁住一部分预算，防止并发请求把同一份余额重复使用。
请求结束后，再用最终 usage 做实扣，多退少补或释放预留。

### 流式请求也要完成计费闭环

流式请求最容易被忽略。
用户断开连接，并不代表上游请求已经停止，也不代表 usage 已经可用。
如果平台在客户端断开时直接结束计费，就可能出现“上游已经消耗，平台没有落账”的缺口。
更稳的做法是尽量继续读取上游尾部事件，拿到 usage 或明确标记估算 usage，然后再完成
扣费和落账。
这个细节决定了流式接口能不能收费上线。

## 06 配额、钱包和折扣：顺序不能乱

一次请求的扣费顺序，不能凭感觉写。
推荐顺序是：
先判断可用计费模型
再计算折前客户价
再用 quota 覆盖可覆盖部分
再对 wallet 部分应用 PAYG 折扣
最后做 wallet 实扣和账本落地

为什么不是先打折再扣 quota？
因为 quota 是预购资源，wallet 是按量付费资源。
折扣是 PAYG 权益，不应该降低 quota 的消耗价值。
为什么不是先扣钱包再扣 quota？
因为用户购买套餐后，预期就是优先消耗套餐额度。如果平台反过来先扣钱包，客户账单解
释会很困难。
在我们的实现里，扣费结果会拆成几类：
结果 含义
quota tokens / units 本次由租户或用户额度覆盖的用量
wallet tokens / units 本次需要钱包承担的用量
wallet cost 钱包实际应扣金额
discount amount 钱包部分获得的折扣
monthly counter adjustment 月阶梯折扣计数的补偿记录
这些字段看起来多，但每一个都对应一个后续问题：
客户问为什么没扣钱包，要看 quota 覆盖。
客户问为什么扣了这么多，要看 wallet cost。
客户问等级优惠在哪里，要看 discount amount。
客户问月阶梯为什么没命中，要看 monthly counter。
财务问这次毛利多少，要看 customer cost 和 upstream cost。

## 07 账本落点：usage log 和 charge line 都要有

只写一个请求总金额，不够。
一个可收费的 AI Token 平台，至少要保存六类记录：
记录 回答的问题
usage log 这次请求是谁发的、用哪个模型、总共扣了多少
charge line 每个维度分别用了多少、单价多少、金额多少
wallet transaction 钱包到底扣了多少钱
quota ledger 哪些额度包被消耗
API Key spend 这个 key 是否超过预算限制

记录 回答的问题
provider evidence 上游到底怎么计费，用于对账
usage log 是请求级账本。
它回答“这一次请求整体发生了什么”。
charge line 是维度级明细账本。
它回答“这一次请求里每个计费维度怎么算出来的”。
例如一条请求可能同时有：
input uncached
cached input read
cache write
output
reasoning output
image output units
如果只有总金额，客服无法解释每个维度；财务也无法检查明细加总是否等于总金额。
所以 charge line 需要和 usage log 做一致性校验：
sum(customer charge lines) = usage log customer cost
sum(upstream cost lines) = usage log upstream cost
LiteLLM spend 和 provider invoice 可以作为上游证据，但不要直接覆盖客户账。
客户最终账本应该以平台 usage log 和 charge line 为准。
原因很简单：上游只知道 provider 侧消耗，不知道平台客户价、OEM 定价、用户等级折
扣、套餐抵扣和钱包扣款。

## 08 常见错误方案
### 错误一：只有一张模型单价表

模型单价表可以作为起点，但不能作为完整计费引擎。
它表达不了 cache、image size、video duration、audio seconds、TTS characters，也
表达不了租户价格和条件计价。

### 错误二：把折扣直接写进模型价格

这样会让模型价格、用户等级、OEM 客户等级、模型固定折扣和月阶梯折扣混成一团。
短期实现简单，长期账单无法解释。

### 错误三：套餐额度也打折

quota 是预购资源，PAYG 折扣应该只作用在钱包实扣部分。
套餐额度二次打折，会让套餐价值、钱包权益和毛利核算都变得混乱。

### 错误四：请求后才检查余额

请求后才扣费，意味着平台先承担上游成本。
在高并发下，即使单个请求看起来金额很小，也可能迅速放大成预算缺口。

### 错误五：把上游账单当客户账单

上游成本只证明 provider 侧消费。
它不等于客户价格、折扣、套餐抵扣、钱包扣款和 OEM 结算规则。

### 错误六：只写一个 cost 字段

只写总金额，后续所有问题都会变成口径争议。
更稳的做法是：请求级 usage log 加维度级 charge line，再配钱包流水和 quota ledger。

### 错误七：OEM 和平台直客共用一套等级折扣

这会造成权益串租户。
平台直客等级、OEM 客户等级、租户模型选品和租户价格应该分层解析。

## 09 计费引擎上线前 18 项检查清单

下面这张表可以直接拿去做计费模块上线前评审。
检查项 判断标准
模型基础价 平台模型必须有可用客户价
条件定价 是否支持 request type、计量单位和参数条件
租户价格 OEM 模型选品和价格是否独立可控
PAYG 开关 关闭 PAYG 时是否只允许 quota 覆盖
用户等级 平台直客等级和 OEM 客户等级是否分开解析
模型折扣 指定模型折扣是否优先于等级默认折扣

 检查项         | 判断标准                                           
 月阶梯         | 是否支持自然月、metric、跨阶梯拆分和补偿                        
 折扣范围        | 折扣是否只作用于钱包部分                                   
 预估用量        | 请求前是否估算最大可能用量                                  
 预算预留        | 是否在上游调用前占用钱包和 API Key budget                   
 配额优先        | tenant quota、user quota、wallet fallback 顺序是否清晰 
 钱包实扣        | 是否有幂等 wallet transaction                       
 扣费失败补偿      | wallet、quota、monthly counter 是否可补偿             
 Usage log   | 是否保存请求级客户账本                                    
 Charge line | 是否保存维度级明细账本                                    
 价格快照        | 是否保存 price source、version、FX 和渠道成本             
 毛利保护        | 客户价低于上游成本时是否拒绝继续放量                             
 对账证据        | provider evidence 与客户账本是否分层                    
这张清单的价值不在于“把字段补齐”，而是把收费上线前的风险点逐个关掉。
只要其中几项没有设计清楚，后面大概率会在余额透支、账单争议、OEM 串价或毛利失控里
付成本。

## 10 最后

 AI Token 计费不是  | usage * price | 。   
那只是最小公式。
一个企业级计费引擎要回答三件事：
请求前：这次请求能不能打上游？
请求后：最终应该扣多少钱？
争议时：平台凭什么这么扣？
所以它必须同时拥有定价体系、折扣体系、配额体系、钱包预留、扣费补偿和账本证据。
为什么 OEM 不是简单加一个租户字段；以及租户模型选品、下游客户等级、配额池、钱
包、代充值和平台结算边界应该怎么拆。

这个系列会继续围绕企业级 Token 聚合平台，把网关、路由、计费、对账、OEM 和上线门
禁拆成可以复用的工程清单。
AI聚合网关之三:NewAPI、LiteLLM 和自研原 AI聚合平台之五：设计一个企业级 Token 聚
生 Adapter 的边界 合平台OEM体系

---


## 第5篇：OEM体系：设计一个企业级 Token 聚合平台OEM体系

从租户状态机、Host 租户识别、模型选品、客户等级、代充值、AI 配额池到 API Key 边界，拆一
套 AI Token 聚合平台的 OEM 功能实现。
能收费上线的 OEM 体系，至少要回答这 8 个工程问题：
 问题       | 当前实现口径                           
 租户怎么开通和停 | Admin 显式创建、审核、暂停、恢复、终止，状态机控制入口放行 
用
 请求怎么识别租户     | Host 优先，受信来源才允许 Header 备选，未知 Host 不回落平台   
 数据怎么隔离       | Repository 显式接收 tenantID，平台数据和 OEM 数据分开查询 
 OEM 能卖什么模型   | 平台模型目录左连接租户配置，租户独立上架、下架、定价                
 客户怎么运营       | 租户内客户列表、封禁、手动调级、代充值和客户分析                  
 折扣怎么生效       | OEM 客户等级独立配置，AI PAYG 折扣只作用在按量钱包扣费         
 配额怎么分配       | Admin 给租户发放 AI 配额池，OEM 再分配给下游客户，请求时优先扣配额  
 API Key 能不能代 | 当前只开放客户自管，OEM 代管 Key 隐藏，等权限、审计和归属校验齐了再    
 管            | 开                                         

OEM 分销不是换一个入口，而是把平台能力拆成租户化的开通、选品、定价、客户、资金、
配额、账本和权限边界。
这篇不讲抽象多租户概念。
我只讲我们当前这套 AI Token 聚合平台已经落地的 OEM 功能设计，以及为什么这些细节决
定它能不能真正对外分销。

## 01 先把 OEM 拆成 4 个角色，而不是 1 个 tenant_id

 tenant_id |  只是数据字段。 
OEM 是商业角色、权限边界和账本边界的组合。
我们当前把 OEM 体系拆成四类角色：
这四个角色最关键的差异是：
 角色       | 可以做什么               | 不能做什么           
 平台 Admin | 创建租户、审核、暂停、终止、查看平台账 | 不绕过审计直接代客户改业务结果 
本和审计
 OEM Owne   | 管自己的客户、模型选品、客户等级、代充     | 不看平台供应商凭证、全局路由策略 
 r          | 值、配额和经营报表               | 和其他租户数据          
 OEM  下 游   | 使用自己的 API Key、钱包、配额、账单和 | 不访问同租户其他客户资源     
 客户         | 调用日志                    
 Gateway/Bi | 执行租户校验、模型门禁、预算扣费和证据     | 不相信前端参数决定租户归属    
 lling      | 落账                      

这里最容易犯的错误，是把 OEM Owner 和 OEM 下游客户都当成“租户用户”。
它们不是一种角色。
Owner 是经营者。
下游客户是消费者。
经营者可以做选品、定价、代充值、客户等级和配额分配；消费者只能使用自己的 Key、钱
包、额度和账单。
 所以我们没有把 OEM 做成一个简单的  |                       | tenant_id |  字段改造，而是把它拆成一组后台能力：    
 -------------------- | --------------------- | --------- | ---------------------- 
 功能域                  | 当前能力                  |           | 解决的问题                  
 租户生命周期               | 创建、审核、暂停、恢复、终止        |           | 防止租户被普通 CRUD 误恢复或误放行   
 租户入口                 | 域名解析、Header 受信备选、状态拦截 |           | 防止未知 Host 和伪造租户 Header 
 商品和模型                | 租户商品、租户 AI 模型、租户价格    |           | 防止平台目录默认暴露给所有 OEM      
 客户运营                 | 客户列表、封禁、等级、代充值        |           | 让 OEM 能独立经营下游客户        
 资金配额                 | Owner 钱包、客户钱包、AI 配额池  |           | 区分钱和用量资源               
 API Key              | 客户自管，代管暂缓             |           | 避免密钥泄露和跨客户越权           
 账本可见性                | 平台账、OEM 账、客户账分层       |           | 让经营报表有边界，底层账本可追责       
这张表就是本文最核心的判断：
OEM 能不能上线，不看有没有换 Logo，而看这几条链路有没有闭环。

## 02 租户开通不是后台 CRUD，而是状态机

租户不是后台字典项。
如果租户管理只是普通 CRUD，会出现几类很难解释的问题：
审核中的租户被直接改成 active。
已终止的租户被误恢复。
Owner 账号被绑定到多个租户。
高风险租户暂停后，API 入口还在继续调用。
出问题后只看到一个状态值，看不到是谁做了动作。
所以我们把租户生命周期做成显式动作，而不是任意字段更新。
当前状态流转是：
```

create
-> pending_review
-> approve -> active
-> reject -> rejected
active
-> suspend -> suspended
-> terminate -> terminated
suspended
-> activate -> active
-> terminate -> terminated
不同状态直接决定租户能力能不能放行：
状态 含义 当前处理口径
pending_review 已创建，待审核 不开放正式租户能力
active 正常运营 正常放行
rejected 审核拒绝 不允许通过普通状态接口恢复
suspended 暂停运营 默认拦截，只保留少量自服务维护入口
terminated 终止 不允许通过普通接口恢复
创建租户时，系统不是只插一行租户主数据。
它会同时完成几件事：
校验租户编码格式和唯一性。
校验 Owner 用户存在、未绑定其他 OEM、不是 Admin。
要求操作人确认 Owner 绑定。
初始化默认品牌配置。
初始化运行配置，默认不开放 OEM 代管 API Key。
初始化结算配置。
把 Owner 绑定到租户。
写入审计事件。
审核和状态变更也是独立动作：
```
Admin create tenant
-> pending_review
Admin review tenant
-> approve / reject

Admin change tenant status
-> suspend / activate / terminate
这看起来比 CRUD 麻烦。
但它换来的东西很重要：每个状态变化都有合法来源、合法前置状态、操作人、原因和审计
记录。
在 OEM 场景里，这不是“后台严谨一点”的问题，而是平台能不能控制分销风险的问题。

## 03 租户识别：Host 优先，未知 Host 不能自动落到平台

OEM 的入口边界从请求进来的第一秒就开始了。
我们当前的租户识别策略是：
```
1. 先用 Host 查租户域名
2. Host 没命中时，只有平台白名单 Host + 受信来源，才允许 Header 备选
3. 仍没命中时，只有平台白名单 Host 可以回落到平台租户
4. 其他未知 Host 直接返回 tenant not found
5. 命中租户后，再校验租户状态
这里有三个关键点。
第一，域名命中后不允许 Header 覆盖。
如果一个请求已经从某个 OEM 域名进来，就应该以 Host 解析结果为准。否则调用方可以
通过 Header 把请求切到另一个租户。
第二，未知 Host 不回落平台。
很多系统早期会写一个方便逻辑：识别不到租户，就当平台入口。
在 OEM 里这很危险。
未知 Host 应该 fail-closed，而不是悄悄变成平台租户请求。
第三，缓存只是热路径，不是事实源。
我们会把租户解析结果缓存成结构化数据：
```
tenant_id
tenant_code
tenant_status
domain
缓存 miss 时回源数据库，再回填缓存。

中间件最终会把可信租户信息写入请求上下文：
```
tenant_id
tenant_code
tenant_status
后面的鉴权、模型列表、API Key、计费、账单、审计，都只认这个上下文，不认前端传来的
租户参数。
这就是为什么 OEM 不能只靠前端路由和页面皮肤。
租户边界必须从入口中间件开始。

## 04 多租户隔离：显式 tenant scope，而不是 ORM 魔法

 多租户隔离不是所有表都有  |     | tenant_id |  就结束了。 
真正的隔离发生在每一次查询和写入。
我们当前没有做 ORM 全局透明注入，而是继续使用显式 tenant scope：
```
Service 从可信上下文拿 tenant_id
-> Repository 方法显式接收 tenantID
-> tenant_id IS NULL 或 tenant_id = ? 或 Admin scope-all
原因是 AI Token 聚合平台里有三种不同查询语义：
 场景            |     | 查询口径                           |     | 风险控制         
 平台直客          |     | tenant_id IS NULL + user_id    |     | 不误读 OEM 用户资源 
 OEM 下游客户      |     | tenant_id = ? + user_id        |     | 不跨租户、不跨客户    
 OEM Owner 管客户 |     | tenant_id = ? + target_user_id |     | 目标客户必须属于当前租户 
 Admin 全局视角    |     | 专用 Admin scope-all             |     | 必须有 RBAC 和审计 
如果用全局透明注入，短期省代码，长期会出现两个问题。
一个是 Admin 跨租户查询不好表达。
Admin 有些页面确实需要全局视角，但它必须走专用 Admin 路径、权限和审计，而不是偷
偷关掉全局过滤。
另一个是平台数据和 OEM 数据不是同一种过滤。
 平台直客是  | tenant_id IS NULL | 。   

OEM 是 tenant_id = ? 。
这两个不能混在一起。
所以我们宁愿让仓储接口多传一个 tenantID，也要让每条读写路径在代码审查、测试和审计
里都能看见。
这也是 OEM 文章里最容易被忽略的实现点：
隔离不是一个字段，是每条业务路径的查询语义。

## 05 OEM 路由能力：把 Admin、Owner、客户入口分开

当前 OEM 能力不是一个大而全的后台页面，而是一组按角色拆开的接口能力。
Admin 侧负责租户治理：
Admin 能力 作用
租户列表和详情 查租户状态、Owner、配置和经营信息
创建租户 绑定 Owner，初始化默认配置
审核租户 pending 到 active 或 rejected
状态管理 suspend、activate、terminate
租户商品和客户等级查看 平台视角辅助运营和排障
租户结算和提现审核 平台财务视角控制风险
租户配额池调整 给 OEM 发放或扣减 AI 配额资源
OEM Owner 侧负责租户经营：
OEM Owner 能力 作用
租户设置和品牌 管理站点展示和基础配置
域名绑定 建立 OEM 入口
商品选品 控制可售商品和租户价格
AI 模型选品 控制可售模型、PAYG 和 quota 策略
客户管理 列表、搜索、封禁、解封、等级、代充值
客户等级 配置折扣、AI 权益、专属 SKU 和服务等级
配额池分配 给下游客户发放和回收 AI 配额
经营报表 看客户收入、用量、排行和结算数据

客户侧只拥有自己的资源：
客户能力 作用
自己的 API Key 创建、查看掩码、启停、配置 allowed models
自己的钱包 充值、消费、账单
自己的 AI 配额 查看可用额度和消耗
自己的调用日志 排查请求和账单
这套拆法的好处是：
Admin 不需要进入 OEM 日常运营细节。
OEM Owner 不接触平台供应商和底层成本。
下游客户不接触 OEM 后台经营信息。
Gateway 和 Billing 始终从租户上下文、用户身份和 Key 归属执行校验。
OEM 不是把所有按钮塞进一个后台。
它是把“平台治理、租户经营、客户使用、网关执行”这四层分清楚。

## 06 AI 模型选品：平台模型目录不等于租户可售目录

AI Token 聚合平台做 OEM 时，模型选品是最核心的商业能力之一。
平台有某个模型，不代表所有 OEM 都应该能卖这个模型。
我们当前把模型拆成两层：
```
平台 active 模型目录
-> OEM 租户模型配置
-> 上架 / 下架
-> allow_payg
-> quota_enabled
-> 租户级价格
-> 终端模型列表
-> Playground
-> OpenAI-compatible /v1/models
-> API Key allowed_models
-> Gateway 调用前模型门禁
OEM 管理页返回的不是“租户已经配置过的模型列表”。
而是：
```

平台 active 模型目录
left join
租户模型配置
这样一行模型里可以同时看到：
平台模型信息。
平台参考价格。
当前租户是否已选择。
当前租户价格。
PAYG 是否允许。
Quota 是否允许。
当前租户状态。
为什么要这么做？
如果只返回已配置模型，OEM 就看不到还有哪些平台模型可以上架，也没法做批量上架、下
架和定价。
更重要的是，模型列表页不是最终门禁。
真正调用时，Gateway 还会按 tenant_id + model_code 再查租户模型配置：
状态 Gateway 行为
租户未上架 拒绝
租户已下架 拒绝
PAYG 关闭且 quota 未开启 拒绝
PAYG 开启但必要价格缺失 拒绝
上架且价格完整 使用租户价格参与预估、扣费和 usage log
这里最关键的是价格门禁。
PAYG 模型必须配置完整的租户价格：
模型类型 必要价格
Chat / Embedding / Responses input + output
Image per image
Audio transcription per second
TTS per char

价格字段全部按 decimal 处理，前端以字符串传输。
不允许用 float。
也不允许出现“模型能调用，但价格缺失导致免费打上游”的半可用状态。
模型选品不是展示功能，它是 Gateway 计费门禁的一部分。

## 07 客户等级：OEM 自己的等级，不复用平台会员等级

OEM 下游客户不是平台直客的子集。
他们有自己的等级、折扣、权益和运营策略。
我们当前的客户等级能力包括：
能力 当前实现口径
等级管理 OEM 在租户内创建、编辑、删除等级
唯一性 level_code
只要求同一租户内唯一
调级方式 由 OEM 后台手动调级，不做自动升级
商品折扣 用于租户商品和 SKU VIP 价
AI 折扣 ai_discount_rate
用于 AI PAYG 钱包扣费
专属 SKU 控制部分商品可见和权益
服务等级 用于客服和运营分层
这里有一个非常重要的边界：
AI PAYG 折扣不回退到商品折扣。
商品折扣和 AI 按量扣费是两种不同的账。
商品折扣影响 SKU、套餐、订单价。
AI PAYG 折扣只影响钱包按量扣费部分。
它不影响已经分配的 AI 配额消耗，也不应该和商品折扣混用。
Gateway 构建路由上下文时，会按当前用户和租户查客户等级：
```
tenant_id > 0
-> OEM customer level

tenant_id = 0
-> platform user level
这让平台直客等级和 OEM 客户等级在运行时分开。
同名 VIP 等级也不会互相污染。
如果不拆这层，后面会出现很典型的账单争议：
```
为什么平台 VIP 折扣影响了 OEM 客户？
为什么 A 租户的 Enterprise 等级影响了 B 租户？
为什么商品折扣被拿去算 AI 按量账单？
所以客户等级不是运营后台的小功能。
它是 OEM 计费解释能力的一部分。

## 08 客户管理：封禁、调级、代充值都必须带租户边界

OEM Owner 管客户，不能只是“用户列表加筛选”。
当前客户管理至少覆盖四条链路：
能力 当前实现口径
客户列表 只查当前租户下用户，支持关键字、状态、分页
封禁 / 解封 使用用户域统一状态
手动调级 目标等级必须属于当前租户
代充值 从 OEM 操作人钱包转到目标客户钱包
封禁这件事尤其容易做错。
很多后台只改客户列表里的状态，但登录、下单、支付、API 调用链路没有统一识别。
这样客户被封禁后，仍然可能用旧会话继续下单，或者继续支付已创建订单。
我们的口径是：
```
客户封禁
-> 用户域状态变为 banned
-> 登录链路识别
-> 订单创建链路识别
-> 支付链路再次识别
-> API 使用链路继续受 tenant/user 状态约束

代充值也不是“直接把客户余额加大”。
它是一笔同租户内的钱包转账：
```
OEM operator wallet
-> wallet transfer
-> OEM customer wallet
当前代充值链路会做这些校验：
金额必须为正。
租户 ID、操作人 ID、目标客户 ID 必须有效。
目标客户必须属于当前租户。
操作人不能给自己代充值。
钱包扣减和入账在同一事务里完成。
写钱包流水。
写审计事件。
这几个细节决定了代充值是不是一笔能解释的资金动作。
如果只是直接改余额，后面客户问“谁充的、从哪里扣的、为什么多了一笔钱”，平台很难给出
证据。

## 09 钱包额度和 AI 配额池必须分开

OEM 体系里，“额度”这个词最容易混。
我们当前强制拆成两类：
```
钱包余额
钱，用于充值、代充值和 PAYG 钱包扣费
AI 配额池
用量资源，用于平台给租户发放，再由租户分配给下游客户
两条链路分别是：
```
Admin 发放 OEM 钱包额度
-> OEM Owner wallet
-> 代充值 transfer
-> OEM customer wallet
Admin 发放 AI tenant quota pool
-> tenant_quota_pools

-> tenant_quota_allocations
-> AI 请求优先扣配额
钱包余额解决的是资金。
AI 配额池解决的是用量资源。
它们不能放进同一张账里。
当前 AI 配额池有三张核心账：
对象 作用
tenant_quota_pools 租户总配额池，记录总量、已分配、已使用
tenant_quota_allocations OEM 分配给下游客户的配额
tenant_quota_ledger 发放、分配、回收、扣减、恢复的流水证据
配额链路有几个硬约束：
发放、分配、回收、扣减都有幂等键。
配额池可分配量 = 总量 - 已分配量。
客户可回收量 = 已分配量 - 已使用量。
分配前校验目标客户属于当前租户。
扣减时锁定 allocation 和 pool，防止并发超扣。
单次 AI 消耗可以跨多个 allocation 扣减。
usage log 或后续落账失败时，可以按扣减明细恢复配额。
请求进入 AI 网关时，如果结算策略要求 quota allocation，配额不足就直接拒绝，而不是
悄悄兜底钱包。
这个设计看起来保守，但它避免了一类很难解释的问题：
```
客户以为自己买的是配额
平台发现配额不够后自动扣了钱包
OEM 和客户都认为账单口径变了
所以配额模式要么明确 fallback 策略，要么 fail-closed。
不能靠隐式兜底解决。

## 10 API Key 代管：当前隐藏，不半开放

OEM 代管下游客户 API Key 是一个很有诱惑力的功能。

渠道商会希望能替客户：
创建 Key。
禁用 Key。
设置模型白名单。
设置 spend limit。
重置 Key。
查看客户 Key 状态。
但这个能力不能半开放。
当前我们的边界是：客户 API Key 自管，OEM 代管 Key 隐藏。
也就是：
当前登录用户只能管理自己的 Key。
Key 明文只在创建时返回一次。
列表只返回 masked key。
Gateway 按 Key 绑定的 tenant/user 计费。
请求体里伪造目标客户不会改变 Key 归属。
不提供 /tenant/customers/:id/api-keys 这类 OEM 代管入口。
为什么不先做一个简单版？
因为代管 Key 一旦开放，至少要补齐 6 个条件：
条件 必须解决的问题
目标客户归属校验 防止 OEM 操作其他租户客户
操作人权限和租户开关 防止普通成员越权代管
明文只创建时返回 防止后台长期查看密钥
allowed_models 校验 防止 Key 绕过租户模型选品
spend limit 校验 防止 Key 绕过预算策略
全量审计 追踪谁替哪个客户做了什么
这里的核心不是“能不能创建 Key”。
而是：
```
谁有权替谁创建？
创建后费用归谁？
客户能不能知道这件事？

泄露后责任怎么追？
审计里能不能还原动作？
在这些条件没有完整闭环之前，隐藏比半开放更稳。
这也是 OEM MVP 里很重要的产品判断：不是所有“渠道商想要”的能力都应该第一版开放。
高风险能力必须等权限、审计、密钥展示和计费边界齐了再开。

## 11 账本可见性：OEM 要经营报表，不等于看平台全部账

OEM 需要看经营结果。
但经营结果不等于平台全局账本。
当前可见性拆成三层：
视角 可以看 不该看
OEM Own 下游客户收入、调用量、模型分布、客户排 平台供应商凭证、全局上游成本、其
er 行、租户结算 他租户数据
OEM 下 游 自己的 API Key、用量、账单、钱包、配额 同租户其他客户数据、OEM 后台利润
客户
平台 Admi 全局账本、上游成本、风控和审计 明文 API Key、绕过审计的代入操作
n
这和前面计费引擎文章里的结论一致：
客户最终账本以平台的 usage log、charge line、钱包流水和 quota ledger 为准。
但展示给不同角色时必须裁剪。
OEM 看板应该服务经营：
客户收入。
请求量。
Token 用量。
模型分布。
Top 客户。
客户趋势。
结算和提现。
它不应该把平台内部的供应商凭证、合同成本、全局路由策略直接暴露出来。
否则 OEM 很快会从经营后台变成平台内部成本后台。
这会带来两个问题：

第一，平台商业空间被暴露。
第二，供应商和全局成本数据变成新的越权面。
所以 OEM 报表要能解释客户账单，但不能泄露平台底层账本。

## 12 当前 OEM 功能实现清单

下面这张表是当前这套 OEM 体系可以拿去做技术评审的实现清单。
 能力域  | 当前已落地的关键点                      | 上线判断     
 租户创建 | 租户编码唯一、Owner 绑定、默认品牌配置、默认运行配置、 | 不是裸 CRUD 
结算配置
 租户审核 | pending_review 到 active / rejected，拒绝必须有原因 | 可追责     
 租户状态 | active、suspended、terminated 显式动作流转         | 高风险租户可停 
用
 租户入口  | Host 优先，受信 Header 备选，未知 Host fail-closed | 入口可信    
 租户上下文 | 注入 tenant_id、tenant_code、tenant_status   | 后续链路统一取 
信
 数据隔离 | 显式 tenant scope，平台 NULL tenant 和 OEM tenant 分 | 可测试、可审计 
开
 Admin 边界 | Admin 专用 scope-all、RBAC、审计 | 不靠普通接口越 
权
 模型选品    | 平台目录左连接租户配置                              | OEM 可独立上架 
 模型定价    | 租户级 input/output/image/audio/TTS 价格      | 可独立售卖     
 PAYG 门禁 | 价格不完整时拒绝列表展示和调用                          | 防免费打上游    
 模型列表    | 终端模型、Playground、OpenAI-compatible 模型列表受租 | 展示和调用一致   
户配置约束
 API Key 白名 | allowed_models 写入前校验租户可售模型 | Key 不绕过选品 
单
 客户列表     | 只查当前租户客户，支持筛选和分页      | 不跨租户    
 客户封禁     | 登录、下单、支付链路识别 banned   | 封禁真正生效  
 客户等级     | 租户内唯一，手动调级            | 等级不串租户  
 AI 折扣    | AI PAYG 折扣独立于商品折扣     | 账单可解释   
 代充值      | 同租户钱包转账，不直接改余额        | 资金可追踪   
 Owner 钱包 | Admin 发放钱包额度给租户 Owner | 钱包和配额分离 

 能力域        | 当前已落地的关键点                      | 上线判断     
 AI 配额池     | pool / allocation / ledger 三层账 | 用量资源可分配  
 配额幂等       | 发放、分配、回收、扣减、恢复都有幂等和事务锁         | 防重复和并发   
 配额扣减       | AI 请求优先扣租户分配额度，失败可恢复           | 账实一致     
 API Key 自管 | 当前用户只能管自己的 Key，明文只返回一次         | MVP 风险可控 
 API Key 代管 | OEM 代管隐藏，等权限、审计、归属校验齐再开        | 不半开放高风险  
能力
 可见性裁剪 | OEM 看经营数据，平台保留底层账本 | 不泄露平台成本 
这张表比“有没有白标页面”更适合判断 OEM 能不能上线。
只要其中几项没有收口，后面大概率会在这几类问题里付成本：
跨租户越权。
客户串价。
模型免费调用。
代充值对不上账。
配额并发超扣。
API Key 代管泄露。
OEM 报表暴露平台成本。

## 13 常见错误方案
### 错误一：把 OEM 做成一套白标前端

白标前端只能解决“看起来像谁”的问题。
OEM 要解决的是：谁能卖什么、谁能改什么、钱怎么走、账怎么看、出事后谁负责。

### 错误二：公开请求靠参数传 tenant_id

租户归属必须来自可信 Host、认证上下文或受信内部入口。
如果公开请求可以靠前端参数决定租户，后面的鉴权、计费和账本都会建立在不可信输入
上。

### 错误三：平台模型默认给所有 OEM 可售

平台模型目录不等于租户可售目录。
OEM 必须独立控制模型上架、下架、价格、PAYG 和 quota 策略。

### 错误四：平台等级和 OEM 客户等级混用

平台直客等级属于平台。
OEM 客户等级属于租户。
混用会导致权益串租户，折扣账单也解释不清。

### 错误五：代充值直接改客户余额

代充值必须是一笔钱包转账。
它要有扣款方、收款方、金额、事务、流水、审计和失败处理。

### 错误六：把钱包额度和 AI 配额池叫成一个东西

钱包是钱。
AI 配额池是用量资源。
它们可以共同参与扣费策略，但不能混成同一张账。

### 错误七：提前开放 OEM 代管 API Key

没有目标客户归属校验、masked key、审计、模型白名单和 spend limit 校验时，代管
Key 是高风险能力。
先隐藏，比半开放更稳。

### 错误八：OEM 看板直接展示平台上游成本

OEM 需要经营分析，不等于可以看平台全部成本和供应商信息。
平台账、OEM 账、客户账要分层。

## 14 最后

OEM 分销不是换 Logo。
它是一套完整的租户化经营系统：
```
入口要可信
数据要隔离
模型要选品
价格要完整
客户要能运营
资金要能追踪
配额要能恢复
Key 要有边界
账本要分视角

做 AI Token 聚合平台时，OEM 是一个很自然的商业扩展方向。
但它不是前端项目，也不是一个字段改造。
它要求平台把网关、模型、计费、钱包、配额、账本和权限都变成租户化能力。
为什么 Token 聚合平台不能只靠单实例网关和单渠道 provider，而要同时设计网关无状
态、预算预留幂等、渠道健康检查、自动降级、Fallback 和对账证据。
这个系列会继续围绕企业级 Token 聚合平台，把网关、路由、计费、对账、OEM 和上线门
禁拆成可以复用的工程清单。
AI聚合网关之三:N喜e欢wA作PI者、LiteLLM 和自研原生
Adapter 的边界
链路
AI聚合网关之四：从用户等级A、IO聚E合M平 到台完之整五：设计一A个I聚企合业平级台 To之k六en： 聚To合ke平n转发后端架构与LLM
台OEM体系
扣费链路 渠道高可用设计
高可用设计
```

---


## 第6篇：后端架构：Token转发后端架构与LLM渠道高可用设计

刚好今天周末,我们花点时间来聊聊从模块化单体、API/Worker 运行角色、未来微服务边界，到转
发实例、共享池、租户独享池、channel 调度、负载均衡和健康监测。
设计之初,主要是下面这2个问题.
第一个问题：后端服务要不要一开始就拆微服务？
第二个问题：LLM 上游不稳定，渠道要怎么做冗余、负载均衡和故障隔离？
这两个问题都和高可用有关，但不是同一层问题。
后端 API 和 Worker 解决的是平台业务复杂度：用户、租户、订单、财务、AI 计费、对账、
审计，这些能力怎么在一个可交付的系统里协作。
LLM 渠道层解决的是上游执行复杂度：同一个模型背后可能有多个上游、多个区域、多个转
发实例、共享池、租户独享池，平台要怎么选路、限流、降级和恢复。
所以我们当前的架构判断是：
 问题   | 当前设计                    |     | 为什么            
 后端业务 | 模块化单体 + API/Worker 运行角色 |     | 先保证交付效率、本地事务和清 
 复杂度  |                         |     | 晰模块边界          

 问题   | 当前设计                      | 为什么              
 异步和补 | Worker 承接定时任务、健康检查、对账、异步落 | 不把慢任务和可重试任务压在 AP 
 偿    | 账                         | I 请求线程           
未来微服 service interface、repository interface、outb 等组织边界和独立交付需求出现
 务      | ox、队列抽象、任务状态机                      | 后再拆            
 LLM 渠道 | 转发实例 + channel + 共享池/独享池 + Route P | 上游不稳定要在渠道层解决，不 
 冗余     | olicy                              | 靠拆业务服务解决       
负载和健 Redis 实时负载 + health score + Worker 健康 只有当前能接流量的 channel 才
 康   | 任务  | 进入候选池 
不想单独纠结讲“单体好还是微服务好”,那句PHP是最好的语言的战争还历历在目。
摆一个更实际的问题,一切从需求出发：
在 AI Token 聚合平台这个阶段，哪些边界应该先做成代码边界和运行角色，哪些边界应该
等组织和业务稳定后再拆成微服务。

## 先把两类高可用分开

如果把“平台业务高可用”和“模型渠道高可用”混成一个问题，后面一定会设计混乱。
平台业务高可用关注的是：
API 能不能稳定承接同步请求。
预算、钱包、额度、API Key spend limit 能不能在高并发下守住。
Worker 能不能处理异步账单、健康检查、对账、通知、补偿。
模块之间有没有清晰契约，未来是否能拆。
模型渠道高可用关注的是：
同一个公开模型能不能配置多个 channel。
同一个上游路线能不能部署到多个转发实例。
普通用户、平台高级用户、OEM 租户、独享客户能不能走不同池子。
路由时能不能按健康分、实时负载、权重和策略选择。
一个转发实例不可用时，系统是按策略回退，还是明确失败。
这两类问题不能互相替代。
后端拆成微服务，解决不了上游模型区域故障、账号限流、转发实例拥塞。
LLM 渠道做了冗余，也解决不了平台内部账本、租户、订单、对账边界混乱。
平台业务边界和模型执行边界必须分层设计。

## 为什么当前没有一上来拆微服务

我们早期确实评估过微服务形态：用户服务、订单服务、财务服务、租户服务、AI 服务、通
知服务、工单服务等，每个域都可以拆成独立进程。
但从工程现实看，当前阶段不适合这么做。
当前约束 如果过早拆微服务会怎样
业务边界仍在变化 服务边界会频繁调整，跨服务接口反复迁移
团队还没有按业务域形成独立小队 拆服务不会降低协作成本，反而会增加联调和排障成
本
用户、租户、订单、财务、AI 计费仍需要频 本地事务能解决的问题，会被提前变成分布式一致性
繁联动 问题
运行基础设施还要控制复杂度 多服务部署、配置、灰度、监控、追踪、告警都会提
前膨胀
很多能力仍处在快速演进期 每次需求变化都可能穿透多个服务，交付速度下降
微服务不是“把代码拆小”。
我一直认为,微服务真正解决的是企业内部组织架构边界问题,而不仅仅是代码边界：
不同团队可以独立交付。
不同业务域可以独立发布。
不同服务可以独立扩缩容。
某个域故障时有清晰责任边界。
数据生命周期和一致性策略可以按业务域独立演进。
如果组织还没有形成这些边界，微服务会先带来成本，而不是收益。
更准确地说：
服务边界不是照抄大厂或者脑补出来的，是内部组织协作和交付节奏长出来的。
当前我们更需要的是一个边界清晰的模块化单体，而不是一组边界还不稳定的微服务。

## 当前不是“大单体”，而是模块化单体

不拆微服务，不等于把所有代码堆在一个目录里。
我们现在的后端结构是模块化单体：一个应用进程内包含多个业务模块，但每个模块有自己
的层次和契约。
一个典型模块内部是这样的：

```
module
-> handler      只做 HTTP binding、参数校验、响应格式
-> service      编排业务流程
-> repository   通过接口访问数据
-> model        承载领域状态和领域方法
模块之间不应该互相穿透。
比如订单模块需要用到用户能力，它应该依赖用户模块暴露出来的 service interface，而不
是直接去访问用户模块内部 repository。
repository 也不是随便查表。多租户场景里，租户上下文必须贯穿到数据访问层。一个用户
 ID 本身不够安全，很多查询必须同时受  | tenant_id |  和资源归属约束。 
这套结构带来的收益不是“目录好看”，而是未来可拆。
 当前做法                               | 现在的收益      |     | 未来拆服务时的收益          
 service interface                  | 模块间调用可控    |     | 可替换成 RPC / HTTP 契约 
 repository interface               | 数据访问可测试、可替 |     | 可迁移到独立 DB 或远程 clie 
                                    | 换          |     | nt                 
 handler/service/repository/model 分 | 业务规则不散落在入口 |     | 拆服务时迁移边界更清楚        
 层                                  | 层          
 统一错误码和响应 envelope                  | API 行为一致   |     | 网关和服务间协议更稳定        
 租户上下文贯穿                            | 多租户隔离可证明   |     | 拆库或拆服务时边界明确        
所以当前的核心不是“单体还是微服务”，而是：
我们先把业务边界做成代码边界、接口边界、事务边界和运行角色边界。等组织边界出现，
再把其中一部分提升为服务边界。

## API 和 Worker 是运行角色，不是两个业务服务

当前后端只有一个应用入口，但通过运行角色启动成 API 或 Worker。
```
同一套代码
APP_ROLE=api     -> 启动 HTTP API
APP_ROLE=worker  -> 启动异步任务和定时任务
这个设计很关键。
API 进程负责同步请求：

API 进程职责 说明
HTTP 接入 对外暴露用户、订单、财务、AI、Admin、OEM 等接口
鉴权和租户上下文 识别用户、API Key、租户、来源策略
请求级门禁 限流、预算预检、模型校验、权限校验
核心事务写入 需要同步完成的状态变更
对外响应 返回前必须能解释本次请求的结果
Worker 进程负责异步和后台任务：
Worker 进程职责 说明
通知投递 外部通道慢且可能失败，不应该占用 API 请求线程
订单超时和履约任务 定时扫描、幂等执行、失败重试
财务任务 账单、统计、对账、补偿
AI 渠道健康检查 定时计算 health score，驱动降级和恢复
转发实例存活检查 定时探测实例可达性，连续失败后降级
异步计费落账 高并发时把钱包、额度、usage log、charge line 从请求线程转移到 Worker
月阶梯计数 flush Redis 原子计数，后台周期落盘
异步对账 发现卡住的 outbox 或需要人工复核的账单事件
API/Worker 分成两个进程，是为了隔离同步请求和异步任务。
但它们现在还不是两个微服务。
原因很简单：Worker 不是一个独立业务域，它是多个业务域的异步执行面。
订单有 Worker 任务，财务有 Worker 任务，通知有 Worker 任务，AI 渠道也有 Worker
任务。如果把 Worker 简单当成“另一个服务”，就容易出现一个坏味道：同步路径写一套业
务规则，异步路径再写一套业务规则。
正确做法是：
API 和 Worker 共享同一套领域模型、repository 契约、配置和基础设施，只是在运行时
承担不同职责。
这就是当前 API/Worker 设计的价值。

## Worker 的工程价值，不是“异步”两个字

很多系统说自己有 Worker，但只是把一些逻辑挪到后台。

真正有价值的 Worker，需要回答三个问题：
1. 哪些事情不应该放在 API 请求线程？
2. 任务失败后怎么重试？
3. 多个 Worker 实例同时运行时，怎么保证同一件事不会重复执行？
我们现在的 Worker 框架做了几件基础能力。

### 第一，支持长任务和定时任务

长任务用于持续消费队列，例如异步计费 outbox。定时任务用于健康检查、对账扫描、月计
数 flush、订单超时处理等。

### 第二，定时任务支持 singleton

多个 Worker 实例可以水平部署，但某些任务同一时间只能有一个实例执行。比如渠道健康
检查、实例存活检查、异步对账扫描，如果多个 Worker 同时跑，可能会重复降级、重复恢
复、重复写审计。
所以 Worker 的 singleton cron 会用 Redis lease 做互斥：拿到锁的实例执行，没拿到锁
的实例跳过；执行期间续约，退出时释放。

### 第三，API 和 Worker 之间通过队列抽象解耦

当前通知链路使用 Redis List 做跨进程桥接，但业务代码依赖的是 Publisher/Consumer
抽象，而不是直接依赖某个具体队列实现。
```
API 侧
-> Publish(event)
-> Redis Queue
Worker 侧
-> Subscribe(topic)
-> claim / handle / retry
这意味着未来切换到 Kafka 时，不需要重写通知消费者业务逻辑，只需要替换
Publisher/Consumer 的基础设施实现。

### 第四，异步计费使用 durable outbox

高并发 AI 请求里，钱包、额度、API Key spend、月阶梯计数都可能成为热行。如果请求线
程在上游返回后同步锁这些表，单用户高 QPS 很容易把延迟打爆。
异步计费的方向是：
```
API 请求线程
-> Redis 原子预算 reserve / pending

-> 写 durable billing event
-> 返回响应
Worker
-> claim pending event
-> 钱包扣款
-> 额度扣减
-> usage log
-> charge line
-> API Key spend
-> 成功标记 posted
-> 失败进入 retry 或 reconcile_required
这里有两个重点。
一个是 outbox 必须 durable。否则 API 返回成功后，后台没有可靠事件，账本就丢了。
另一个是 Worker 必须幂等。钱包扣款有业务引用，额度扣减有 marker，outbox 有状态
机，重复 claim 不应该造成重复扣费。
这才是 Worker 的价值：它不是把慢逻辑“丢到后台”，而是把慢任务、重试任务、补偿任务
做成可证明的异步边界。

## 为未来拆微服务做了哪些铺垫

当前不拆微服务，不代表未来不能拆。
我们真正要避免的是两种极端：
现在就拆，导致每个需求都跨服务联调。
完全不留边界，未来想拆时只能重写。
当前已经做的铺垫可以用一张表概括：
 铺垫    | 当前形态                                  | 未来拆分时怎么用                   
 模块契约  | service interface                     | 替换成 RPC / HTTP contract    
 数据访问  | repository interface                  | 替换成远程 client 或独立 DB 实现     
 运行角色  | API / Worker                          | 拆成 API 服务、worker 服务、对账服务   
 异步边界  | queue / outbox / job                  | 迁移到 Kafka consumer 或独立任务服务 
 幂等身份  | request_id / event_id / business key  | 跨服务重试仍然安全                  
 状态机   | intent / task / outbox / reconcile 状态 | 作为最终一致性锚点                  
 审计日志  | 操作人、原因、状态变化                           | 拆服务后仍能追踪责任                 
 可观测维度 | request、tenant、channel、instance       | 拆服务后变成链路追踪标签               

未来如果要拆，不应该按 controller 文件夹拆，也不应该按数据库表名拆。
合理的拆分顺序更像这样：
1. 先拆运行角色：API / Worker。
2. 再拆异步执行面：通知、对账、健康检查、异步账单。
3. 再拆稳定业务域：用户、财务、AI 账本、渠道管理。
4. 最后再考虑数据物理拆分。
为什么数据拆分放最后？
因为数据一旦物理拆开，事务语义、查询路径、对账方式都会变化。如果业务边界还没稳
定，过早拆库会让每一次产品调整都变成数据一致性问题。

## 什么时候才应该真正转微服务

微服务不是信仰题，而是判断题。
可以用这张 Go / No-Go 表来判断：
 条件    | 适合继续模块化单体    | 适合拆微服务          
 团队组织  | 一个团队维护多个模块   | 多个团队分别负责不同业务域   
 交付节奏  | 模块一起发布可接受    | 某些域必须独立发布       
 扩缩容   | API 多实例能解决   | 某个域有明显独立负载峰值    
 数据一致性 | 本地事务更重要      | 可以接受事件最终一致性     
 故障隔离  | 实例级隔离足够      | 某个域故障必须不影响其他域   
 基础设施  | 监控、队列、灰度还在演进 | 服务治理、追踪、告警、灰度成熟 
当这些条件没有出现时，拆微服务只是在提前支付复杂度。
当这些条件出现时，模块化单体里已经存在的接口、outbox、任务状态机、审计和可观测字
段，就会成为拆分基础。

## LLM 渠道高可用，不能靠后端微服务解决

讲完后端 API/Worker，再看 LLM 渠道。
LLM 渠道的不稳定来自执行层：
某个上游账号被限流。
某个区域网络波动。
某个转发实例健康异常。
某个模型路线临时不可用。
某个共享池被大客户打满。
某个 OEM 独享池配置还没完成。
这些问题不是把 API 拆成用户服务、订单服务、AI 服务就能解决的。
正确的边界是：
```
API
-> 只做平台门禁、预算、路由选择和账本收口
channel
-> 平台侧流量调度和成本归因单元
转发实例
-> 上游协议转发和执行入口
Worker
-> 健康检查、实例探活、路由配置对账、异步账单和补偿
这四个对象的职责不能混。

API 不能变成上游连环重试机器。
转发实例不能变成平台账本系统。
Worker 不能阻塞同步请求。
channel 不能只是 provider key 的别名。

## 转发实例只做转发

转发实例是一个执行入口。
它可以部署在不同区域，也可以按租户、用户等级、业务场景绑定到不同池子。它要做的是
协议转发、上游适配、实例内部 timeout、实例内部 retry、实例内部 provider fallback。
转发实例应该做：
应该做 原因
接收平台选中的执行请求 平台已经完成租户、预算和 channel 选择
按内部路由别名调用上游 屏蔽 provider 之间的模型名差异
处理上游协议差异 让平台侧保持稳定接口
承接实例内部 timeout / retry / fallback 这些属于执行层能力
返回响应、usage 和执行证据 供平台账本和对账使用
转发实例不应该做：
不应该做 为什么
判断客户钱包余额 钱包是平台财务账本，不是执行层职责
判断用户等级折扣 折扣属于计费引擎
判断 OEM 分账 分账属于租户商业模型
写客户 usage log usage log 是平台客户账本
决定租户能不能走共享池 这是 Route Policy 和租户绑定策略
暴露上游凭证给平台外部用户 凭证必须留在实例和平台内部
转发实例的关键字段也体现了这个边界：
字段能力 架构意义
instance_code 稳定实例标识
base_url 执行入口

字段能力 架构意义
auth token encrypted 实例访问凭证加密保存
deploy_region 支持区域化调度
status active / disabled / degraded
priority 多实例排序
max_concurrency 实例并发上限
health_score 实例健康过滤
pending setup 未完成配置的实例不能接流量
这里的核心原则是：
转发实例越纯，平台越容易替换上游适配层。
如果把钱包、折扣、租户、分账都写进转发实例，未来换转发实现时，等于要重写平台账
本。

## channel 才是平台的流量调度单元

channel 不是一个简单的 provider key。
channel 表达的是：
```
某个公开模型
在某个租户范围内
通过某个转发实例
使用某个转发路由别名
以某个权重、并发上限和健康状态
参与平台调度和计费归因
也就是说，转发实例是执行入口，channel 是平台侧的调度、隔离和归因单元。
关键字段包括：
channel 字段 用途
public_model_name 客户请求看到的模型名
transfer_model_name 转发实例内部路由别名
instance_id 绑定哪个转发实例
tenant_id 空表示共享 channel，非空表示租户专属 channel

channel 字段 用途
weight 负载均衡权重
priority 候选排序
max_concurrency 单 channel 并发硬上限
current_load 展示副本，实时路由以 Redis 为准
health_score 路由健康过滤
currency / cost 上游成本归因
metadata provider、能力、发布信息
为什么 channel 要承载成本和归因？
因为同一个公开模型可能背后有多个上游路线，成本不同、币种不同、稳定性不同、可用能
力不同。
如果只在转发实例里表达这些差异，平台侧就不知道本次请求到底走了哪条商业路径，也无
法准确计算毛利、对账和渠道质量。
所以平台账本里必须记录本次选择的 channel、转发实例、路由策略、上游模型名和成本快
照。

## 共享池、租户独享池和混合池

企业级平台不能只有一个“默认上游”。
不同客户会有不同诉求：
普通用户希望成本低、可用即可。
高级用户希望走更稳定的共享高质量池。
OEM 租户希望默认走自己的实例池。
大客户希望独享实例和专属 channel。
某个用户或等级需要灰度某条新上游路线。
当前可以用两层配置表达：
```
tenant_llm_bindings
tenant_id
instance_id
binding_type = dedicated / shared_pool / hybrid
priority
status
channel

tenant_id = NULL -> 平台共享 channel
tenant_id = 当前租户 -> 租户专属 channel
几类典型场景如下：
场景 转发实例 channel fallback
直营普通用户 共享转发实例 共享 channel 可在共享池内调度
平台高级用户 高质量共享实例或指定实例 高级 channel 按策略回退
OEM 普通客户 租户绑定实例 租户默认 channel 回到租户默认池
OEM 独享客户 独享转发实例 租户专属 channel 通常不允许共享兜底
单用户灰度 指定实例或指定 channel 灰度 channel 可以禁用 fallback
这里最重要的是最后一列。
共享池不能默认兜底一切。
如果一个 OEM 独享客户买的是独享能力，独享池故障时，系统不应该偷偷把请求打到共享
池。短期看成功率提高了，长期看成本、SLA、合规边界都会被破坏。
是否允许共享兜底，必须由策略显式表达。

### 路由不是 if/else，而是 RouteContext + Route Policy

如果只靠代码里写 if/else，很快就会失控。
比如：
某租户默认走实例 A。
某租户下的高级客户走实例 B。
某个用户灰度新上游 channel C。
图片请求走专门的多模态 channel。
chat 请求按共享池调度。
命中特定策略但目标不可用时直接失败。
这些规则不能写死在请求代码里。
我们把运行时路由上下文抽象成 RouteContext：
```
tenant_id
user_id
level_scope = platform_user_level / oem_customer_level / none
user_level_id

public_model_name
request_type
Route Policy 按这些维度匹配：
维度 能表达什么
tenant_id 租户级上游策略
user_id 单用户定向或灰度
level_scope + user_level_id 平台用户等级或 OEM 客户等级
model_code 指定模型
request_type chat / responses / image / audio 等请求类型
priority 多策略命中时的优先级
specificity 用户级策略优先于等级，等级优先于模型宽泛策略
策略目标有两种：
target_type 含义
channel 直接指定 channel 候选
instance 指定转发实例，再从实例下找匹配模型的 channel
fallback 也必须显式配置：
fallback_policy 含义
disabled 目标不可用直接失败
tenant_default 回到租户默认绑定池
shared_allowed 明确允许进入平台共享池
next_policy 继续匹配下一条策略
这个设计解决的是“灵活”背后的治理问题。
灵活不是随便兜底。
灵活是每个兜底都有策略记录、审计记录和可解释的目标范围。

## 默认路由：先租户绑定池，再共享池

没有命中 Route Policy 时，系统走默认路由。

默认路由不是简单随机挑一个上游，而是先构建候选池：
```
SelectChannel(tenant, public_model)
-> 查租户 active bindings
-> 按 binding priority 构建实例候选
-> 查实例下匹配 public_model 的 channels
-> 读取 Redis current_load
-> 过滤状态、健康、并发、pending setup
-> 若租户候选为空，再进入 shared pool
-> weighted least-load 选择 channel
-> Redis current_load++
-> 返回 SelectedChannel
-> 请求结束 ReleaseChannel
候选过滤条件如下：
 过滤条件                           | 数据来源           | 为什么              
 channel active                 | DB             | 禁用 channel 不能接流量 
 channel health_score >= 50     | DB / Worker 更新 | 低健康分从候选池剔除       
 current_load < max_concurrency | Redis          | 不能超过并发硬上限        
 instance active                | DB             | 禁用实例不能接流量        
 instance health_score >= 50    | DB / Worker 更新 | 实例不健康时整体剔除       
 not pending setup              | 实例配置状态         | 未配置凭证的实例不能接生产流量  
这里要注意：DB 里的 current_load 只是展示副本，路由时以 Redis 为准。
原因很直接。
高并发路由需要毫秒级的实时计数，DB 字段不适合承担这个职责。DB current_load 更适
合作为 Admin 面板展示和冷备快照，由 Worker 周期同步。

## 负载均衡：weighted least-load，不是简单轮询

渠道负载均衡不能只做轮询。
因为不同 channel 的质量、成本、限额和并发能力不一样。
一个 channel 权重高，表示它更适合承接流量。但如果它当前已经很忙，就应该降低它在本
轮选择里的有效权重。
当前算法是 weighted least-load：
```

availableSlots = max_concurrency - current_load
effective_weight = weight * availableSlots / max_concurrency
语义很清楚：
负载为 0 时，拿满配置权重。
负载越高，有效权重越低。
达到并发上限，直接从候选池剔除。
如果策略目标带 target_weight，会作为权重乘数叠加进去。
Redis 负载 key 是：
```
ai:channel:load:{channelID}
路由时会批量 MGET 候选 channel 的负载，选中后 INCR。
这里还有一个并发保护：
```
读到 current_load = 99
max_concurrency = 100
两个请求同时选中该 channel
两个请求都准备 INCR
如果没有 INCR 后检查，就可能突破上限。
所以实现里会在 INCR 后再看新值：
如果没有超过 max_concurrency，继续执行。
如果超过，立即 DECR，移除该候选，重选一次。
如果仍然没有可用候选，返回无可用 channel。
请求结束时必须 ReleaseChannel，把 Redis 计数减回去。
Release 使用不低于 0 的 Lua 脚本，避免重复释放导致负载变成负数。同时 key 有 TTL，
避免进程崩溃后计数永久残留。

## 健康监测：Worker 管状态，不塞进请求线程

渠道健康不能靠人工盯。
当前有两类健康治理。
第一类是 channel / instance 的运行窗口健康分。

当前执行器会把平台选中的成功调用写入 Redis 分钟级 bucket。窗口读取器本身按 ok /
total 聚合，后续如果要把更多失败样本纳入同一窗口，不需要改健康任务的状态机。
Worker 每 60 秒读取最近窗口，计算 health score，并把结果写回 channel 和实例记录。
简化后的模型是：
```
success_rate = ok / total
health_score = success_rate * 100
状态机是：
条件 行为
score < 30 首次出现 写 degraded 计时 key
score < 30 持续 3 分钟 系统自动降级
score > 70 首次出现 写 recovering 计时 key
score > 70 持续 5 分钟 系统自动恢复
Admin 手动禁用 不自动恢复
低分后无流量 可重置，避免低分无流量导致永远无法恢复
第二类是转发实例存活检查。
Worker 每 60 秒探测转发实例的 liveliness endpoint。连续失败达到阈值后，实例进入降
级；连续成功达到阈值后，降级实例恢复。
这类检查解决的是实例入口本身不可达的问题，和 channel 的业务流量窗口不同。
所以这里有两个层次：
健康对象 解决的问题
channel health score 某条模型路线在平台侧是否适合继续接流量
instance liveness 某个转发实例入口是否可达
两者都由 Worker 维护，而不是塞进 API 请求线程。
原因是健康检查本质上是周期性治理，不应该让用户请求承担扫描、降级、恢复、审计和告
警的成本。

## 同模型多上游怎么配置

同一个公开模型，可以对应多个 channel。

例如：
public model channel 转发实例 转发路由别名 作用
model-a channel-a-east instance-east route-a 上游 A 东区
model-a channel-a-west instance-west route-a 上游 A 西区
model-a channel-b-east instance-east route-b 上游 B 东区
model-a channel-b-west instance-west route-b 上游 B 西区
对客户来说，请求的都是 model-a 。
对平台来说，每个 channel 有自己的：
租户范围
权重
并发上限
健康分
成本价
上游模型名
转发实例
路由策略归因
这才是真正可运营的同模型多上游。
这里还有一个重要取舍：
平台层不会在一次请求失败后跨 channel 连环重试。
平台先选择一个 channel，本次请求的 usage、成本、健康、证据都归属这个 channel。
provider 级 retry 和 fallback 应该属于转发执行层的内部路线能力。否则平台层 A
channel 已经产生上游成本，再切到 B channel 成功，账本和证据归属会非常难解释。
简单说：
```
平台层：
选择一个 channel
记录本次请求归因
完成账本收口
转发执行层：
在选定路线内部处理 provider retry / fallback
这个边界看起来保守，但对账、毛利和争议处理会稳定很多。

## API / Worker 和渠道架构如何配合

把两条主线合起来，可以得到这张架构图：
```
Client / SDK

v
API 进程
鉴权 / 租户 / 预算 / 模型门禁
构建 RouteContext
调用 Route Policy + RoutingService

v
SelectedChannel
channel_id
instance_id
route_policy_id
cost snapshot

v
转发实例
执行上游协议转发
返回 response / usage / evidence

v
API 收口
usage log / charge line / budget commit

v
Worker
健康检查 / 实例探活 / 异步账单 / 对账 / 补偿
这张图里，每个对象都有自己的边界：
架构层 做什么 不做什么
API 进程 同步请求、门禁、预算、路由选择、响应收口 不做后台扫描和跨 channel 连环重
试
Worker 进 健康检查、实例探活、异步账单、对账、补偿 不阻塞用户请求
程
Route Polic 决定候选池、租户/用户/等级策略、fallback 不直接调用上游
y 语义
channel 平台侧调度、隔离、成本和归因 不保存上游凭证
转发实例 上游协议转发和执行层重试 不写客户账本
真正稳定的架构，不是每层都能做所有事，而是每层只做自己该做的事。

## 常见错误设计
### 第一种错误：业务还没稳定就拆微服务

结果是服务边界天天变，跨服务联调、数据一致性、配置发布先爆炸。

### 第二种错误：把 Worker 当成另一个服务随便写业务逻辑

结果是同步路径和异步路径各写一套规则。等需要补偿和对账时，没人能证明两套规则一
致。

### 第三种错误：把转发实例做成业务系统

一旦转发实例里写了钱包、租户、折扣、OEM 分账，平台账本和执行层就耦死了。未来想换
上游适配层，成本会非常高。

### 第四种错误：channel 只是 provider key 的别名

如果 channel 不能表达租户范围、权重、并发、健康、成本和归因，它就不是调度单元，只
是配置项。

### 第五种错误：独享池不可用时自动回共享池

短期成功率变高，长期破坏成本、SLA 和合规边界。能不能回共享池，必须由策略显式决
定。

### 第六种错误：用 DB current_load 做实时路由

高并发路由需要实时原子计数。DB 字段适合展示，不适合承担毫秒级调度。

### 第七种错误：平台层 A 主 B 备跨 channel 重试

一旦 A 已经产生上游成本，再切到 B 成功，账本、usage、provider evidence 会变得很难
对齐。

## 架构检查清单

如果你也在设计 AI Token 聚合平台，可以用这张清单自检。

### API / Worker：

是否有统一应用入口，并通过运行角色区分 API 和 Worker。
API 是否只承接同步请求和必要强一致操作。
Worker 是否承接定时、异步、补偿、对账、健康检查。
模块之间是否通过 service interface 调用。
是否避免跨模块直接访问 repository。
Worker 任务是否有业务唯一键和幂等保护。
队列和事件是否有抽象接口，未来可替换基础设施。

哪些模块未来可以拆服务，边界是否已经能描述。

### 微服务拆分：

是否已经有独立团队负责独立业务域。
是否需要独立部署、独立扩缩容。
是否能接受事件最终一致性。
是否已经有服务治理、链路追踪、告警和灰度能力。
数据归属是否清楚。
失败补偿和幂等机制是否成熟。

### LLM 渠道：

转发实例是否保持纯执行层职责。
channel 是否承担平台调度和归因。
同一个 public model 是否可以配置多个 channel。
channel 是否可以绑定不同转发实例和区域。
是否支持共享池、租户独享池和混合池。
是否有 Route Policy 表达租户、用户、等级、模型、请求类型。
fallback_policy 是否显式控制共享池兜底。
负载均衡是否使用实时 current_load。
health_score 是否由 Worker 根据运行窗口更新。
Admin 手动禁用是否不会被自动恢复覆盖。

## 结尾

这篇文章的核心判断是：
AI Token 聚合平台当前更适合用 API + Worker 的模块化单体承载业务复杂度，同时把
LLM 渠道高可用独立建模为转发实例、channel、共享池、独享池、Route Policy、负载和
健康治理。
微服务不是架构起点。
微服务是组织、业务和基础设施发展到某个阶段后的结果。
在那之前，更重要的是把边界先做对：代码边界、运行角色边界、异步边界、账本边界、渠
道边界。

AI聚合网关之三:N喜e欢wA作PI者、LiteLLM 和自研原生
Adapter 的边界
链路
AI聚合平台之五：设计一个企A业I级聚 合To平ke台n之 聚五：设计一AI个聚企合业平级台 之To七ke: n如 聚何合让平Token中转平台的每一
台OEM体系
合平台OEM体系 条收费都经得起对账考验
高可用设计
```

---


## 第7篇：对账系统：如何让Token中转平台的每一条收费都经得起对账考验

采购怕多付、产品怕说不清、技术怕查不到，token中转平台对账怎么做?
有一次我们做用量核对，看到一个很容易误判的结果。
平台成功请求 289,600 条，失败请求 2,000 条。
成功请求里，有 1,300 条平台已经成功落账，但转发层没有对应证据。
还有 17,700 条请求虽然总量对齐，但 reasoning 明细差了 150,000。
失败请求里，2,000 条都没有产生 token 和扣费。
如果只看汇总，这一天像是“平台多记了 71,356,500 tokens”。
但逐请求排查后发现，主要问题不是平台多扣费，而是转发层证据漏写、上游 504、平台先
超时但转发层后来成功计费，以及明细维度口径差异。
这就是 Token 聚合平台对账最容易被低估的地方。
它不是把“平台用量”和“上游账单”简单相减。
它要回答的是四个问题：

 问题       | 典型数据               |     | 错误处理会怎样       
 客户账单自己是否 | 1,300 条平台成功但转发层证据缺 |     | 把证据缺失误判成平台多扣费 
成立 失
 请求证据是否完整 | 71,356,500 token 汇总差异 |     | 只看总量，看不到差异类型        
 上游成本是否真实 | 1,600 条平台失败但转发层已成功    |     | 客户失败、平台收入为 0、上游成本已经 
 发生       | 计费                    |     | 发生                  
 供应商月账是否能 | 发票和平台成本可能跨口径偏差        |     | 直接改客户钱包，制造资金事故      
解释
AI Token 聚合平台的对账能力，不能只做“平台总量减上游总量”。它必须能回答：客户账是
否成立、请求证据是否完整、跨协议转换后 usage 是否归一、上游成本是否可信、供应商发
票是否能解释。

## 对账难在哪里

Token 聚合平台的对账难，不是因为表多。
真正的难点是：同一笔请求，在不同系统里代表不同事实。
角色 /
     | 它关心什么 | 数据例子 |     | 它不能代表什么 
系统
 客户   | 请求是否成功、账单扣了多少   | 模型 A 客户扣费约 466.49 |     | 不代表上游一定有完整 
      | 钱               | 元                 |     | 日志         
 平台账本 | usage、钱包、额度、折扣是 | 单日 289,600 条成功请求需 |     | 不直接等于供应商发票 
      | 否一致             | 要可下钻              
 转发层证 | 请求有没有打到上游、上游返   | 1,300 条平台成功但转发层   |     | 不直接等于客户最终账 
 据    | 回了什么            | 证据缺失              |     | 单          
 供应商账 | 官方成本和月度应付金额     | 按供应商、模型、日期汇总      |     | 不解释每个客户为什么 
 单    |                 |                   |     | 扣这些钱       
这四类事实不能互相替代。
接口成功，不等于所有证据完整。模型 A 的 1,300 条请求就是这样：平台已经成功落账，但
转发层没有写出对应证据。
上游有记录，也不等于客户就该被补扣。模型 D 的 1,600 条请求是平台失败、转发层后来成
功计费，这种情况要进入财务复核，而不是后台任务直接补扣客户钱包。
总 token 对齐，也不等于所有计费维度都对齐。模型 C 的 17,700 条成功请求里，total
tokens 完全一致，但 reasoning 明细差了 150,000。

请求失败，也不能默认没有成本。上游 504 的 2,000 条失败请求，平台和转发层 token 与
成本记录都是 0，这才可以归类为可解释失败。
所以对账第一步不是“谁多谁少”，而是先把差异类型分清楚。

## 上游账单不能直接当客户账单

很多平台的第一版对账很容易这么做：
```
客户账单 = 上游成本记录
这个做法在业务小的时候看起来很省事。
但只要上游证据出现漏写、延迟、重复、跨协议字段差异，客户账单就会被上游系统牵着
走。
 错误做法      | 数据证据                 | 会出什么问题         
 直接用转发层成本记 | 模型 A 有 1,300 条成本证据缺失 | 会把证据漏写误判成平台多扣费 
录做客户账单
 只按天汇总对账   | 汇总差异 71,356,500 tokens，但原因   | 看不到平台独有、上游独有、tok 
           | 至少 4 类                       | en 明细差异          
 失败请求默认不查  | 上游 504 有 2,000 条，另有平台失败      | 会漏掉“客户失败但上游已计费”的 
           | 但上游成功计费 1,600 条              | 成本风险             
 发现差异自动改钱包 | 模型 C 只是 150,000 reasoning 明细 | 会把报表口径问题变成资金事故   
差异
只看 total tokens 模型 C total tokens 对齐，但 reasoni cache、reasoning、image、au
     | ng 维度不一致 | dio 等维度解释不了 
对账系统首先是证据系统，其次才是资金修正系统。
证据系统负责发现、归类、标记、下钻、留痕。
资金修正必须走审核、调整单、审计记录。
这条边界如果不守住，对账任务就会从“发现风险”变成“制造资金风险”。

## 跨协议转换后，账怎么对

还有一个问题，经常被忽略。
下游客户可能是用 OpenAI-compatible 访问平台：
```

POST /v1/chat/completions
model = 模型 C
但平台内部可能会这样执行：
```
入口事实：openai_chat
公开模型：模型 C
路由结果：转发到上游供应商 X
上游协议：anthropic_messages
上游模型：供应商侧真实模型名
也就是说，下游看见的是 OpenAI-compatible 入口；平台和上游之间，可能是
Anthropic-compatible 或其他供应商协议。
这不是问题。
真正的问题是：平台有没有把转换前后的事实都保存下来。
如果只保存 request_type=chat ，只能说明客户从 OpenAI Chat 入口进来，不能说明上游
一定是 OpenAI。
所以我们会同时保存两类事实。
事实
记录什么 给谁看 解决什么问题
类型
入 口 客户通过 OpenAI Chat、Responses、Imag 客户侧可以看到入口 解释客户为什么这
事实 es 还是 Anthropic Messages 进入 协议和公开模型 么调用
上 游 实际供应商、上游模型、转发模型名、渠道、 只给运营和财务后台 解释平台为什么产
事实 实例 / 对账使用 生这个上游成本
客户账单按平台公开模型和平台价格解释。
上游成本按上游供应商和上游模型解释。
这两条账必须通过同一个请求身份、同一个 usage log 和同一组计费明细连接起来。

## Usage 必须先归一化

跨协议时，不同上游的 usage 字段并不等价。
OpenAI-compatible 常见字段是：
```
prompt_tokens
completion_tokens
total_tokens

Anthropic-compatible 可能返回的是：
```
input_tokens
output_tokens
cache_read_input_tokens
cache_creation_input_tokens
如果平台把上游字段原样丢进客户账单，后面一定会出问题。
比 如 Anthropic-compatible 的 输 入 token ， 可 能 不 包 含 cache read 和 cache
creation。平台如果不做归一化，就会出现普通 input、cache read、cache write 之间的
拆分错误。
更稳妥的方式是：
```
平台客户账本维度：
input / output / cache read / cache write / reasoning / image / audio ...
上游原始证据：
保留供应商原始 usage 字段，用于对账和争议处理
对文本模型来说，最关键的是先把输入、输出和 cache 写清楚：
```
total_tokens = prompt_tokens + completion_tokens
prompt_tokens = uncached_input_tokens
+ cached_prompt_tokens
+ cache_creation_input_tokens
cache_creation_input_tokens = cache_creation_5m_input_tokens
+ cache_creation_1h_input_tokens
+ 可能存在的未拆 TTL cache write
这里的 cached_prompt_tokens 可以理解为 cache read。
cache_creation_input_tokens 是 cache write，进一步拆成 5 分钟、1 小时，以及供应商
只返回聚合值时暂时无法拆 TTL 的 cache write。
这个公式的价值不是为了让账单看起来更复杂，而是为了避免把三类输入混在一起：
输入类型 账单含义 对账风险
uncached input 本次真正新输入的 token 和普通 input 单价绑定
cached input read 命中缓存的输入 token 价格通常不同，不能当普通 input

 输入类型                 | 账单含义          | 对账风险                     
 cache creation input | 本次写入缓存的 token | 5 分钟、1 小时和未知 TTL 可能是不同价格 
如果这三类输入不先拆开，OpenAI-compatible 入口转到 Anthropic-compatible 上游
时，就很容易出现“总 token 看起来对齐，但计费维度解释不清”的问题。
模型 C 的数据说明了这个问题。
 指标               | 平台侧           | 转发层           | 差异       
 成功请求             | 17,700        | 17,700        | 0        
 total tokens     | 1,499,215,500 | 1,499,215,500 | 0        
 cache tokens     | 1,356,235,400 | 1,356,235,400 | 0        
 reasoning tokens | 0             | 150,000       | -150,000 
 平台上游成本           | 约 7,842.06    | 约 7,841.78    | 约 0.27   
这组数据说明三件事。
第一，请求数完全对齐。
第二，total tokens 和 cache tokens 对齐，说明主计费口径没有漂。
第三，reasoning 明细不一致，但它没有改变 total tokens，也没有改变客户总扣费。
所以这不是资金事故。
它是计费明细口径缺失，应该补维度映射和报表说明，而不是改客户钱包。

## 四层对账模型

在这个前提下，四层对账不是一个技术名词。
它是一条证据链。
客户问“我这条请求为什么扣了钱”，平台不能只回答“上游账单里有这笔”。
平台至少要能把这条请求从客户入口查到上游账单。
 层级 这层看什么     | 能回答什么问题       | 典型字段                   
 L0  客 下游客户访问 | 客户这条请求在平台侧发生了 | 客户请求 ID、租户、用户标识、API Ke 
 户请求 平台的每一条   | 什么、为什么扣费或为什么失 | y 标识、公开模型、状态、错误、usag   
 账本 请求        | 败             | e、客户价、钱包/额度消耗          

 层级 这层看什么         | 能回答什么问题       | 典型字段                     
 L1  上 转发实例访问     | 平台这条请求到底转发到了哪 | 客户请求 ID、上游请求 ID、channel、 
 游转发 上游的每一条       | 个上游、哪个模型、是否拿到 | 转发实例、上游模型、tokens、上游返     
 证据 请求日志          | 了 usage 或错误   | 回状态、错误日志                 
 L2  平 L0 和 L1 的逐 | 客户账本和转发证据是否能互 | 成功但证据缺失、失败但上游成功、tok      
 台内部 请求核对         | 相解释           | ens 差异、成本差异、证据完整性        
对账
 L3  上 L1 和上游官 | 转发实例记录的上游消耗，能 | 供应商、上游模型、上游请求 ID、日 
 游账单 方账单的核对    | 否被供应商账单解释     | 期、计费项、币种、官方金额、发票金  
 对账            |               | 额                  
这张表可以作为对账系统的产品检查表。
每一层都回答一个不同问题，也对应不同责任。

### L0：客户请求账本

L0 是下游客户访问平台的明细账。
它的粒度不是“某天某个模型用了多少 token”，而是每一条请求。
客户给平台传入一个请求，平台会生成或接收一个客户请求 ID。后续所有客户侧可见的状
态，都要围绕这个 ID 展开。
一条 L0 记录至少要能反查这些信息：
这是谁的请求：租户、用户标识、API Key 标识。
请求了什么：公开模型、接口类型、是否流式、多模态类型。
结果是什么：成功、失败、超时、取消、错误码、错误摘要。
用量是多少：input、output、cache、reasoning、total tokens。
钱怎么算：客户价、折扣后金额、额度消耗、钱包实扣、预算预留和释放。
证据在哪里：请求日志、响应摘要、错误日志、计费明细、钱包流水、额度流水。
这层解决的是客户账单解释问题。
客户问一条请求为什么扣费，平台首先要能按客户请求 ID 查出：
请求是否成功。
usage 是怎么得到的。
扣了多少额度。
扣了多少钱包。
哪些计费明细组成了最终金额。
如果 L0 都解释不清，后面拿再多上游账单也没用。

因为客户买的是平台服务，不是直接拿供应商发票报销。

### L1：上游转发证据

L1 是转发实例访问上游的执行日志。
这层不是客户账本。
它回答的是另一件事：
平台这条客户请求，到底有没有真的打到上游？
所以一条 L1 记录必须同时保留两个 ID：
客户请求 ID：用于关联 L0。
上游请求 ID：用于关联供应商账单、上游后台、错误排查。
这两个 ID 一起出现，才叫可追溯。
只保留客户请求 ID，查不到供应商侧证据。
只保留上游请求 ID，又解释不了客户侧是哪一笔账。
一条 L1 记录至少要能反查这些信息：
走了哪个 channel。
走了哪个转发实例。
public model 被映射成了哪个上游模型。
上游返回了哪些 usage 字段。
上游是否返回错误、超时、限流、内容拦截。
如果失败，失败发生在建连、首包、流式中途，还是最终解析。
如果成功，上游请求 ID 是什么，tokens 和成本证据是什么。
在那次单日核对里，平台成功请求 289,600 条，转发层可匹配成功证据少 1,300 条。
继续下钻后发现，这 1,300 条不是客户请求 ID 丢了，而是转发层流式日志处理失败，导致
成功请求没有写入完整的上游执行证据。
这类问题在 L1 里应该被归类为：
```
平台成功，转发层证据缺失
它要生成待复核差异，运营后台可以看到，后续修复转发层日志写入或路由策略。
但它不应该触发自动退款。
因为 L1 缺失说明证据链有洞，不等于 L0 客户账本一定错。

### L2：L0 和 L1 的内部对账

L2 是平台内部对账。
它把客户请求账本和上游转发证据按客户请求 ID 拉到一起，逐条判断能不能解释。
这里会出现四类高频问题。
第一类：L0 成功，L1 缺证据。
模型 A 就是这个问题。平台侧 1,600 条成功请求里，有 1,300 条转发层证据缺失。这部分
客户侧扣费约 466.49 元，上游成本约 344.14 元。
处理方式不是自动退款，而是确认 L0 usage、计费明细、钱包和额度是否自洽，再补 L1 证
据链。
第二类：L0 失败，L1 也失败。
模型 B 里有 2,000 条失败请求，平台侧没有 token、没有客户扣费，转发层也没有成功成
本。这类请求可以解释为上游不可用或超时失败，不进入客户收费。
第三类：L0 失败，L1 后来成功。
模型 D 里，平台侧失败记录有 1,600 条，但转发层后来出现成功成本记录，对应
61,394,100 tokens 和约 687.70 元客户金额估算。
这类问题最危险。
客户看到的是失败，没有被扣费；供应商侧可能已经发生消耗。
它不能静默补扣客户钱包，也不能忽略上游成本，而是要进入人工复核、超时策略修正和转
发执行边界修正。
第四类：L0 和 L1 总量一致，但明细字段不一致。
模型 C 里，17,700 条成功请求、1,499,215,500 total tokens、1,356,235,400 cache
tokens 都对齐，但 reasoning tokens 差了 150,000。
这说明主账没有漂，客户扣费不需要调整；但明细维度需要补映射、补报表说明。
L2 的价值就在这里。
它不是拿上游日志覆盖客户账。
它是把每一条客户请求拆成可以解释的差异类型。

### L3：L1 和上游账单的内部对账

L3 不是直接对客户账本。
它对的是转发实例日志和供应商官方账单。

这层通常会按这些维度聚合：
供应商。
上游模型。
上游请求 ID。
项目或 workspace。
计费项。
币种。
日期。
模型 C 里，平台上游成本约 7,842.06，转发层原始成本记录归一到同一币种后约
7,841.78，差异约 0.27。
这个差异不应该按单请求改客户钱包。
它更应该在官方成本、汇率、四舍五入、成本导入窗口里解释。
月结发票也是 L3 的一部分。
如果发票比平台上游成本估算多出一笔，处理顺序应该是：
1. 先按供应商、上游模型、日期、计费项定位。
2. 再看官方成本导入是否完整。
3. 再看转发层证据是否有缺失、重复或延迟。
4. 最后看平台客户账本是否需要人工调整。
模型 D 这种“平台失败但转发层成功计费”的类型，首先是 L2 差异；如果供应商账单也确认
了这笔消耗，它还会进入 L3。
它涉及 61,394,100 tokens 和约 687.70 元客户金额估算。
平台侧看是失败，没有成功账单；上游侧看可能已经产生成本。
这不是后台任务可以静默处理的问题。
它必须进入财务复核。
到这里，L0 到 L3 的职责才完整：
```
下游客户请求
-> L0：平台客户请求账本
-> L1：转发实例上游执行证据
-> L2：L0 和 L1 的逐请求内部对账
-> L3：L1 和上游官方账单对账
只有这条链路完整，平台才能对下游客户解释：

这条请求是谁发的，走了哪个模型，打到了哪个上游，产生了多少 usage，为什么成功或失
败，为什么扣费或不扣费，上游账单里对应哪一笔。
这也是为什么 NewAPI 这类通用中转面板不适合作为企业级聚合平台的核心账本控制面。
NewAPI 适合快速搭建多模型网关，做基础的渠道、模型、额度、日志和充值管理。
但企业级聚合平台面对的不是“能不能转发”。
它面对的是“每一条客户请求能不能被解释，每一笔扣费能不能被证明，每一笔上游成本能不
能被归因”。
如果计费系统只围绕通用额度和请求日志展开，很容易停在 L0 或 L1。
客户账本、转发证据、内部对账、上游账单对账没有分开建模，就会出现三个问题：
客户请求 ID 和上游请求 ID 没有稳定关联，出问题只能靠日志碰运气。
客户扣费和上游成本混在一起，无法清楚区分客户争议、平台毛利、供应商账单问题。
对账任务缺少边界，容易把上游日志当成客户钱包调整依据。
所以 NewAPI 可以做轻量阶段的工具，也可以做通用网关能力参考。
但当平台要做企业客户、OEM、财务审计、月结发票和争议处理时，核心账本必须掌握在平
台自己手里。

## 案例一：平台成功，但转发层证据缺失

先看模型 A。
指标 数值
平台侧模型 A 成功请求 1,600
转发层可找到成本证据 300
平台成功但转发层证据缺失 1,300
缺失证据对应 total tokens 71,353,900
客户侧扣费 约 466.49 元
平台上游成本估算 约 344.14 元
这类数据第一眼很吓人。
平台成功 1,600 条，转发层只找到 300 条证据。直觉上很容易判断为“平台是不是多扣了？”
但逐请求排查后，结论不是这样。
排查路径是：

1. 先按请求身份逐条比对，确认不是匹配字段丢失。
2. 再看平台账本，确认请求状态成功、usage 来源正常。
3. 再看转发层日志，发现流式日志聚合失败。
4. 最后分类为：平台成功，转发层证据漏写。
这类问题不能简单退款，也不能用转发层缺失反推平台账本错误。
正确动作是：
保留平台账本。
标记转发层证据缺失。
生成复核项。
修复转发层日志写入或相关路由策略。
证据缺失要修证据链，不要直接动客户钱包。

## 案例二：上游 504，平台失败但没有扣费

再看模型 B。
指标 数值
平台失败请求 2,000
明确 stream headers timeout 1,900
其他 504 100
平均失败耗时 约 96 秒
平台侧 token / 成本记录 0
转发层 token / 成本记录 0
这类问题看起来像事故，但它不一定是账务事故。
关键要核对三件事：
1. 客户是否拿到了失败响应。
2. 平台是否没有扣费。
3. 上游是否也没有产生费用。
这批请求里，平台失败，平台 token 为 0，平台没有扣费；转发层失败记录里 token 和成本
记录也是 0。
这说明它是可解释失败。
后续要做的是渠道健康、超时策略、上游稳定性排查，而不是账务补偿。

如果只看“失败 2,000 条”，产品和运营会紧张。
但如果同时看到“token 和成本记录均为 0”，就知道这批失败没有变成资金风险。

## 案例三：平台失败，但转发层已经成功计费

真正危险的是模型 D 这种。
指标 数值
平台失败记录 1,600
平台计费明细 0
平台 token / 客户成本 / 上游成本 0
转发层成功成本记录 1,600
转发层 total tokens 61,394,100
粗略客户金额估算 约 687.70 元
这类请求的平台表现是失败。
客户没有成功账单，平台也没有收入。
但转发层后续写入成功成本记录，说明上游最终完成并产生了用量。
这类问题通常出现在长耗时请求、流式尾部、平台超时边界、上游继续执行等场景里。
它和模型 B 完全不同。
模型 B 是失败且无成本。
模型 D 是平台失败但上游可能已经产生成本。
所以它不能被简单归类为“失败请求，无需处理”。
正确动作应该是：
生成差异复核项。
标记为“平台失败但上游已计费”。
进入运营和财务复核。
结合客户是否收到有效内容、平台是否有响应证据、上游成本是否确认，决定是否调整。
这类问题才是真正的成本风险。
如果没有 L1/L2/L3 对账，平台很可能每天都在漏掉这类上游成本。

## 案例四：总量一致，但明细维度不一致

模型 C 是另一类容易误判的问题。
 指标               | 平台侧           | 转发层           | 差异       
 成功请求             | 17,700        | 17,700        | 0        
 total tokens     | 1,499,215,500 | 1,499,215,500 | 0        
 cache tokens     | 1,356,235,400 | 1,356,235,400 | 0        
 reasoning tokens | 0             | 150,000       | -150,000 
 平台上游成本           | 约 7,842.06    | 约 7,841.78    | 约 0.27   
成功请求数完全一致。
total tokens 完全一致。
cache tokens 完全一致。
成本差异只有约 0.27。
但 reasoning 明细少了 150,000。
这不是客户总扣费问题。
它是报表维度问题。
如果客户账单只展示总费用和总 token，这批数据没有明显风险。
但如果运营后台要按 reasoning 维度分析模型成本，或者后续要对 reasoning 做独立价格
策略，这个字段就必须补齐。
所以这类差异的处理方式是：
不动客户钱包。
补 usage 归一化字段。
补维度映射。
在旧数据或历史记录上做兼容标记。
对账的价值就在这里：它不只是发现“钱错了”，也发现“解释链不完整”。

## 修正不能止于接口成功

对账发现问题后，修正也不能只看接口能不能返回 200。
一次新模型能力兼容性修正，至少要验证六类调用形态：
 验证项      |     | 为什么要测     
 baseline |     | 最基础请求是否正常 

验证项 为什么要测
reasoning 参数 推理参数是否被保留和正确转发
tools 工具调用是否触发协议转换异常
tools + reasoning 最容易触发兼容性问题
stream + usage 流式场景能否拿到最终 usage
stream + tools + reasoning + usage 组合场景是否仍能落账
只测 baseline 没有意义。
因为很多对账问题恰恰发生在组合场景里。
一个看起来很小的参数策略，可能导致平台走错上游协议；一个流式请求，可能接口层已经
返回，但最终 usage 没有进入平台账本；一个转发实例配置，可能单节点正常，双节点流量
一分摊就出现差异。
所以修正闭环应该是：
1. 先确认上游本身支持目标能力。
2. 再确认当前转发配置为什么触发错误路径。
3. 单节点调整。
4. 验证普通请求、工具调用、推理参数、流式 usage。
5. 再扩到第二个转发实例。
6. 最后从平台入口发起 smoke。
7. 回查 usage log，确认账本成功落地。
对账问题的修正闭环不是“接口成功”，而是“接口成功、账本成功、证据成功、后续可对
账”。

## 对账任务不能直接改钱包

这是整篇最重要的产品原则。
对账任务只能发现差异，不能直接处理资金。
原因很简单：不是所有差异都代表客户账单错误。
原因 数据证据 正确处理
有些差异是证据缺失，不是客 模型 A 1,300 条转发层证据缺失，但 生成复核项，修复证据链
户账单错误 平台账本已成功
有些差异是上游维度口径不 模型 C reasoning 差 150,000，但 to 补维度映射和报表说明
同，不影响扣费 tal tokens 对齐

 原因            |     | 数据证据         | 正确处理        
 有些差异是供应商月结偏差， |     | 发票差异只在月结窗口确认 | 财务复核，不自动改钱包 
需要财务确认
 有些差异是真实成本风险 |     | 模型 D 平台失败但转发层成功计费 1, | 告警、人工审核、必要时 
             |     | 600 条                | 走调整流程       
如果对账任务直接改钱包，它会遇到几个无法回答的问题：
模型 A 的 1,300 条证据缺失，是不是应该退款？
模型 C 的 reasoning 明细差异，要不要补扣？
模型 D 的上游成本已经发生，客户没有成功账单，要不要补扣？
供应商发票多了一笔，应该摊给哪个客户？
这些问题都不是后台任务能自动判断的。
后台任务可以发现差异。
资金动作必须进入复核流程。

## 产品经理应该关心哪些对账能力

如果你是产品经理，评估一个 Token 聚合平台的对账能力，不要只问“有没有对账报表”。
要问这些更具体的问题：
 能力   | 验收问题                                 |     | 为什么重要  
 日级汇总 | 能不能看到 289,600 成功、2,000 失败、71,356,500 |     | 先判断影响面 
token 差异
 请求级下 | 能不能把 1,300 条证据缺失请求逐条定位 |     | 避免汇总误判 
钻
 差异分类 | 能不能区分证据缺失、token 明细差异、平台失败但上 |     | 决定是否影响资金 
游成功、上游失败无成本
 跨协议快 | 能不能同时看到 openai_chat 入口和真实上游供应商 |     | 解释 OpenAI 入口转 Anthro 
 照    | / 模型                           |     | pic / 其他上游           
 客户账本 | 能不能只展示客户价、客户计费明细、钱包 / 额度扣减     |     | 避免暴露上游成本             
 运营对账 | 能不能看到转发层证据、官方成本、发票差异           |     | 支撑运营和财务排查            
证据
 复核流程 | 能不能分配、备注、调整、豁免、关闭、重开 |     | 差异处理可追踪    
 审计记录 | 每次人工动作是否有操作人、原因、调整引用 |     | 防止资金修正不可追溯 
这些能力看起来不像“核心调用链路”。

但它们决定平台能不能从接口中转，变成可收费、可对账、可运营的商业系统。

## 最后

Token 聚合平台的对账能力，真正决定它能不能从“接口中转”变成“可收费业务”。
这套设计的关键不是多建几张表，而是把四类事实保留下来：
下游怎么调用平台。
平台怎么给客户落账。
平台怎么转到上游。
供应商最后怎么计费。
OpenAI-compatible 只是客户入口协议，不代表上游一定是 OpenAI，也不代表上游字段
可以直接成为客户账单。
平台必须把入口事实、上游事实、归一化 usage、客户账本和供应商证据分开保存，再用请
求身份把它们串起来。
这样，客户问“为什么扣这笔钱”，平台能解释。
运营问“为什么平台和转发层不一致”，平台能下钻。
财务问“为什么供应商发票多了一笔”，平台能追到模型、渠道、日期和证据。
一切皆有答案. Adapter 的边界
链路
台OEM体系
高可用设计
AI聚合平台之六：Token转发后端架构与LLM AI聚合平台之八：别只卷模型单价，API Key
渠道高可用设计 权限、用量控制和调用日志分析才是企业级
```

---


## 第8篇：控制台：API Key 权限、用量控制与调用日志分析

从 API Key 的模型白名单、调用白名单、限额策略和日志联动讲起，再到模型广场、价
格详情、Playground、账单统计和安全治理。
很多 token 中转平台控制台的第一版，通常只有三件事：
```
创建 API Key
查看余额
复制接口地址
这能让个人用户调通接口。
但企业客户接入时，马上会继续追问：
```
这把 Key 能用哪些模型？
最多能花多少钱？
只能从哪些来源调用？
出问题后能不能按 Key 查日志和账单？
所以企业级 token 控制台的核心，不是把功能菜单堆满。
真正值钱的是把 API Key 做成最小治理单元。
一把企业级 API Key 至少要同时承担 6 个边界：
 API Key 边界 | 要回答的问题                    
 身份边界       | 这把 Key 属于哪个项目、环境或团队       
 模型边界       | 这把 Key 能调用哪些模型            
 来源边界       | 这把 Key 能从哪些 IP / CIDR 调用  
 预算边界       | 这把 Key 最多能花多少钱            
 状态边界       | 这把 Key 当前能不能调用，为什么不能调用    
 证据边界       | 这把 Key 的请求、用量、费用和错误能不能查回来 
如果控制台只支持“创建、删除、复制 Key”，它只是一个密钥仓库。

如果它能同时管模型、来源、预算、状态和证据，它才是企业客户能用来上线的治理面。
这篇文章就按这个逻辑拆一遍：企业级 token 中转平台的控制台，应该怎么从 API Key 管理
延伸到模型选型、调用日志、用量统计、钱包账单和上线前验证。

## API Key 管理的 6 个边界

先给一张可以直接拿去评审的表。
边界 页面能力 企业价值
身份 Key 名称、脱敏值、最后使用时间 区分项目、环境和团队
模型 空白名单允许全部；非空白名单只允许指定模型 避免测试 Key 打到高成本或生产模型
来源 全局白名单 + 单 Key 覆盖，IP / CIDR 一行一条 Key 泄露后降低外部滥用风险
预算 累计、周、月限额；超限停止或警告继续 控制项目级和团队级成本
状态 激活、停用、撤销、超限 降低误停生产流量和遗留 Key 风险
证据 调用日志、用量统计、账单明细按 Key 追踪 客户能自助排障和解释成本
这张表背后的判断很简单：
API Key 不是鉴权字符串，而是企业级 token 控制台里的最小运营和治理单元。
企业客户通常不会只创建一把 Key。
更常见的是这样拆：
Key 用途 管理策略
生产环境 严格模型白名单、固定出口 IP、硬限额
测试环境 较低周限额、允许更多验证模型
团队项目 单独预算、单独统计、单独排障
外部客户 独立停用、独立账单解释、独立来源限制
这也是为什么控制台不能按“菜单完整度”设计，而要按“企业如何接入、上线、控费、排障和
治理”设计。

## 创建 Key：明文只展示一次，边界要一起配置

Key 创建的第一个原则，是完整密钥不能长期留在控制台里。
当前设计应该是：
```

创建时展示完整 Key
-> 用户确认已经保存
-> 关闭后不再展示明文
-> 列表只展示脱敏值和前缀
这个交互解决两个问题。
第一，控制台不能变成长期明文密钥库。
第二，排查问题时，技术同事需要通过前缀或展示值识别 Key，但不应该看到完整密钥。
所以创建完成后，要明确提示用户把 Key 保存到：
密码管理器。
环境变量。
服务器安全配置。
企业内部密钥管理系统。
但这还不够。
企业控制台真正要避免的是“先发一把裸 Key，再回头补策略”。
因此创建 Key 时，就应该同步配置这些边界：
创建项 作用
累计消费上限 限制这把 Key 生命周期内最多花多少钱
每周消费上限 防止短周期异常流量打穿预算
每月消费上限 对齐企业预算和结算周期
超限策略 决定超限后停止请求还是只记录告警
模型白名单 控制这把 Key 能调用哪些模型
来源白名单 控制这把 Key 能从哪些 IP / CIDR 调用
这样创建出来的不是一把裸 Key。
它一开始就带着预算边界、模型边界和来源边界。
这对企业客户非常关键。
很多线上事故不是因为“不会创建 Key”，而是因为“创建完 Key 以后忘了补策略”。

## Key 状态：激活、停用、撤销、超限必须分清

企业客户不会只关心“有没有这把 Key”。
他们真正关心的是：

```
这把 Key 当前能不能调用？
为什么不能调用？
是不是额度打满了？
关闭它会不会影响线上服务？
所以 Key 列表不能只显示一个开关。
至少要区分 4 类状态：
状态 含义 常见处理
激活 可以正常调用 继续观察用量和最后使用时间
已停用 人工停止调用 排查后可以重新启用
已撤销 Key 已删除或废弃 不再恢复，应该更换新 Key
已超限 命中消费上限 调整额度或等待周期重置
状态筛选也很重要。
当一个团队反馈接口突然不可用，支持人员应该先能看出这把 Key 是否已停用或已超限，而
不是直接去查上游模型。
最后使用时间同样不是装饰字段。
它能回答：
这把 Key 是否仍在生产服务中使用。
某个旧项目 Key 是否已经沉默。
关闭 Key 前最近是否还有调用。
最近 7 天哪些 Key 仍然活跃。
对企业客户来说，Key 生命周期管理不是“删掉不用的 Key”。
它真正降低的是误停生产流量、长期遗留密钥和异常调用无人发现的风险。

## 模型白名单：不要让测试 Key 打到生产模型

很多平台只在账号层面控制模型权限。
这对企业客户不够。
企业内部通常会按项目、环境、团队、客户拆 Key。
如果所有 Key 都能调用所有模型，就会出现很现实的问题：
```

测试 Key 调用了高成本模型
临时脚本打到了生产模型
外包项目拿到了不该用的多模态能力
某个团队绕过采购策略直接调用新模型
所以模型权限必须下沉到 Key。
Key 级模型白名单要明确两个状态：
```
空白名单：允许全部模型
非空白名单：只允许选择的模型
这里最容易被误解的是“空白名单”。
空白名单不是“没有配置”，而是“允许全部模型”。
所以页面上必须把这个状态讲清楚。
非空白名单则要展示已限制几个模型，并能进入编辑。
这个能力的价值不在于“多了一个选择框”。
它真正解决的是模型开放策略：
场景 模型白名单策略
生产对话服务 只开放稳定对话模型
图像项目 只开放图片生成和图片编辑模型
语音项目 只开放转写和文本转语音模型
测试项目 开放更多验证模型，但配低额度
高成本模型 只给指定项目 Key 开放
模型白名单还要和 Playground 联动。
当用户在 Playground 选择一个模型时，控制台应该只展示与这个模型兼容的 Key。
如果没有任何兼容 Key，页面应该提示用户回到 Key 管理页调整模型白名单。
不要让用户等到请求失败才知道权限不匹配。

## 来源白名单：全局默认 + 单 Key 覆盖

Key 泄露后，如果任何来源都能调用，消费风险会被放大。
所以 Key 管理不能只靠“请不要泄露”这种口头约束。

它要能限制调用来源。
更适合企业客户的设计是两层白名单：
白名单层级 作用
全局白名单 给当前账号所有 Key 设置默认来源边界
单 Key 白名单 为某一把 Key 设置独立来源边界，并覆盖全局规则
白名单内容按 IP / CIDR 一行一条配置。
启用时应该至少有一条，数量也要有上限，避免把这个配置做成难以维护的规则库。
为什么需要两层？
因为企业客户既需要默认安全策略，也需要例外。
例如：
```
全部 Key 默认只允许公司出口 IP
生产 Key 单独允许云服务器 NAT 出口
测试 Key 临时关闭来源限制
外部客户 Key 单独绑定客户出口地址
如果只有全局白名单，生产、测试、外部客户会互相影响。
如果只有单 Key 白名单，配置成本又太高。
全局默认 + 单 Key 覆盖，才更适合企业场景。

## 消费限额：累计、每周、每月和超限策略要分开

很多平台只做账号余额。
这对企业客户不够。
账号余额回答的是：
```
整个账号还有多少钱？
Key 限额回答的是：
```
这个项目、这个环境、这个团队最多能花多少钱？
Key 级限额至少要拆成三类：

限额类型 解决什么问题
累计消费上限 控制一把 Key 总共最多花多少钱
每周消费上限 防止短周期异常流量打爆预算
每月消费上限 对齐企业内部月度预算和结算周期
列表里也要能看到预算进度：
```
已消费 / 总上限
周：已消费 / 周上限
月：已消费 / 月上限
这让采购和技术能提前看到预算风险，而不是等月底账单出来才发现异常。
限额之外，还要有超限策略。
策略 适用场景
超限停止 生产环境、强预算项目、外部客户项目
警告继续 测试环境、压测、临时验证场景
这两个策略不能混成一个。
生产环境通常更适合超限停止。
测试环境可以警告继续，避免低额度阻塞临时验证。
这类差异化配置，才是企业客户愿意继续往下看的地方。
因为它解决的是实际管理问题，不是功能列表好看。

## Key 不能孤立：要接上日志、统计和账单

Key 管理页本身要能跳到调用日志和用量统计。
因为管理员看到某把 Key 异常后，下一步通常不是继续看 Key 列表，而是：
```
看这把 Key 最近调了什么模型
看它花了多少钱
看它有没有失败请求
看它是不是命中了限额
看它是不是从不该出现的来源调用
调用日志里也要支持按 API Key 筛选。

用量统计里也要支持按 API Key 筛选。
账单明细里也要能把消费追到具体 Key 或具体请求。
这样才能形成闭环：
```
Key 管理设置边界
-> 请求执行时按边界校验
-> 调用日志按 Key 反查请求
-> 用量统计按 Key 看趋势
-> 账单按 Key 解释成本
如果 Key 管理只停在“创建密钥”，后面的日志、统计、账单都会少一个重要维度。
这就是为什么我认为 API Key 是企业级 token 平台的最小治理单元。
它既是鉴权入口，也是权限边界、预算边界、来源边界和排障边界。

## 调用日志：客户侧账本入口

调用日志很容易被做成“技术日志”。
比如只展示：
```
时间
模型
状态
token
这不够。
企业客户遇到一次失败调用，真正要查的是：
```
这条请求谁发的？
用的哪个 Key？
请求了哪个模型？
入口协议是什么？
成功还是失败？
用了多少 token？
命中了多少 cache？
扣了多少钱？
套餐和钱包怎么拆？
为什么这条请求慢或失败？
所以调用日志应该是客户侧账本入口，而不是后端日志截图。
调用日志页至少要支持这些筛选：

时间范围。
API Key。
请求 ID。
模型。
调用状态。
请求类型。
入口协议。
cache 命中。
表格里至少要展示：
字段 解决什么问题
时间 + 请求 ID 精确定位单次调用
API Key 找到调用方、项目或环境
模型 + 入口协议 判断从哪个协议、哪个模型进入
请求类型 区分对话、向量、图像、音频、视频等
prompt / completion / total tokens 解释基础文本用量
cache read / cache write 解释缓存命中和缓存写入
多模态指标 解释图片、音频、TTS、视频等用量
费用、折前、优惠、钱包费用 解释客户侧扣费
延迟和状态 判断性能和失败原因
更关键的是，调用日志要能展开账单明细。
展开后应该看到每个计费维度的数量、单位、单价、金额和结算来源。
这解决的是客户争议里最常见的问题：
```
为什么这条请求是这个金额？
如果只能看到一个总金额，客服和客户都没法继续解释。
如果能看到维度级明细，就能解释：
普通输入多少。
缓存读取多少。
缓存写入多少。

输出多少。
哪部分走套餐。
哪部分走钱包。
是否有折扣。
这也是企业级控制台和个人中转面板的区别。
客户不只是要“看到调用成功”，还要能解释每一笔费用。

## 用量统计：采购看趋势，技术看异常

调用日志解决单条请求。
用量统计解决一段时间内的趋势。
采购和技术看统计页，关注点不一样。
采购更关心：
钱花在哪些模型上。
哪些 Key 消耗异常。
套餐是不是被有效消耗。
钱包扣费是否增长过快。
cache 有没有省钱。
技术更关心：
哪个模型调用次数最多。
哪个模型错误率高。
哪个 Key 流量异常。
多模态调用是否增长。
当前模型分布是否符合业务预期。
所以用量统计页不能只显示总 token。
它至少要支持按时间范围和 API Key 筛选，并展示：
指标 价值
总请求、总 token、总费用、错误率 看整体趋势
模型消费结构 看钱花在哪些模型上
cache hit、cache read/write、缓存节省 看缓存是否真的降低成本
套餐抵扣 token、钱包计费 token 看是消耗套餐还是持续扣钱包

指标 价值
多模态指标 看图像、音频、视频、TTS 的增长
这几个字段放在一起，才能回答采购最关心的问题：
```
我们到底是在消耗预购套餐，还是在持续扣钱包？
如果统计页只显示总 token，就看不出这个问题。
如果同时展示 quota tokens 和 wallet tokens，采购就能判断套餐配置是否合理。
如果再展示 cache saved cost，产品和技术就能判断缓存策略有没有真实节省成本。

## 钱包账单：资金动作要能追到 AI 用量

模型调用最终都会落到钱。
所以 token 控制台必须和钱包、月账单、余额预警打通。
钱包页至少要展示：
```
可用余额
冻结金额
交易流水
交易流水可以按交易类型和日期范围筛选，用于定位充值、消费、退款和冻结记录。
月账单则按月展示：
充值。
消费。
退款。
净变动。
交易笔数。
在账单明细里，还要能看到关联的 AI 用量摘要，包括模型、请求类型、token、usage 来源
和钱包费用。
这让采购和财务能从资金流水追到模型用量。
余额预警则解决另一个问题：
```

余额不足不要等到请求失败时才发现。
它不是复杂功能，但对企业客户很实用。
尤其是按量付费场景里，余额不足会直接影响线上调用。
这里要守住一个边界：
客户侧控制台只展示客户账本、钱包流水、套餐抵扣和账单解释。供应商侧成本明细、平台
毛利和执行侧调度策略应该留在运营后台。
这不是信息隐藏。
这是账本边界。
客户需要解释自己为什么被扣费。
平台内部需要解释上游成本和毛利。
两条线不能混在同一个客户页面里。

## 模型广场要放在 Key 后面

模型广场当然重要。
但在企业级控制台里，它不应该抢在 Key 治理前面。
Key 管理先回答：
```
谁能用？
能用哪些模型？
最多能花多少钱？
能从哪里调用？
模型广场回答的是另一个问题：
```
这把 Key 到底应该开放哪些模型？
所以模型广场不能只做成模型名称列表。
它至少要展示：
信息 作用
模型类别 区分对话、向量、图像、视频、语音等能力
厂商和能力标签 判断模型适合通用问答、代码、多模态还是音频任务

信息 作用
上下文长度 判断是否能承载长文档、长对话或复杂工具调用
价格维度 判断输入、输出、缓存、多模态等成本结构
套餐余量 判断调用时优先消耗套餐还是钱包
当前折扣 判断不同用户等级下的实际价格
试用入口 选中模型后进入上线前验证
这对 Key 管理很重要。
因为模型白名单不能靠管理员凭记忆填模型名。
它应该基于模型广场里的公开模型目录做决策。
比如：
生产 Key 只开放稳定对话模型。
图像项目 Key 只开放图片生成和图片编辑模型。
语音项目 Key 只开放转写和文本转语音模型。
高成本模型只放给指定项目 Key。
模型广场展示的是客户可选择的公开模型目录，不是内部执行侧渠道清单。
客户需要看到能力、价格和可用性。
客户不需要看到供应商侧成本明细、渠道权重、上游凭证、转发实例和调度策略。
企业控制台要让客户会选模型，但不能把内部路由复杂度甩给客户。

## 模型详情要把价格讲透

模型广场解决“选哪个”。
模型详情解决“怎么算钱”。
很多平台的模型详情只写：
```
输入价格
输出价格
这对现在的 AI 模型已经不够。
真实计费维度要复杂得多。

尤其是支持缓存、多模态、视频、语音以后，客户看到一个总金额时，必须能知道钱花在哪
个维度。
模型详情里要把这些价格解释清楚：
价格维度 为什么要展示
input tokens 普通输入成本
output tokens 模型生成成本
cache read 缓存命中读取成本
cache write 缓存写入成本，可能按不同 TTL 拆价
image 图片生成、图片编辑等成本
video 视频时长、清晰度、比例等成本
audio second 语音识别、音频处理等成本
TTS character 文本转语音按字符或文本量计费
pricing rule 某些模型是否有条件价格或特殊计费规则
quota / wallet 本次调用优先走套餐还是钱包
企业客户不只关心平台标价。
他们还关心：
```
我的用户等级折扣是多少？
我的套餐还剩多少？
这次调用会不会扣钱包？
套餐过期时间是什么？
钱包补扣规则是什么？
所以模型详情页需要同时展示模型能力、上下文长度、按量付费状态、完整价格表、套餐剩
余额度、已使用额度、过期时间和钱包补扣规则。
如果一个模型支持套餐额度优先，就应该明确告诉客户：
```
优先消耗套餐额度
套餐不足时再按钱包价格补扣
模型详情不是宣传页。
它是客户做采购预算、产品选型和技术接入前的价格说明书。

## Playground 是上线前验证台

Playground 不应该是绕过权限的体验页。
它更适合放在 Key 和模型选型之后。
真正有价值的试用，是用真实账号、真实 Key、真实模型权限和真实余额状态，提前验证上
线风险。
上线前至少要验证三件事：
风险 Playground 要怎么挡住
选错模型 只能选择当前账号可用的模型
Key 权限不匹配 按模型白名单筛出可用 Key
成本不可控 展示余额、候选模型数量和可用 Key 数量
Playground 可以覆盖主要请求类型：
对话。
向量。
图片生成和编辑。
视频生成。
语音转写。
文本转语音。
这些不是“试试看”的小功能。
它们的价值是减少上线风险。
一个团队准备把图像能力接进业务，正确流程应该是：
```
先在 Key 管理里创建项目 Key
-> 只开放目标图像模型
-> 设置周/月消费上限
-> 配置调用来源白名单
-> 到模型广场确认价格和套餐余量
-> 到 Playground 用这把 Key 跑通参数
-> 再接入生产代码
这样试出来的结果才有意义。
因为它和生产环境共享同一套权限、余额和模型目录。
如果 Playground 绕过权限，它只能证明模型能用，不能证明客户的生产调用能安全上线。

## 文档中心要和控制台互相跳转

很多平台把文档中心做成外部站点，控制台做成另一个系统。
结果是：
```
客户看完文档，不知道在哪里创建 Key
创建完 Key，不知道如何配置 SDK
请求失败后，不知道查错误码、调用日志还是账单
更好的方式，是让文档和控制台互相跳转。
文档中心至少要覆盖：
快速入门。
认证与 API Key。
AI 编程工具配置。
API 参考。
计费与账单解释。
多模态接口。
视频生成。
Python / Node.js / cURL / Go 示例。
错误码。
模型列表。
数据留存与隐私。
故障排查和 FAQ。
它和控制台之间应该形成一条接入路径：
```
文档告诉用户怎么接入
控制台让用户创建 Key
模型广场让用户确认能力和价格
Playground 让用户用真实 Key 试模型
调用日志让用户验证请求
账单页让用户确认费用
这条路径跑通以后，平台的接入工单会明显减少。
因为客户遇到问题时，能先自助定位：
是认证问题。
是模型权限问题。

是余额或限额问题。
是参数不支持。
是上游失败。
是账单解释问题。
文档中心不只是写给开发者看的。
它也是客户成功、技术支持和销售交付的共同入口。

## 安全设置不能后补

Token 控制台是高风险入口。
因为它能创建 Key、看账单、管理余额、发起请求、配置权限。
所以安全能力不能等客户出问题后再补。
账户安全至少要包括：
修改登录密码。
最近登录记录。
登录 IP、设备、时间。
MFA 启用和关闭。
TOTP 二次验证。
Key 安全至少要包括：
Key 可禁用。
Key 可删除。
Key 可限制模型。
Key 可限制来源 IP / CIDR。
Key 可设置消费上限。
这些能力对应的是不同风险：
风险 控制台能力
账号被盗 MFA、登录记录
Key 泄露 禁用 Key、删除 Key、来源白名单
误调用高成本模型 模型白名单
预算被打爆 Key 级消费上限、周/月限额、超限策略
余额不足影响业务 余额预警

企业客户不只怕调用失败。
他们更怕的是：
```
Key 泄露以后还在继续扣费
测试 Key 打到了生产模型
某个团队把预算打爆
账号异常登录无人发现
余额不足导致线上业务中断
这些问题不是模型能力问题。
它们是控制台治理问题。

## 页面按任务拆，对象按事实拆

从工程实现看，控制台页面不是孤立 UI。
它们围绕同一组后端事实对象展开。
可以把核心关系拆成这样：
```
APIKey
-> Key 管理 / Playground Key 选择 / 调用日志筛选 / 用量统计筛选
UsageLog
-> 调用日志 / 账单明细 / 用量统计 / 上线验证
AIModel
-> 模型广场 / 模型详情 / Playground 模型选择
WalletTransaction
-> 钱包流水 / 月账单 / AI 消费关联
Docs
-> 快速入门 / API 参考 / 错误码 / 计费解释
Security
-> 登录记录 / MFA / 密码 / 账号保护
这里有三个工程原则。
第一，前端页面按用户任务拆。
用户不是按数据库表工作。
采购要看模型和成本，技术要查日志，产品要试模型，安全要管 Key 和账号。
第二，后端对象按业务事实拆。

模型是模型，Key 是 Key，usage 是 usage，钱包流水是钱包流水。
不要为了一个页面方便，把这些事实揉成一个不可复用接口。
第三，关键页面要能互相串起来。
例如：
```
API Key 先设置模型、来源和限额
-> 模型广场确认应该开放哪些能力
-> 模型详情确认价格和套餐余量
-> Playground 用这把 Key 试模型
-> 正式请求按 Key 执行
-> 调用日志按 Key 和请求 ID 查记录
-> 用量统计按 Key 和模型看趋势
-> 月账单查看钱包消费
这条链路越清楚，客户越能自助完成接入和排障。

## 企业级 token 控制台检查清单

如果你正在做 token 中转平台，可以直接拿这张表对照。
检查项 是否必须
API Key 创建时支持名称、限额、模型白名单和来源白名单 必须
API Key 有一次性明文展示和脱敏列表 必须
API Key 支持激活、停用、撤销、超限等状态区分 必须
API Key 支持按模型白名单限制可用模型 必须
API Key 支持全局来源白名单和单 Key 覆盖 企业客户必须
API Key 支持累计消费上限、周/月限额和超限策略 必须
API Key 列表能展示最后使用时间、7 日活跃和已设边界数量 必须
Playground 能按 Key 的模型白名单过滤可用 Key 必须
调用日志和用量统计能按 API Key 追踪 必须
调用日志能按请求 ID 精确查询 必须
调用日志能展示 usage、费用、延迟和状态 必须
调用日志能展开计费明细 企业计费必须
用量统计能按模型和 Key 分析 必须
统计页能区分套餐抵扣和钱包扣费 必须

检查项 是否必须
钱包流水能关联 AI 消费 必须
月账单能解释充值、消费、退款和净变动 必须
余额预警能提前通知 企业客户必须
模型广场能展示模型能力、价格、折扣和套餐余量 必须
模型详情能解释多维计费、条件价格和钱包补扣规则 必须
Playground 使用真实 Key、模型权限和余额状态 必须
文档中心覆盖认证、计费、错误码和示例 必须
控制台账号支持 MFA 和登录记录 企业客户必须
客户侧不暴露供应商侧成本信息和执行侧调度信息 必须
这张表背后的判断是：
企业级 token 控制台的核心，不是功能堆满，而是让客户从接入、上线、控费、排障到安全
治理都能自助闭环。

## 最后

一个 token 中转平台，如果只有 API Key 和余额，它更像个人工具。
要变成企业级平台，控制台必须同时服务四类角色：
采购看成本。
产品看能力。
技术看请求。
安全看边界。
这也是为什么我会把 API Key 管理放在这篇文章最前面。
因为 API Key 不是一个入口字段。
它是模型权限、调用来源、消费预算、运行状态和排障证据的交汇点。
这个系列会继续围绕企业级 Token 聚合平台，把网关、路由、计费、对账、控制台、文档和
上线门禁拆成可以复用的工程清单。
Adapter 的边界喜欢作者

链路
AI聚合平台之七: 如何让TokenA中I聚转合平平台台的之每五一：设计一C个lau企d业e 级Co Tdoek e和n C聚o合de平x 使用第三方平台 API
台OEM体系
条收费都经得起对账考验 Key 配置手册（Mac / Windows）
高可用设计
```

---


## 第9篇：接口设计：Token聚合平台的接口设计与实现逻辑

这不是一份模型名称清单，而是一套可复用的接口设计：模型如何发布、请求如何路由、不同模态
怎样计费，以及同步、流式和异步任务如何保持一致。
很多模型聚合平台都从 /v1/chat/completions 开始。接入两三个文本模型时，一个兼容接
口加一个转发层似乎就够了；等到 Responses、Anthropic Messages、图片编辑、语音转
写、TTS 和异步视频陆续加入，问题很快就变了。
此时需要管理的不再是一个 URL，而是一组完全不同的交付合同：JSON 能够回放，SSE 只
能防止重复执行，音频按秒或字符计费，图片按最终成品数计费，视频还要跨越创建、回
调、轮询、交付和终态结算。
本文以 16 个常见模型接口为例，拆解它们背后的模型目录、租户权限、API Key、渠道路
由、预算预留、usage 和账本逻辑。读者可以按同样的边界设计自己的模型聚合平台。

## 先看结论：16 条路由不是一条转发链

1. 一套完整的模型网关可以由 16 条模型相关路由组成：OpenAI-compatible、
Anthropic-compatible、Gemini Native 图片生成、平台原生异步视频任务及
Provider 回调。内部 request_type 应统一命名，语音合成建议只保留 tts ，避免统计
和定价出现两个口径。
2. 普通 Chat、Responses、Embeddings、Images、Audio 等模型并不是硬编码白名
单，而是运行期动态判断。一个模型真正可调用，需要同时满足：活跃模型目录、租户启
用与价格、API Key 白名单、活跃渠道/路由、能力门禁、余额/额度等条件。
3. 平台在每次请求中只选择 一个逻辑 Channel；Provider 级 retry / fallback 放在转发实
例边界。平台跨 Channel 重试与转发实例内部重试不能同时失控，否则一次请求可能被
重复执行。
4. 通用模型走动态目录和渠道配置；图片、音频、视频等具有特殊参数或计量单位的模型，
需要额外的能力契约和发布门禁。
5. 原生协议入口应按实际能力分阶段发布。例如只开放 Gemini 图片生成时，应明确拒绝通
用文本、输入图片、Live 和流式接口，不能因为 Provider 支持就自动透传。
6. 异步视频必须同时设计创建、回调、轮询、交付、取消和终态结算，缺少任意一环都不能
形成完整产品能力。
7. “模型已支持”需要同时具备目录、租户价格、API Key 权限、健康 Channel、能力门禁和
真实调用证据，不能只看配置文件或接口文档。
状态说明：

标记 含义
已发布 路由、服务、价格、渠道和调用证据均已准备完成
Beta / 受限 只开放明确列出的模型、字段或能力范围
待发布 处理链存在，但价格、渠道、白名单或验证证据尚未齐备
辅助接口 不执行完整推理计费主链，例如模型列表、token count、任务查询
Console 包 JWT 控制台入口，浏览器只提交 api_key_id ，后端进程内复用同一 GatewayServ
装 ice

## 一张图看懂整体架构

普通模型请求以转发实例为主要适配层。两个例外是：
Gemini Native 入口可按 Channel metadata 在 Google Direct 与转发实例之间选择。
Seedance 视频是平台原生异步适配器，不经过转发实例。

### 模型出现在目录里，不等于客户一定能调用

下图适用于 Chat、Responses、Anthropic、Embeddings、Images、Audio 和 Gemini
Native，不适用于走原生适配器的 Seedance 视频。

因此，“Provider 可配置”“目录为 active”“文档列出模型”“有 Channel”都不能单独证明模型
可用。 GET /v1/models 只是请求所属租户和 API Key 的模型发现快照：租户绑定池非空
时，它不会合并共享池，也不检查实例实时健康、pending 状态和并发容量；实际请求则可
能按具体模型回退共享池并执行这些实时检查。最终可调用性仍以真实请求经过全部门禁和
路由后的结果为准。
Seedance 视频不经过 tenant_ai_models 、通用 RouteContext 或 Channel Balancer，
而是走独立的 GA 门禁和 Doubao Native Channel 选择：

### 平台选择 Channel，转发实例处理 Provider 级重试

平台在一次上游失败后不再选择第二个 Provider Channel；Provider 级 timeout、retry
和 fallback 由转发实例负责。这样可以避免平台重试与转发实例重试叠加，造成重复执行和
成本失控。

## 模型支持面要拆成三层
### 第一层：通用目录模型

Chat、Embeddings 等标准模型适合通过模型目录、租户模型、价格规则和 Channel 动态
发布。新增模型时不修改网关主链，只新增目录数据、上游映射和价格配置。
Channel 可以对接 OpenAI、Anthropic、Gemini、Azure、DeepSeek、Mistral、
Qwen 、 Moonshot 、 MiniMax 、 Zhipu 、 Baidu 、 Meta 、 Doubao 或 其 他 第 三 方
Provider。这里的“可以配置”只表示能够进入适配层，不代表该 Provider 的所有参数、工具
和模态都已经发布。

### 第二层：特殊能力契约

图片、音频和视频模型不能只依赖一个 category 字段。每个模型还需要声明自己的能力契
约：
能力类型 至少需要声明的边界
Chat / Responses 上下文长度、输出上限、工具、多模态、缓存、流式
Embeddings 输入形态、批次数量、向量维度、输入 token 定价
Images 尺寸、比例、质量、格式、图片数、生成或编辑、是否流式
Audio Transcription 文件类型、文件大小、语言、返回格式、秒数计费
TTS voice、格式、speed、最大字符数、字符计费
Video 时长、分辨率、音频、参考资产、异步状态、交付方式

### 第三层：运行期可用性

 模型目录中的  | provider | 、 category |  和能力标签必须与真实接口一致。把 TTS 或转写模型 
标成 Chat，会让模型详情、参数表单、能力门禁和计费维度一起出错。
因此，可调用模型集合应使用下面的交集计算：
```
active 模型目录
∩ 租户已启用模型
∩ 已发布价格或 quota
∩ API Key allowed_models
∩ 可用 Channel
∩ Provider feature gate
模型名称出现在配置模板里，只能作为部署示例，不能直接进入客户可见清单。

## 16 个对外接口先看全表

协议 / 用

 # 方法与路径 |     |     | 模式 上游 | 计费与幂等 

途
 1 POST /v1/chat/c |     | OpenAI  C  | JSON 转发实例   | 输入、缓存、输出 token；非        
 ----------------- | --- | ---------- | ----------- | ----------------------- 
 ompletions        |     | hat  facad | / SSE       | 流式可按 request identity 回 
                   |     | e          |             | 放，流式只防重复、不回放            
 2                 |     | OpenAI     | R JSON 转发实例 | 文本 token；可含受控图片工        
POST /v1/respon
 ses |     | esponses | / SSE | 具；非流式可回放，流式只防 
重复
3 POST /v1/messag Anthropic JSON 转发实例 Anthropic input/cache 5m/
 es  |     | Messages | / SSE | 1h/output；非流式可回放， 
流式只防重复
 4 POST /v1/messag |     | Anthropic | JSON 转发实例      | 不预占、不写常规模型费用、    
 es/count_tokens   |     | token 预估  |                | 不扣费              
 5 GET /v1/models  |     | OpenAI 风  | JSON 模型目录 / 渠道 | 不调用 Provider，不计费 
格可用模型 查询
清单
 6 POST /v1/embedd |     | OpenAI   | E JSON 转发实例 | 按 input token；非流式可回 
 ings              |     | mbedding |             | 放                   
s
 7 POST        | /v1/image | OpenAI    | I JSON 转发实例；两个   | 按最终图片数和价格规则；同 
 s/generations |           | mages fac | / SSE Gemini 模型走 | 步可回放，流式只防重复   
ade Gemini 映射
 8 POST  | /v1/image | OpenAI    | I multi 转发实例 | 按最终图片数；同步可回放， 
 s/edits |           | mages edi | part /       | 流式只防重复        
         |           | t         | SSE          

协议 / 用

 # 方法与路径 |     |     | 模式  | 上游  | 计费与幂等 

途
 9 POST        | /v1/video | 平台异步视 | JSON | Doubao Seeda | 创建时预占，成功终态结算，         
 ------------- | --------- | ----- | ---- | ------------ | --------------------- 
 s/generations |           | 频创建   |      | nce Native   | 失败/过期由 finalizer 释放；r 
equest identity 复用任务
 1 GET /v1/videos/ |     | 查询视频任 | JSON | 平台任务库 | owner scoped，不新增推理 
 0 generations/:id |     | 务     |      |       | 费用                 
1 GET /v1/videos/ 代理下载视 二 进 受控抓取 Provi 仅  succeeded + proxy_strea
 1 generations/:i |     | 频/尾帧 | 制流  | der URL | m ，写访问审计，不新增推理 
 d/content/:inde  |     |      |     |         | 费用             
x
1 DELETE /v1/vide 取消视频任 JSON Doubao Seeda 调 Provider delete；终态幂
 2 os/generation |     | 务   |     | nce Native | 等返回；统一 finalizer 释放 
 s/:id           |     |     |     |            | 预算预占                
1 POST /v1/provid Provider JSON Provider → 平 task token + timestamp/no
 3 ers/doubao/seed |     | 状态回调 |     | 台   | nce/HMAC；检查 nonce cla 
 ance/callback     |     |      |     |     | im 结果；终态 CAS、交付与      
结算
 1 POST /v1/audio/ |     | OpenAI     | A multi | 转发实例 | 按音频秒数；响应有 duration 
 4                 |     | udio  Tran | part    |      | 则用真实值，否则按文件大小      
transcriptions
                   |     | scriptions |        |         | 估算；可回放              
 ----------------- | --- | ---------- | ------ | ------- | ------------------- 
 1 POST /v1/audio/ |     | OpenAI     | T JSON | 转发实例    | 按 Unicode 字符；先完成计   
 5 speech          |     | TS         | →  音   |         | 费提交再把音频流交给 Handl    
                   |     |            | 频流     |         | er；不可回放但防重复         
 1 POST /v1beta/mo |     | Gemini     | N JSON | Google  | Direct 按实际图片数；非流式可回 
 6 dels/{model}:ge |     | ative 图片   |        | 或转发实例   | 放；仅两个图片模型           
 nerateContent     |     | 生成         

### 三类协议入口如何鉴权

 接口族                |     | 可用鉴权                             
 ------------------ | --- | -------------------------------- | -------------------------- | --- | --- 
 OpenAI-compatible  |     | Authorization: Bearer 平台 API Key 
 与视频客户接口            |     |  或平台 HMAC                        
 Anthropic Messages |     | 上述两种 +                           | x-api-key: 平台 API Key      
 Gemini Native      |     | 上述两种 +                           | x-goog-api-key: 平台 API Key 
Seedance Provider c 不使用客户 API Key；服务层校验 task token、时间戳和 HMAC，并检查
 allback |     | nonce claim 结果； |     | claimed=false |  必须按重放请求拒绝 
Console  /api/v1/a 用户 JWT + owned  api_key_id ，浏览器不接触 API Key 明文
i/playground/*

Bearer / SDK header 鉴权会校验 Key 前缀与哈希、状态、硬消费限额、域名租户、来源
IP 以及 allowed_models 。HMAC 额外校验 method、path、body SHA256、±300 秒时
间窗和 Redis nonce 防重放。
对 Chat、Responses、Anthropic 和 Gemini 等原始 JSON，Handler 会拒绝客户覆盖以
下顶层字段： api_key 、 extra_headers 、 headers 、 base_url 、 api_base 。内部转发
实例 virtual key 或 instance master key 由服务端选择，客户的 Authorization 不会被当
作上游密钥透传。

## 逐个拆开 16 个接口的处理流程
### 5.1 GET /v1/models

这 个 接 口 不 会 向 转 发 实 例 或 Provider 发 起 模 型 discovery 。 模 型 详 情 接
口 /v1/models/{id} 未 注 册 ； 按 code 查 询 只 存 在 于 JWT Console
的 /api/v1/ai/models/:code 。

### 5.2 通用同步推理主链

适用入口：非流式 Chat、Responses、Anthropic Messages、Embeddings、Images、
Image Edits、Gemini Native、Audio Transcription。每个入口在参数、usage 和上游客
户端上有差异，但交易骨架一致。

计费可以采用两种提交模式：高吞吐场景先持久化 pending billing event，再由 Worker
写 usage log 和 charge lines；规模较小时也可以在请求线程内使用同步事务。无论采用哪
种模式，客户成功响应前都必须留下可恢复的计费事实。若计费提交失败，服务需要执行额
度、钱包补偿，或者转入 reconcile / review 状态。

### 5.3 POST /v1/chat/completions

Chat 会保留原始顶层 JSON 并进行最小改写； model 、 stream 和流式 include_usage 由
网关控制。消息数组会经过结构化 DTO 重写，因此 message 级未知扩展字段不是完全透明
透传。

### 5.4 POST /v1/responses

结果中包含图片时，预检和结算都要同时覆盖文本与图片。推荐分别生成文本输入、文本输
出和图片输出 charge line，避免把整笔请求切换成单一图片价格，导致文本费用丢失或无法
解释。

### 5.5 POST /v1/messages 与 /v1/messages/count_tokens

 Messages 的非流式响应可回放，流式只做重复成功保护。 |     |     |     |     |     |     | count_tokens |  是免费辅助预检 
 ------------------------------ | --- | --- | --- | --- | --- | --- | ------------ | -------- | --- 
接口，目前没有完整 request claim、账单日志或 charge line。

#### 5.5.1 GLM-5.2 经 Anthropic Messages 的 usage 与计费适配

GLM-5.2 可以通过智谱提供的 Claude API 兼容方式调用，但兼容协议与原生计费字段并不
天然一致。聚合平台需要依赖转发实例完成 usage 转换，并通过真实请求确认缓存命中、流
式终帧和输出 token 都能进入账单。
官方证据：
 智 谱 |   Claude  |     |     | API  兼 | 容   | 文   | 档   |   明 确 | 给   
出   base_url=https://open.bigmodel.cn/api/anthropic 、 model=glm-
  和  |                            |     |     |  调用示例，同时提示某些场景仍存在接口差异。 
 --- | -------------------------- | --- | --- | ---------------------- | --- | --- | --- | --- | --- 
 5.2 | /api/anthropic/v1/messages 
该页只打印  message.content  或整个 SDK 对象，没有列出  usage.input_tokens 、
 usage.output_tokens |             | 、   | cache_read_input_tokens |       |     |  等原始响应字段。 
 ------------------- | ----------- | --- | ----------------------- | ----- | --- | --------- | --- | --- | --- 
 GLM  Chat           | Completion  |     | 响                       | 应 定 义 |   明 | 确 包 含     |     |     | 、   
usage.prompt_tokens
 usage.completion_tokens |     |     |     |     |     |     |     |     | 、   
 ----------------------- | --- | --- | --- | --- | --- | --- | --- | --- | --- 
usage.total_tokens  和  usage.prompt_tokens_details.cached_tokens 。
 智 谱 上 | 下 文 缓 存 | 文 档 |   给 | 出 了 完 整 |   usage  | 样   | 例 ： |     | 、   
prompt_tokens=1200
 completion_tokens=300 |     |     | 、 cached_tokens=800 |     |     | 。该页还明确缓存是隐式自动识别，缓 
 --------------------- | --- | --- | ------------------- | --- | --- | ----------------- | --- | --- | --- 
存命中 token 是总输入 token 的子集，并按较低价格计费；示例口径为“新内容 = 总输
入 - 缓存命中”。
转 发 实 例 的   Anthropic  Messages  adapter  应 读 取   OpenAI  usage  中
的   prompt_tokens_details.cached_tokens ， 再 输 出   Anthropic  usage ：
input_tokens  =  prompt_tokens  -  cache_read_input_tokens  -
cache_creation_input_tokens ，并单独返回  cache_read_input_tokens 。
以一组  prompt=1200、cached=800、output=300  的 usage 为例，字段变换和收费拆分如
下：

这一拆分与智谱官方的基本计费规则一致：
 计费维度    | 官方 GLM 口径              | 转发实例 / 平台落点    | 结论  
 新输入 tok | prompt_tokens - cached | input_tokens   | 符合  
 en      |                        |  →             
         | _tokens                | input_uncached 
_tokens
缓存命中 t prompt_tokens_details. cache_read_input_ 符合，但必须确认真实 Messag
 oken | cached_tokens | tokens             | es 响应或转发实例转换确实返回 
      |               |  →  input_cached_r | 该字段              
ead_tokens
 输出 token | completion_tokens | output_tokens | 符合  
→  output_tokens
 缓存创建 /                        | 官方只描述首次请求自动              | 平台支持  cache_cre           | GLM 无官方字段证据，应保持       
 ----------------------------- | ------------------------ | ------------------------- | --------------------- | ----- 
 写入 token                      | 建立缓存，没有返回独立              | ation_input_token         | 为 0                   
                               | 写入量                      | s                         
 5 分钟 / 1                      | 官方未声明 Anthropic 式        | 平台支持 5m / 1h              | 不适用于 GLM 已公开的计费口      
 小时缓存写                         | TTL bucket               | 两种 charge line            | 径，不应为 GLM 配置这两类价      
 入                             |                          |                           | 格                     
 要使金额也符合官方规则，GLM-5.2 的租户价格必须把  |                          |                           | input_uncached_tokens |  配为标准 
 输入价，把                         | input_cached_read_tokens |  配为 GLM 缓存命中价，并把输出维度配为标准输 
出价。官方缓存页写的是“通常为标准价格的 50%”，这是说明性口径，不应替代目标套餐/账
号的实际价格表。
不能只凭兼容文档宣称“计费已验证正常”，原因有三点：

1. 智谱没有在 Claude API 兼容页公开 Messages 原始 usage 样例；兼容页本身也提示存
在部分差异。
2. 如果真实 /v1/messages 未返回缓存字段，平台只能把已返回的 input_tokens 当普通
输 入 计 费 ； 如 果 整 个 usage 缺 失 ， 则 会 进 入 平 台 token 估 算 并 记
录 usage_source=estimated ，无法证明与上游账单严格一致。
3. 流式请求必须确认最终 message_delta.usage / message.usage 中存在 input、
output 和 cache read 数据，不能只验证正文 SSE 能正常输出。
上线前最小验收应连续发送两次具有大段相同 system/history 的非流式请求，再重复一次流
式请求，并对比：Provider 原始响应、转发实例响应、平台 usage log、三个 charge line
（uncached input、cached read、output）及最终金额。缓存命中用例应满足 prompt =
uncached + cached read 、 total = prompt + output ，且不产生 GLM 未声明的 cache
write 5m/1h 费用。

### 5.6 Chat / Responses / Messages / Images 的流式主链

非流式成功快照可以回放；SSE 和二进制流无法安全回放，只能阻止同一 request identity
的重复成功调用。Gemini Images facade 明确拒绝流式。

### 5.7 POST /v1/embeddings

 Handler 应在入口明确拒绝空  | model 、空  | input 、混合类型数组和超大批次，不要把参数 
错误延迟到模型目录或上游。批量输入还要校验返回向量数与输入项数一致。
 5.8  /v1/images/generations |  与  /v1/images/edits 

需 要 注 意 ： OpenAI Images facade 的 Gemini 分 支 直 接 调 用 转 发 实 例 的
Gemini generateContent 客户端，不会复用下一节中的 Google Direct 选择逻辑。

### 5.9 POST /v1beta/models/{model}:generateContent

契约文件与运行时 Validator 必须保持一致。如果该入口只发布“文本提示生成图片”，就应明
确拒绝输入图片、图片编辑、通用文本生成和流式 Gemini Native，并在模型详情中展示同
一能力边界。

### 5.10 /v1/audio/transcriptions

按文件大小估算并不等于 Provider 官方音频时长，只能作为预算预留和上游缺失 duration
时的回退基线。usage log 必须同时记录秒数与 upstream 或 estimated 来源。

### 5.11 /v1/audio/speech

实际主链统一使用 request_type=tts ；代码中另有 audio_speech 枚举，但 API surface
mapper 把两者都映射到 OpenAI TTS。二进制响应不做历史 body 回放。

### 5.12 Seedance 视频：创建、Callback、Polling、交付与结算

### 任务辅助接口

视频能力应通过总开关、Provider 开关、租户、模型和 Channel 白名单逐步发布。回调入
口必须检查 nonce 的 NX claim 结果： claimed=false 应直接按重放拒绝。终态 CAS 可
以减少重复状态写入，却不能替代入口防重放。

## Console 与 Playground 没有另造一套网关

Console 通常需要四类 JWT 接口：模型目录与详情、Playground 可选模型、各模态测试入
口，以及按 request_id 查询 usage 和计费状态。

 Playground 不应该重新实现模型调用。浏览器只提交  |     | api_key_id | 、模型和请求参数，后 
端验证 Key 所有权后进程内复用同一个 GatewayService。这样控制台与外部 SDK 才能共
享模型权限、预算、路由、usage 和计费规则。
前端实现还要守住三个边界：
 1. 从模型广场跳转时读取  model |  参数，不能总是选择列表第一项。 
2. 视频创建后持续轮询任务和计费状态，直到进入终态。
3. SSE 客户端复用统一的 token refresh、协议错误解析和业务错误展示，不能只识别正文
增量。

## 上线前必须逐项检查的设计边界

优
 先 事项 | 影响  
级
阻 写接口和 Provider callba 全局安全中间件在 Gateway 路由前执行；标准 SDK 和 Provid
 断 ck 的安全中间件匹配 | er callback 必须进入各自鉴权流程，同时不能降低浏览器入口 
的防护
高 Seedance  Provider  call nonce store 对重复值会返回  claimed=false ；调用方必须同
 back 的 nonce 防重放 | 时检查 error 与 claimed，不能只依赖后续终态 CAS 
 ---------------- | --------------------------------- | -------------------------- | --- 
 高 取消异步任务必须进入 fin | 只更新  cancelled                    |  状态而不释放 reservation，会让预算长期 
 alizer           | 占用                                

优
 先 事项 |     | 影响  
级
 高 契约、Validator 与模型详 |     | 文档允许而运行时拒绝的字段会误导 SDK 和客户 
情保持一致
 高 /v1/models      |     | category、参数、架构和 request type 错配会直接影响计费与 
  正确描述 audio/video |     | 表单                                      
元数据
 高 Responses 图片工具采用 |     | 预检与结算都要覆盖文本和图片，不能在两个价格分支之间简单        
 组合计费               |     | 切换                                  
 中 同一模型的不同协议入口      |     | Native 与兼容 facade 的直连、流式和参数支持需要明确展示 
保持能力一致
 中 /v1/models |     | 模型列表不做实时并发承诺，但租户池与共享池规则不能互相矛 
  与真实路由共享回退策略 |     | 盾                            
 中 Channel    |  与权 | 严格优先级和同层加权是两种算法，不能只靠查询排序表达   
priority
重语义分开
 中 count_tokens |     | 免费预估不生成模型费用，但仍要记录调用频率、模型和延迟 
使用独立审计链
 中 Claude 兼容协议验证缓存 |           | 至少验证非流式、流式和缓存命中，并对齐 uncached、cache  
 usage             |           | d read、output 三类费用                  
 中 Embeddings      | /  Images | 空 model、input、prompt 和超大批次应在调用上游前拒绝 
在入口校验必填项
 中 Console 闭环模型跳转、 |     | 控制台必须与外部 SDK 复用同一业务链和错误语义 
视频轮询和 SSE 错误
 低 audio_speech          |     | 统计、价格配置和文档若未统一，容易重复或漏算能力 
  与  tts  双 request type 
 低 区分 OpenAI Video 兼容    |     | 两种协议不能用“支持视频”一句话混为一谈     
接口与平台异步视频接口

### 写接口的安全中间件顺序必须单独验证

根路径下的 SDK 接口、浏览器接口和 Provider callback 使用不同的认证材料。发布检查不
能只验证 Handler 本身，还要从全局中间件开始验证完整顺序：需要豁免浏览器 CSRF 校验
的机器接口，应继续接受 API Key、HMAC、时间戳与 nonce 等对应保护，不能用“关闭全
部校验”解决路径冲突。

## 16 个接口之外的能力需要单独设计

本文的 16 个接口覆盖同步推理、流式响应、多模态生成、音频和异步视频，但下面这些能力
不能直接套用同一条主链：
OpenAI legacy /v1/completions
/v1/batches
、 /v1/files 、 /v1/evals
Images variations
Fine-tuning、Vector Stores、Realtime
Rerank、Moderations
OpenAI-compatible Video API 与平台自定义异步视频接口
通用 Gemini 文本 generateContent 、Gemini streamGenerateContent 、Live API
其他 Anthropic 原生资源接口
Batch、service tier、data residency、grounding、context cache storage、Live、音
视频多模态、 asset:// 和真人参考资产都需要单独的权限、计费、合规和交付设计。即使
转发实例能够透传，也不能绕过平台的发布与计费门禁。

## 一个模型真正发布需要走完哪些步骤

模型接入向导跨越实例、Route、目录、价格、Channel 和租户选品，很难放进一个数据库
事务。实现时应为每一步保存状态，并提供幂等重试、反向补偿和人工恢复入口，避免中途
失败后留下无法识别的半成品配置。

## 判断“模型已支持”的证据顺序

判断一个模型是否“已经支持”，证据强度应从低到高排列：
1. 静态文档和示例配置
：只能证明团队计划支持，不能证明运行时可用。
2. 接口与服务代码
：能够证明入口和处理分支存在，仍不能证明配置已经发布。
3. 部署环境数据
：模型目录、租户模型、Channel、价格规则和 API Key 权限必须同时有效。
4. 指定租户的模型列表响应
：能够证明该租户和 Key 可以发现模型，但还不代表实时渠道一定健康。
5. 带真实 Provider 凭证的 smoke

：验证协议、参数、流式事件、usage 和错误响应。
6. 完整账单与对账证据
：请求日志、usage、费用明细、路由快照和上游账单能够逐项对应，才具备生产发布依
据。
任何“支持某模型”的发布文案，至少应附上部署环境、租户范围、模型目录状态、Channel、
价格规则、API Key 权限、接口面、验证时间和可追踪的请求证据。

## 写在最后

这 16 个接口放在一起，最值得关注的不是平台兼容了多少个 URL，而是每一种模态都建立
了独立且可解释的业务合同。
Chat 和 Messages 需要处理 token、缓存与流式事件；Responses 还可能同时出现文本和
图片费用；Images 和 Audio 改变了计量单位；TTS 在交付二进制流前必须先完成账本提
交；Video 则把一次模型调用扩展为跨请求、跨 Worker、跨回调的异步任务。
对产品经理来说，这份盘点给出了“支持某模型”应包含的完整能力边界；对采购和运营来说，
它解释了模型目录、价格、渠道与真实可用性之间的差别；对开发团队来说，它可以直接作
为接口验收和模型发布的核对基线。
企业级模型聚合平台的核心竞争力，不是把 Provider URL 换成自己的域名，而是让每次请
求都能被授权、被路由、被计量、被结算，也能在失败和争议发生时被完整解释。
Adapter 的边界
链路
Claude Code 和 Codex 使用A第I聚三合方平平台台之 A五PI：设计一T个o企ke业n聚级合 T平ok台en的 聚鼻合祖平OpenRouter，凭什么
Key 配置手册（Mac / Windo台wOs）EM体系 值 13 亿美元？
高可用设计
```

---


## 第10篇：案例分析：OpenRouter 凭什么值 13 亿美元

它不以自研基础模型为核心，却把模型、供应商和开发者组织成一张推理网络。13 亿美元估

值背后，资本真正看中的不是模型数量，而是分发、流量、数据和企业治理形成的控制力。

2026 年 5 月，OpenRouter 完成 1.13 亿美元 B 轮融资。公司没有公开披露估值，但媒体

报道其投后估值约为 13 亿美元。TechCrunch

一个成立于 2023 年、不以训练基础模型为核心的 Token 聚合平台，凭什么得到这个估值？

如果只看产品表面，答案似乎是模型多：模型广场、API Key、余额、日志、渠道、工作空

间、限额和监控，一个都不少。

但 13 亿美元买的显然不是一张功能清单。真正值钱的是 OpenRouter 已经占据了应用与上

游模型之间的推理控制点，并形成了三条相互增强的增长链：

```
开 发 者 越 多  -> 供 应 商 越 愿 意 接 入
请 求 越 多    -> 路 由 数 据 越 准 确

生 产 流 量 越 大  -> 企 业 治 理 需 求 越 强

按公开数据，它已经是事实上的头部入口：当前官方供应商页面称平台连接 70 多家供应商、

服务超过 1000 万开发者；定价页列出 400 多个模型；2026 年 5 月披露的周处理量达到

25 万亿 Token，半年增长约 5 倍。Provider Network、Pricing、融资与规模披露

这些数字主要来自公司公开披露，并非统一市场审计数据。目前也没有覆盖所有 AI Gateway

和模型聚合平台的权威市场份额报告，因此本文不试图证明它是严格意义上的全球第一，而

是分析它成为头部平台的机制。

要看清 13 亿美元估值的逻辑，不能只看 OpenRouter 今天有什么，还要看这些能力按什么

顺序出现。

从公开版本记录看，它的演进主线很清晰：

```
模 型 市 场 与 统 一  API
-> 多 供 应 商 路 由
-> 用 量 和 计 费 透 明

-> 质 量 感 知 路 由
-> 企 业 治 理 与 多 模 态

-> Agent 运 行 基 础 设 施


这条时间线说明：聚合平台不能从“大而全的控制台”开始。它要先形成模型交易闭环，再把

路 由 、 账 单 和 可 靠 性 做 成 平 台 能 力 ， 最 后 才 有 资 格 进 入 企 业 治 理 和   Agent  编 排 。

OpenRouter 的估值基础，也不是模型数量单独带来的，而是开发者分发、供应商网络、真

实流量数据和企业治理共同作用的结果。

本文只分析带来平台能力变化的版本，普通模型上新不计入功能演进。官方公告归档主要覆

盖 2024 年 12 月以后；更早阶段根据官方成立时间、后续版本的既有能力和现有文档反向归

纳。

## 第一阶段：模型市场先验证统一交易

### 时间：2023 年至 2024 年

OpenRouter 官方称平台始于 2023 年。早期产品更接近一个 LLM Marketplace：开发者

使用一个账户、一份余额和一套统一接口，访问不同模型。OpenRouter About

这一阶段形成了后续演进所需的最小底座：

OpenAI-compatible 统一接口；

模型目录和模型详情；

平台余额与统一结算；

多家模型供应商接入；

基础的供应商回退；

在线调试和调用记录。

此时最重要的不是高级路由，而是验证三个问题：开发者是否愿意通过一个入口访问多个模

型，平台能否持续扩充模型供给，充值和调用能否形成完整交易闭环。

```
一 个  API Key

-> 统 一 模 型 目 录

-> 不 同 模 型 和 供 应 商
-> 统 一 余 额 与 调 用 记 录

### 产品判断：模型数量只是供给规模，统一账户、统一接口和统一结算才是聚合平台的最小商业闭环

## 第二阶段：BYOK 把平台从 Token 销售商变成基础设施

### 时间：2024 年 12 月

2024 年 12 月，OpenRouter 集中发布 BYOK、结构化输出、加密货币支付、端点信息和

Web Search 等能力。官方公告归档


其中影响产品边界最大的是 BYOK。用户可以继续使用自己在上游供应商的账户和额度，同

时获得 OpenRouter 的统一 API、路由与用量分析。Bring Your Own API Keys

平台由此可以同时服务三种关系：

接入方式

资金关系

平台提供的价值

平台统一充值

用户向平台购买额度

模型供给、路由、结算

用户自带 Key

用户与供应商直接结算

兼容、路由、分析、回退

企业供应商合同

企业拥有议价和配额

统一治理、观测和权限

### 产品判断：当客户可以绕过平台余额，平台仍然有使用价值，说明它出售的已经不只是 Token，而是统一接入、路由和治理能力

## 第三阶段：路由从内部逻辑变成对外产品

### 时间：2025 年 1 月至 3 月

Auto Router 可以根据请求自动选择模型；Nitro 和 Floor 分别按吞吐量与价格选择供应商

端点。Auto Router、Nitro and Floor

供应商路由逐步形成一套可配置策略：

按价格、吞吐量或延迟排序；

设置供应商优先级和自动回退；

按参数兼容性过滤端点；

限制数据收集、地域和最高价格；

只选择满足零数据保留要求的供应商。

这些能力后来沉淀为完整的 Provider Routing 配置体系。Provider Routing 文档

同期上线的 Zero Completion Insurance 规定：没有产生有效输出，或者以特定错误原因

结束的请求不收费。Zero Completion Insurance

这里出现了一个关键分界：上游执行过请求，不等于下游客户应该付费。 聚合平台必须定义

自己的成功语义、收费语义和异常处理规则。

### 产品判断：平台价值从"替客户接入多个模型"，升级为"替客户选择更合适的模型和供应商，并屏蔽失败差异"

## 第四阶段：用量与配置开始服务生产流量

### 时间：2025 年 4 月至 6 月

OpenRouter 在这一阶段补齐了生产使用需要的控制能力：


按模型、API Key 和供应商查询调用记录；

在响应流中返回 Token 与费用；

查看端点可用率；

管理和测试 BYOK Key；

把模型、路由和生成参数保存为 Preset。

Live Usage回答“这一笔请求用了多少、花了多少”；Presets则把模型、供应商偏好、系统提

示词和生成参数从业务代码中分离出来。

```
业 务 代 码 只 引 用  Preset

-> 控 制 台 调 整 模 型 、 参 数 和 路 由
-> 新 配 置 直 接 作 用 于 后 续 请 求

-> 业 务 代 码 不 必 重 新 发 布

Preset 看起来只是配置功能，实际解决的是模型配置频繁变化与业务发布周期不一致的问

题。它让模型切换、A/B 测试和供应商调整从代码发布动作变成平台运营动作。

### 产品判断：完成"能调用"之后，下一步不是继续堆模型，而是让调用可查询、成本可解释、配置可变更

## 第五阶段：同一个模型的不同端点不再被视为等价

### 时间：2025 年 7 月至 2026 年 3 月

多供应商聚合到一定规模后，OpenRouter 开始公开强调 Provider Variance：即使模型名

称相同，不同供应商端点的工具调用正确率、吞吐量和稳定性仍可能不同。

Exacto 利用真实工具调用结果，分析 JSON 合法性、Schema 匹配和工具名称正确率，再

选择质量更好的供应商端点。Exacto

后续版本继续增加：

Response Healing：修复部分格式错误的 JSON；

Provider Explorer：比较端点价格和性能；

吞吐量与延迟阈值；

模型基准测试和有效价格；

Auto Exacto 动态质量路由。

Response Healing把一部分供应商响应差异收口在网关；Auto Exacto则根据持续变化的质

量和性能数据重新评估端点。

```
静 态 路 由 ：

价 格 最 低  -> 选 择 供 应 商


质 量 感 知 路 由 ：

参 数 支 持  + 实 时 延 迟  + 实 际 吞 吐 量
+ 工 具 调 用 正 确 率  + 历 史 稳 定 性

-> 选 择 供 应 商

### 产品判断：路由不再只是权重和优先级配置，而是由真实请求质量数据驱动的持续决策系统

## 第六阶段：企业客户需要组织级治理

### 时间：2026 年 4 月至 5 月

Workspaces 把一个企业账户拆分为多个项目、团队或 Agent 环境。每个 Workspace 可以

拥有独立的 API Key、BYOK 配置、路由默认值、Preset、成员和观测数据。Workspaces

Guardrails 进一步提供：

日、周、月预算限制；

模型和供应商白名单；

零数据保留要求；

Prompt Injection 检查；

DLP 与敏感信息检查；

按成员或 API Key 分配策略；

通过 Management API 管理策略。

Guardrails把预算、安全和模型权限从“账户设置”升级为可分配的组织策略。企业版同时提供

统一账单、发票、信用额度、SSO、IP 白名单和观测平台集成。OpenRouter Enterprise

### 产品判断：当平台服务对象从个人开发者变成企业，账户模型必须升级为组织、项目、成员、Key 和策略之间的关系模型

## 第七阶段：多模态复用同一套治理底座

### 时间：2026 年 4 月至 6 月

OpenRouter 陆续增加异步视频生成、Speech、Transcription、统一图片生成、Response

Cache，以及服务端 Web Search 和 Web Fetch。

Video  API 、 Audio  API 和 Image  API 并 不 是 三 个 孤 立 产 品 。 它 们 复 用 已 有 的 身 份 、 API

Key、供应商路由、统一账单、Usage 记录和企业策略。

```
文 本  / 图 片  / 语 音  / 视 频
-> 统 一 身 份 与  API Key

-> 能 力 发 现 与 参 数 归 一
-> 供 应 商 路 由


-> 用 量 、 成 本 与 账 单
-> 企 业 安 全 策 略

### 产品判断：多模态平台真正需要复用的不是 URL 风格，而是认证、能力发现、路由、计量、结算和治理底座

## 第八阶段：从调用模型走向编排智能

### 时间：2026 年 6 月至 7 月

OpenRouter 随后发布 Advisor、Fusion、Subagent 和 MCP Server 等能力：

Advisor：让普通模型向更强模型发起咨询；

Fusion：多个模型并行生成，再综合结果；

Subagent：主模型把相对独立的工作委派给其他模型；

MCP Server：向编程 Agent 暴露模型目录、价格、基准和测试能力。

相关版本可以在官方公告归档中查看。2026 年 7 月的品牌更新将平台描述为 intelligence

infrastructure，而不再只是模型连接接口。Brand Refresh

### 产品判断：模型聚合平台开始介入模型选择、任务拆分、结果综合和 Agent 工具发现，平台边界从 Gateway 延伸到 Runtime

## 八阶段演进构成 13 亿美元的产品底座

阶段

核心能力

验证的产品命题

V0

V1

V2

V3

V4

V5

V6

V7

统一 API、模型目录、充值

能否形成模型交易闭环

多供应商路由、故障回退

能否屏蔽供应商差异

BYOK、端点发现、Key 管理

不卖 Token 时是否仍有价值

Usage、成本分析、Preset

调用能否查询、解释和运营

质量路由、响应修复

同模型多端点能否择优

Workspace、预算和 Guardrails

能否进入企业组织和治理体系

图片、语音和视频 API

多模态能否复用统一底座

Advisor、Fusion、Subagent、MCP

能否成为 Agent 运行基础设施

这张表最值得借鉴的不是功能名称，而是依赖关系：没有统一调用和交易数据，就做不好路

由；没有请求级 Usage 和端点观测，就做不了质量感知；没有账户、Key、账单和策略模

型，也支撑不了企业 Workspace。

## OpenRouter 占据的是推理控制点


把八个阶段连起来看，OpenRouter 并没有把自己限制成“销售模型额度的平台”。它选择了

一个更有价值的位置：应用与模型供应商之间的推理控制层。

```
开 发 者 与  AI 应 用

-> 模 型 选 择 、 路 由 、 计 费 、 观 测 、 治 理
-> 模 型 厂 商 与 推 理 供 应 商

对开发者，它提供统一接入、模型选择和生产可靠性；对供应商，它提供生产流量、曝光、

自动结算和性能反馈。平台则掌握标准化、匹配、路由和交易证据。

这个位置有一个特殊优势：上游模型越多、价格变化越快、供应商质量差异越大，下游越需

要一个统一控制层。市场碎片化不会削弱它，反而会增加它存在的价值。

## 最低迁移成本完成开发者获客

OpenRouter 采用 OpenAI-compatible 接口。已有应用通常只需要调整  baseURL  和 API

Key，就能继续使用原来的 OpenAI SDK。Quickstart

这个选择带来三个直接结果：

1.  第一次调用可以在几分钟内完成。

2.  已有 AI 应用不必重写模型调用层。

3.  新模型上线后，开发者不必重新接入一套供应商 SDK。

兼容接口本身很容易复制，因此不是护城河。它真正的作用是降低获客阻力，把开发者尽快

带入后面的模型目录、余额、Usage、路由和企业治理体系。

## 双边市场让供给和需求相互增强

OpenRouter 没有只依赖少数固定上游，而是把供应商接入变成标准化市场。供应商需要声

明模型、参数、上下文、价格、数据中心、隐私政策和 Usage，并接受延迟、吞吐量和可用

率监测。

平台公开展示 TTFT、吞吐量和可用率，并按价格、性能与可靠性分配流量。表现更好的供应

商自动获得更多请求；可用率下降后，流量权重也会下降。供应商接入机制

```
更 多 开 发 者 和 请 求

-> 供 应 商 愿 意 接 入

-> 模 型 、 区 域 和 容 量 增 加
-> 路 由 选 择 和 稳 定 性 提 高
-> 吸 引 更 多 开 发 者 和 请 求

这构成了第一层网络效应。开发者获得更多供给，供应商获得更多需求，平台获得更大的路

由空间。


## BYOK 降低企业迁移阻力

BYOK 允许企业继续使用自己的上游合同、云厂商额度、供应商限额和预置吞吐量，同时使

用 OpenRouter 的统一接口、故障回退、用量分析和预算控制。BYOK 公告

它体现了一条重要的商业原则：

即使客户不向平台购买 Token，平台仍然要有独立价值。

这让 OpenRouter 不必把所有客户都变成 Token 转售客户。它还可以出售路由、兼容、观

测和企业治理能力，从而减少与大客户现有供应商合同的冲突。

## 透明定价建立中立信任

OpenRouter 对外强调上游推理价格透明，主要通过充值平台费、BYOK 超额费用和企业服

务收费。当前定价页列出了 Free、Pay-as-you-go 和 Enterprise 三档方案，并将平台费、

模型价格和企业能力分开说明。Pricing

对于聚合平台，客户真正关心的不只是单价，还包括：

这次请求走了哪个供应商；

最终 Token 和费用是多少；

BYOK 是否真正生效；

失败和回退是否收费；

供应商价格变化后如何生效。

价 格 高 度 透 明 时 ， 隐 藏 差 价 可 以 带 来 短 期 利 润 ， 却 会 伤 害 路 由 平 台 最 重 要 的 中 立 性 。

OpenRouter 的经营选择，是用相对可解释的费率换取开发者愿意把更多生产流量交给平

台。

## 真实请求数据形成第二层飞轮

OpenRouter 每处理一次请求，都能积累一组运行数据：模型、供应商、TTFT、总延迟、

Token 吞吐量、错误类型、费用和回退结果。

平台使用滚动窗口统计供应商的 p50、p90、p99 延迟和吞吐量，并允许开发者按照性能门

槛筛选端点。Provider Routing

```
更 多 生 产 请 求

-> 更 多 真 实 性 能 和 质 量 数 据

-> 更 准 确 的 供 应 商 选 择
-> 更 高 的 成 功 率 和 性 价 比

-> 更 多 生 产 请 求


Exacto 和 Auto Exacto 又把工具调用正确率、JSON 合法性和基准表现加入路由判断。竞

争者可以复制路由算法，却无法立即复制同等规模的生产流量和历史结果。这是比模型数量

更强的壁垒。

## 运营策略把产品数据变成分发渠道

OpenRouter 的运营不只依赖广告。它把模型发布、免费体验、排行榜、应用曝光和行业研

究组合成一套持续获客系统。

### 热点模型承接搜索需求

重 要 模 型 发 布 后 ， 开 发 者 会 集 中 搜 索   API 、 价 格 、 上 下 文 、 工 具 调 用 和 供 应 商 性 能 。

OpenRouter 为模型建立长期页面，把模型热点转化成搜索流量，再把流量引向在线体验和

API 调用。

高频更新的模型页、供应商页、价格页和版本公告，使平台天然拥有大量高意图的技术搜索

内容。版本公告归档

### 免费模型承担开发者获客成本

免费模型把注册、充值和生产接入拆成几个低风险动作。开发者可以先免费完成调用，再决

定是否购买额度或接入正式应用。

OpenRouter 曾公开表示会接入更多免费供应商，并直接承担部分热门模型的推理成本。免

费层策略

```
搜 索 模 型

-> 免 费 体 验
-> 创 建  API Key

-> 接 入 开 发 工 具
-> 购 买 额 度 或 配 置  BYOK
-> 生 产 环 境 持 续 使 用

免费层不是利润中心，而是开发者获取和首次成功体验的成本。

### Stealth Model 制造独家供给

匿名 Alpha 模型让开发者提前免费测试尚未公开身份的模型，并把真实反馈提供给模型实验

室。Optimus Alpha

模型厂商获得盲测和反馈，开发者获得提前体验，OpenRouter 则获得独家流量、社区讨论

和模型发布话语权。这比普通“新模型上线”更有传播性。

### 应用排行榜让客户帮助平台传播


开发者在请求中增加应用归属 Header 后，可以进入公开应用排行榜，并获得应用页和用量

分析。App Attribution

开发者获得曝光，OpenRouter 则获得更多归属明确的请求数据、公开案例、外部链接和社

区传播。这是一种用曝光交换数据与分发的机制。

### 行业报告把平台变成数据来源

OpenRouter 把真实用量做成模型排名、趋势页面和研究报告。其 100 万亿 Token 研究被

投资机构、研究者和媒体用于分析真实模型采用情况。State of AI

当行业讨论“哪个模型被真实使用”“开发者正在转向什么任务”时开始引用 OpenRouter，平台

就不再只是 API 服务商，也成为市场信息来源。

## 产品驱动增长完成企业转化

OpenRouter 的商业路径是典型的 Product-Led Growth：先让个人开发者免费或小额使

用，再从生产流量中产生团队管理需求。

```
个 人 开 发 者 试 用

-> 小 团 队 共 享
-> 应 用 进 入 生 产

-> 需 要 预 算 、 日 志 和 权 限
-> 建 立  Workspace

-> 接 入  SSO、 审 计 和 合 规
-> 转 为 企 业 合 同

这条路径降低了企业销售的教育成本。进入采购流程之前，技术团队已经验证了模型覆盖、

路由和兼容性；销售需要完成的是组织治理、合同与服务保障，而不是从零证明产品能否使

用。

## 13 亿美元买的不是每一项功能

能力

强度

形成壁垒的条件

OpenAI-compatible API

模型目录与数量

供应商网络

开发者分发

弱

中

强

强

只能降低接入门槛，很容易复制

需要持续维护价格、参数和能力元数据

依赖流量、结算、准入和长期合作

成为工具和应用的默认模型入口

真实性能数据

很强

依靠大规模生产请求长期积累

质量感知路由

强

算法必须与真实结果数据结合

统一账单与历史

中强

形成财务、运营和排障迁移成本


能力

强度

形成壁垒的条件

企业治理与合同

排名与行业数据

强

强

进入权限、合规、采购和组织流程

形成搜索、引用和品牌网络

真正的护城河不是单个功能，而是一组能力相乘：

```
开 发 者 分 发

x 供 应 商 流 动 性
x 真 实 请 求 数 据
x 路 由 质 量

x 交 易 信 任
x 企 业 治 理

只做统一接口，很容易被替换；只接很多模型，容易陷入价格竞争；只有当请求规模反过来

改善供应商网络和路由质量，平台才形成自我增强的壁垒。

## 头部平台仍然存在边界

OpenRouter 的优势并非不可挑战。

1.  上游依赖：

平台不拥有大部分模型和计算资源，供应商可以改变价格、容量和合作政策。

2.  中心化故障：

聚合平台自身故障可能同时影响所有模型。

3.  利润空间：

推理价格透明，平台不能长期依赖高差价，需要依靠规模和企业服务。

4.  云平台竞争：

大型云厂商拥有算力、企业合同和合规能力。

5.  兼容接口的双刃剑：

OpenAI-compatible 帮助平台获客，也降低了客户切换到其他网关的成本。

2026 年 2 月，OpenRouter 曾因缓存依赖故障出现大面积请求失败。后续修复包括断路

器、受控缓存回填和更准确的错误码。故障复盘

这说明聚合平台一旦成为统一入口，自身可靠性就必须高于单一供应商。发布公开复盘也是

其信任运营的一部分，但真正的壁垒仍然来自持续兑现稳定性。

## 自建 AI 聚合平台的迭代清单

如果正在规划自己的 AI Token 聚合平台，可以按下面的门槛推进，而不是一次性复制全部功

能。

### 1. 先闭合交易

统一 API、模型目录、余额、调用记录能完整跑通。

### 2. 再收口执行

同模型支持多个供应商端点，具备健康过滤和故障回退。

### 3. 补齐证据

每个请求可以查询模型、端点、Token、费用、状态和错误。

### 4. 开放客户资产

支持 BYOK、Key 限额、模型白名单和供应商策略。

### 5. 让配置脱离代码

路由、模型和生成参数可以独立变更和审计。

### 6. 用数据驱动路由

把价格、延迟、吞吐量、成功率和输出质量纳入选择。

### 7. 再进入企业治理

建设组织、项目、成员、预算、安全策略和管理 API。

### 8. 最后扩展运行时

在统一治理底座上承载多模态和 Agent 编排。

### 9. 建立增长入口

用免费模型、热点教程、工具集成和应用案例降低获客成本。

### 10. 公开市场数据

把模型价格、供应商表现、Usage 趋势和故障复盘变成可信内容。

这条路线的核心不是“先少做功能”，而是让每一阶段都拥有可验证的产品目标。前一阶段没有

稳定数据，后一阶段的高级路由和企业治理就只是控制台上的配置项。

## 13 亿美元最终买的是什么

OpenRouter 的功能迭代说明，AI 聚合平台的护城河不是模型数量，而是逐步形成的六类能

力：开发者分发、供应商网络、流量调度、交易证据、真实数据和组织治理。


```

---


## 第11篇：附录：Claude Code / Codex 第三方平台 API Key 配置手册

这篇只讲配置。

## 先准备 4 个值

找第三方平台后台或文档，拿到下面 4 个值：
 名称               | 示例占位符 |     | 用途                                
 Claude Code Base |       |     | Claude Code 走 Anthropic-compatibl 
YOUR_ANTHROPIC_COMPATIBLE_B
 URL | ASE_URL |     | e 网关 
Codex Base URL YOUR_OPENAI_COMPATIBLE_BASE Codex 走 OpenAI / Responses-comp
         | _URL |     | atible 网关    
 API Key |      |     | 第三方平台颁发的 Key 
YOUR_THIRD_PARTY_API_KEY
 模型名 | YOUR_MODEL_NAME |     | 第三方平台允许调用的模型 ID 
注意：
Claude Code 的 Base URL 一般对应 Anthropic Messages 兼容入口。
Codex 的 Base URL 一般对应 OpenAI / Responses 兼容入口。
 Base URL 是否带   | /v1 ，按第三方平台文档来，不要自己猜。 
 下文所有  YOUR_... |  都替换成你自己的值。           

## Mac 配置 Claude Code
### 方式一：临时配置

打开终端，执行：
bash
export ANTHROPIC_BASE_URL="YOUR_ANTHROPIC_COMPATIBLE_BASE_URL"
export ANTHROPIC_AUTH_TOKEN="YOUR_THIRD_PARTY_API_KEY"
export ANTHROPIC_MODEL="YOUR_MODEL_NAME"
claude
 如果第三方平台文档明确要求使用  |     |     | ，改成： 
ANTHROPIC_API_KEY
bash

export ANTHROPIC_BASE_URL="YOUR_ANTHROPIC_COMPATIBLE_BASE_URL"
export ANTHROPIC_API_KEY="YOUR_THIRD_PARTY_API_KEY"
export ANTHROPIC_MODEL="YOUR_MODEL_NAME"
claude
不 要 同 时 配 置 两 种 认 证 变 量 。 走 网 关 或 代 理 时 ， 优 先 按 平 台 文 档 使
用 ANTHROPIC_AUTH_TOKEN 。

### 方式二：长期配置到 zsh

编辑 ~/.zshrc ：
bash
vi ~/.zshrc
追加：
bash
export ANTHROPIC_BASE_URL="YOUR_ANTHROPIC_COMPATIBLE_BASE_URL"
export ANTHROPIC_AUTH_TOKEN="YOUR_THIRD_PARTY_API_KEY"
export ANTHROPIC_MODEL="YOUR_MODEL_NAME"
使配置生效：
bash
source ~/.zshrc
claude

### 方式三：写入 Claude Code settings

创建配置目录：
bash
mkdir -p ~/.claude
vi ~/.claude/settings.json
写入：
json
{
"env": {
"ANTHROPIC_BASE_URL": "YOUR_ANTHROPIC_COMPATIBLE_BASE_URL",
"ANTHROPIC_AUTH_TOKEN": "YOUR_THIRD_PARTY_API_KEY",
"ANTHROPIC_MODEL": "YOUR_MODEL_NAME"

}
}
启动：
bash
claude

### 验证 Claude Code

进入 Claude Code 后执行：
```
/status
再发一句：
```
用一句话回复：ok
如果第三方平台后台能看到这次调用，说明配置生效。

## Windows 配置 Claude Code

以下命令在 PowerShell 中执行。

### 方式一：临时配置

powershell
$env:ANTHROPIC_BASE_URL="YOUR_ANTHROPIC_COMPATIBLE_BASE_URL"
$env:ANTHROPIC_AUTH_TOKEN="YOUR_THIRD_PARTY_API_KEY"
$env:ANTHROPIC_MODEL="YOUR_MODEL_NAME"
claude
如果第三方平台文档明确要求使用 ANTHROPIC_API_KEY ：
powershell
$env:ANTHROPIC_BASE_URL="YOUR_ANTHROPIC_COMPATIBLE_BASE_URL"
$env:ANTHROPIC_API_KEY="YOUR_THIRD_PARTY_API_KEY"
$env:ANTHROPIC_MODEL="YOUR_MODEL_NAME"
claude
临时配置只对当前 PowerShell 窗口有效。

### 方式二：长期配置到用户环境变量

powershell
[Environment]::SetEnvironmentVariable("ANTHROPIC_BASE_URL",
"YOUR_ANTHROPIC_COMPATIBLE_BASE_URL", "User")
[Environment]::SetEnvironmentVariable("ANTHROPIC_AUTH_TOKEN",
"YOUR_THIRD_PARTY_API_KEY", "User")
[Environment]::SetEnvironmentVariable("ANTHROPIC_MODEL", "YOUR_MODEL_NAME", "User")
关闭 PowerShell，重新打开。
检查变量：
powershell
echo $env:ANTHROPIC_BASE_URL
echo $env:ANTHROPIC_AUTH_TOKEN
echo $env:ANTHROPIC_MODEL
启动：
powershell
claude

### 方式三：写入 Claude Code settings

打开配置文件：
powershell
New-Item -ItemType Directory -Force "$env:USERPROFILE\.claude"
notepad "$env:USERPROFILE\.claude\settings.json"
写入：
json
{
"env": {
"ANTHROPIC_BASE_URL": "YOUR_ANTHROPIC_COMPATIBLE_BASE_URL",
"ANTHROPIC_AUTH_TOKEN": "YOUR_THIRD_PARTY_API_KEY",
"ANTHROPIC_MODEL": "YOUR_MODEL_NAME"
}
}
保存后重新打开终端，再执行：
powershell
claude

### 验证 Claude Code

进入 Claude Code 后执行：
```
/status
再发一句：
```
用一句话回复：ok
第三方平台后台能看到调用记录，即配置成功。

## Mac 配置 Codex

Codex 使用 ~/.codex/config.toml 配置第三方 provider。

### 1. 设置 API Key 环境变量

bash
export THIRD_PARTY_API_KEY="YOUR_THIRD_PARTY_API_KEY"
如果要长期生效，写入 ~/.zshrc ：
bash
vi ~/.zshrc
追加：
bash
export THIRD_PARTY_API_KEY="YOUR_THIRD_PARTY_API_KEY"
生效：
bash
source ~/.zshrc

### 2. 编辑 Codex 配置

bash
mkdir -p ~/.codex
vi ~/.codex/config.toml

写入：
toml
model_provider = "third_party"
model = "YOUR_MODEL_NAME"
[model_providers.third_party]
name = "Third Party Platform"
base_url = "YOUR_OPENAI_COMPATIBLE_BASE_URL"
env_key = "THIRD_PARTY_API_KEY"
wire_api = "responses"
说明：
base_url
填第三方平台给 Codex / OpenAI-compatible 使用的地址。
env_key
必须等于前面设置的环境变量名。
wire_api = "responses"
适用于支持 Responses API 的第三方平台。
如果第三方平台给了专门的 Codex 配置示例，以平台示例为准。

### 3. 启动 Codex

bash
codex

### 4. 验证 Codex

发一个最小请求：
```
只回复 ok
第三方平台后台能看到调用记录，即配置成功。

## Windows 配置 Codex

以下命令在 PowerShell 中执行。

### 1. 设置 API Key 环境变量

powershell
[Environment]::SetEnvironmentVariable("THIRD_PARTY_API_KEY",
"YOUR_THIRD_PARTY_API_KEY", "User")

关闭 PowerShell，重新打开。
检查变量：
powershell
echo $env:THIRD_PARTY_API_KEY

### 2. 编辑 Codex 配置

powershell
New-Item -ItemType Directory -Force "$env:USERPROFILE\.codex"
notepad "$env:USERPROFILE\.codex\config.toml"
写入：
toml
model_provider = "third_party"
model = "YOUR_MODEL_NAME"
[model_providers.third_party]
name = "Third Party Platform"
base_url = "YOUR_OPENAI_COMPATIBLE_BASE_URL"
env_key = "THIRD_PARTY_API_KEY"
wire_api = "responses"
保存后重新打开 PowerShell。

### 3. 启动 Codex

powershell
codex

### 4. 验证 Codex

发一个最小请求：
```
只回复 ok
第三方平台后台能看到调用记录，即配置成功。

## WSL 单独说明

如果你在 Windows 的 WSL 里运行 claude 或 codex ，就按 Linux / Mac 的方式配置。
WSL 不会自动读取 PowerShell 里的用户环境变量。

路径也不同：
```
Windows PowerShell:
%USERPROFILE%\.claude\settings.json
%USERPROFILE%\.codex\config.toml
WSL:
~/.claude/settings.json
~/.codex/config.toml
在哪个环境运行工具，就在哪个环境配置 API Key 和配置文件。

## 常见错误

现象 处理
Claude Code 401 检查 ANTHROPIC_AUTH_TOKEN 或 ANTHROPIC_API_KEY 是否按平台文档
配置
Claude Code 404 检查 ANTHROPIC_BASE_URL 是否多写或少写 /v1
Claude Code 仍走官方账 运行 /status ，检查环境变量是否生效
号
Codex 401 检查 THIRD_PARTY_API_KEY 是否存在， env_key 是否写对
Codex 没走第三方平台 检查 ~/.codex/config.toml 的 model_provider 和 base_url
Windows 配置后不生效 关闭 PowerShell / IDE 后重新打开
WSL 配置后不生效 不要看 Windows 环境变量，在 WSL 内重新配置

## 最小检查清单

```
[ ] 已拿到第三方平台 API Key
[ ] 已拿到 Claude Code 使用的 Anthropic-compatible Base URL
[ ] 已拿到 Codex 使用的 OpenAI / Responses-compatible Base URL
[ ] 已拿到可用模型名
[ ] Claude Code 已设置 ANTHROPIC_BASE_URL
[ ] Claude Code 已设置 ANTHROPIC_AUTH_TOKEN 或 ANTHROPIC_API_KEY
[ ] Codex 已设置 THIRD_PARTY_API_KEY
[ ] Codex 已写入 ~/.codex/config.toml 或 Windows 用户目录 config.toml
[ ] 第三方平台后台能看到测试调用记录

## 参考资料

Claude Code LLM gateway configuration: https://code.claude.com/docs/en/llm-
gateway
Claude Code authentication: https://code.claude.com/docs/en/iam
Claude Code settings: https://code.claude.com/docs/en/settings
Codex config basics: https://developers.openai.com/codex/config-basic
Codex advanced configuration: https://developers.openai.com/codex/config-
advanced
AI聚合平台之八：别只卷模型单价，API Key AI聚合平台之九：一文讲清Token聚合平台的
权限、用量控制和调用日志分析才是企业级… 接口设计与实现逻辑
```
