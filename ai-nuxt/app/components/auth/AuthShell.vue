<script setup lang="ts">
type LoginFeatureItem = {
  icon: string
  title: string
  description: string
}

const logoSrc = `${useRuntimeConfig().app.baseURL}logo.png`

const features = [
  { icon: '◈', title: '多模型聚合', description: '一站式接入主流大语言模型，统一管理高效便捷' },
  { icon: '▣', title: '安全加密', description: '端到端加密传输，API Key 安全隔离，保障数据隐私' },
  { icon: '☉', title: '高可用架构', description: '多通道故障自动切换，智能负载均衡，服务稳定在线' },
  { icon: '◎', title: '统一接口', description: '标准化 API 格式，无需适配即可灵活切换多模型' },
  { icon: '▤', title: '灵活计费', description: '按量付费零门槛，覆盖个人开发者到企业级需求' },
] satisfies LoginFeatureItem[]
</script>

<template>
  <main
    class="relative flex w-full min-h-screen items-center justify-center overflow-x-hidden bg-auth-page px-5 pb-14 pt-24 text-ink before:absolute before:inset-0 before:bg-[radial-gradient(circle_at_24%_18%,rgba(255,133,0,0.12),transparent_18%),radial-gradient(circle_at_78%_38%,rgba(255,133,0,0.11),transparent_20%),linear-gradient(90deg,rgba(4,5,9,0.68),rgba(4,5,9,0.5))] before:content-['']"
  >
    <MatrixBackground />

    <section
      class="relative z-[1] grid min-h-[498px] w-[min(960px,100%)] grid-cols-1 overflow-hidden rounded-[24px] border border-[rgba(255,255,255,0.13)] bg-auth-shell shadow-[0_24px_90px_rgba(0,0,0,0.46)] backdrop-blur-[18px] min-[901px]:grid-cols-[45%_55%]"
      aria-label="账号访问"
    >
      <aside class="hidden min-[901px]:block">
        <div class="relative flex h-full w-full flex-col overflow-hidden bg-auth-intro px-8 py-10">
          <div class="pointer-events-none absolute inset-0 overflow-hidden">
            <div class="absolute -right-[110px] -top-20 h-[360px] w-[360px] rounded-full bg-auth-intro-orb-strong" />
            <div class="absolute bottom-[-130px] right-[60px] h-[300px] w-[300px] rounded-full bg-auth-intro-orb-soft" />
          </div>

          <div class="relative z-10 mb-14 flex items-center gap-4">
            <img
              :src="logoSrc"
              alt=""
              class="h-auth-brand-logo w-auth-brand-logo rounded-auth-brand-logo object-contain"
            />
            <div class="flex flex-col gap-auth-brand-title-gap">
              <h1 class="m-0 text-auth-brand-title font-black text-white">智曜TokenHub</h1>
              <p class="m-0 text-auth-brand-subtitle text-auth-brand-subtitle-color">欢迎回来，请登录您的账户</p>
            </div>
          </div>

          <div class="relative z-10 flex flex-col gap-[18px]">
            <LoginFeature
              v-for="feature in features"
              :key="feature.title"
              :icon="feature.icon"
              :title="feature.title"
              :description="feature.description"
            />
          </div>
        </div>
      </aside>

      <slot />
    </section>
  </main>
</template>
