<script setup lang="ts">
import { getUniqueModels } from '~/data/catalog'

const models = getUniqueModels()
const featuredVendors = computed(() => models.filter((model) => model.tags || model.vendor_name).slice(0, 6))

type CopyCard = {
  icon: string
  title: string
  description: string
}

const features = [
  { icon: '↔', title: '统一 API 协议', description: '全面兼容 OpenAI API 格式，无需修改现有代码，接入一个地址即可使用所有主流大模型。' },
  { icon: '▣', title: '多模型聚合', description: `汇聚 DeepSeek、通义千问、豆包、Kimi、智谱等 ${models.length}+ 主流模型，按需选择性价比最优方案。` },
  { icon: '⌁', title: '高可用低延迟', description: '多节点分布式部署，智能路由自动故障转移，确保 99.9% 服务可用率与毫秒级网关延迟。' },
  { icon: '¥', title: '按量计费透明', description: '用多少付多少，无隐性费用，实时监控Token消耗与费用明细，轻松掌控每一分支出。' },
  { icon: '◇', title: '数据安全可控', description: 'API Key 级权限隔离，支持 IP 白名单与速率限制，密钥异地加密存储，全方位保护您的数据安全。' },
  { icon: '</>', title: '开发者友好', description: '即开即用，零配置门槛，提供完善的 API 文档与在线调试面板，快速完成模型接入与验证。' },
] satisfies CopyCard[]

const scenarios = [
  { icon: '＋', title: 'AI 对话应用', description: '构建智能客服、虚拟助手、教育辅导等对话类产品，快速集成大语言模型能力。' },
  { icon: '✎', title: '内容创作生成', description: '自动化生成文章、营销文案、代码注释、翻译文本，大幅提升内容生产效率。' },
  { icon: '⌂', title: '企业知识库', description: '结合 RAG 技术，搭建企业内部知识问答系统，精准检索与智能回答。' },
  { icon: '↗', title: '数据分析洞察', description: '利用大模型分析报告、日志、用户反馈，自动提取关键信息与趋势总结。' },
] satisfies CopyCard[]
</script>

<template>
  <main class="w-full min-h-screen overflow-x-hidden bg-home-canvas text-ink">
    <HomeHero :model-count="models.length" />
    <HomeMarquee :models="models" />

    <section class="bg-page px-6 py-20 max-[640px]:px-4 max-[640px]:py-14">
      <div class="mb-14 text-center">
        <h2 class="m-0 inline-block pb-4 text-[clamp(30px,4vw,48px)] font-extrabold leading-[normal] tracking-normal text-ink">
          <span class="text-brand">为什么选择</span>智曜TokenHub
        </h2>
        <p class="m-0 mx-auto max-w-[560px] text-lg leading-[1.6] text-ink-muted">一站式 AI 模型聚合平台，让开发者专注于应用创新</p>
      </div>

      <div class="grid w-full grid-cols-1 gap-5 px-6 max-[640px]:px-4 min-[641px]:grid-cols-2 min-[901px]:grid-cols-3">
        <FeatureCard v-for="feature in features" :key="feature.title" :icon="feature.icon" :title="feature.title" :description="feature.description" />
      </div>
    </section>

    <section
      class="border-y border-[rgba(255,133,0,0.1)] bg-[linear-gradient(180deg,transparent,rgba(255,133,0,0.03),transparent)] px-6 py-20 max-[640px]:px-4 max-[640px]:py-14"
    >
      <div class="mb-14 text-center">
        <h2 class="m-0 inline-block pb-4 text-[clamp(30px,4vw,48px)] font-extrabold leading-[normal] tracking-normal text-ink">
          <span class="text-brand">接入模型</span>供应商
        </h2>
        <p class="m-0 mx-auto max-w-[560px] text-lg leading-[1.6] text-ink-muted">已接入多款热门 AI 大模型，覆盖全球顶尖大模型服务</p>
      </div>

      <div class="grid w-full grid-cols-1 gap-6 px-6 max-[640px]:px-4 min-[641px]:grid-cols-2 min-[1025px]:grid-cols-3">
        <VendorCard v-for="model in featuredVendors" :key="model.model_name" :model="model" />
      </div>
    </section>

    <section
      class="border-y border-[rgba(255,133,0,0.1)] bg-[linear-gradient(180deg,transparent,rgba(255,133,0,0.03),transparent)] px-6 py-20 max-[640px]:px-4 max-[640px]:py-14"
    >
      <div class="mb-14 text-center">
        <h2 class="m-0 inline-block pb-4 text-[clamp(30px,4vw,48px)] font-extrabold leading-[normal] tracking-normal text-ink">
          <span class="text-brand">三步完成</span>接入
        </h2>
        <p class="m-0 mx-auto max-w-[560px] text-lg leading-[1.6] text-ink-muted">从注册到调用，只需三分钟</p>
      </div>

      <div class="mb-[60px] flex w-full flex-wrap items-start justify-center gap-4 px-6 max-[640px]:gap-3">
        <StepCard step="01" title="注册获取 Key" description="注册账号后，在控制台创建 API Key，获得专属的接口密钥。" />
        <span class="flex items-center justify-center pt-[58px] text-[32px] text-brand-line-active max-[640px]:hidden">→</span>
        <StepCard step="02" title="模型配置" description="在模型广场选择需要的模型，配置额度与速率限制，即可开始使用。" />
        <span class="flex items-center justify-center pt-[58px] text-[32px] text-brand-line-active max-[640px]:hidden">→</span>
        <StepCard step="03" title="开始调用" description="使用标准 OpenAI SDK 替换 Base URL 即可调用，支持 Chat Completion、文生图、TTS 等全部能力。" />
      </div>

      <div class="mx-auto max-w-[640px] overflow-hidden rounded-xl border border-[rgba(255,133,0,0.2)] bg-code-panel">
        <div class="flex h-[42px] items-center gap-2 border-b border-[rgba(255,255,255,0.08)] bg-[rgba(255,255,255,0.04)] px-4">
          <span class="h-2.5 w-2.5 rounded-full bg-[#ff5f57]" />
          <span class="h-2.5 w-2.5 rounded-full bg-[#febc2e]" />
          <span class="h-2.5 w-2.5 rounded-full bg-[#28c840]" />
          <strong class="ml-auto text-xs leading-[normal] text-ink-muted">Python</strong>
        </div>
        <pre class="m-0 overflow-x-auto px-6 py-5 font-mono text-[13px] leading-[1.7] text-code-text"><code>from openai import OpenAI

client = OpenAI(
    api_key="sk-your-api-key",
    base_url="https://api.opcstore.com/v1"
)

response = client.chat.completions.create(
    model="deepseek-chat",
    messages=[{"role": "user", "content": "你好"}]
)
print(response.choices[0].message.content)</code></pre>
      </div>
    </section>

    <section class="bg-page px-6 py-20 max-[640px]:px-4 max-[640px]:py-14">
      <div class="mb-14 text-center">
        <h2 class="m-0 inline-block pb-4 text-[clamp(30px,4vw,48px)] font-extrabold leading-[normal] tracking-normal text-ink">
          <span class="text-brand">适用场景</span>
        </h2>
        <p class="m-0 mx-auto max-w-[560px] text-lg leading-[1.6] text-ink-muted">覆盖 AI 应用开发的各个环节</p>
      </div>

      <div class="grid w-full grid-cols-1 gap-4 px-6 max-[640px]:px-4 min-[901px]:grid-cols-2">
        <ScenarioCard v-for="scenario in scenarios" :key="scenario.title" :icon="scenario.icon" :title="scenario.title" :description="scenario.description" />
      </div>
    </section>

    <section class="relative overflow-hidden border-t border-[rgba(255,133,0,0.14)] bg-cta-canvas px-6 py-24 text-center">
      <div class="absolute bottom-[-180px] right-1/2 h-[480px] w-[480px] translate-x-1/2 rounded-full bg-[rgba(255,133,0,0.18)] blur-[80px]" />
      <div class="relative z-[1] m-0 w-full">
        <h2 class="m-0 mb-[18px] text-[40px] font-black leading-[normal] text-ink">准备好开始了吗？</h2>
        <p class="m-0 mb-[30px] text-[17px] leading-[1.7] text-ink-muted">登录可接入 {{ models.length }}+ 主流 AI 大模型，体验一站式算力超市的便捷。</p>
        <NuxtLink
          to="/console"
          class="inline-flex min-h-11 items-center justify-center rounded-xl border-0 bg-gradient-to-br from-brand to-brand-soft px-[34px] py-[13px] text-[15px] font-bold leading-[normal] text-white shadow-[0_0_28px_rgba(255,133,0,0.38)] transition-all duration-[250ms] hover:-translate-y-[3px] hover:shadow-[0_8px_38px_rgba(255,133,0,0.5)]"
          :prefetch="false"
        >
          开始使用 →
        </NuxtLink>
      </div>
    </section>
  </main>
</template>
