<script setup lang="ts">
defineProps<{
  modelCount: number
}>()
</script>

<template>
  <section class="relative flex min-h-screen items-center justify-center overflow-hidden bg-home-canvas px-6 pb-0 pt-16 text-center">
    <HeroOrbitCanvas />

    <div class="relative z-[1] flex w-full flex-col items-center pt-[14px]">
      <div
        class="relative z-[1] mb-16 inline-flex items-center gap-[9px] rounded-full border border-[rgba(255,133,0,0.35)] bg-[rgba(255,133,0,0.1)] px-[18px] py-2 text-[13px] leading-[normal] text-brand-soft"
      >
        <span class="h-[7px] w-[7px] rounded-full bg-brand animate-[brand-pulse_2s_infinite]" />
        <span>已稳定接入 {{ modelCount }}+ 主流大模型 · 持续扩展中</span>
      </div>

      <h1
        class="relative z-[1] m-0 mb-8 flex flex-wrap items-center justify-center gap-4 text-[clamp(36px,5.5vw,70px)] font-black leading-[1.08] tracking-normal"
      >
        <span class="bg-gradient-to-br from-brand to-brand-soft bg-clip-text text-transparent [-webkit-text-fill-color:transparent]">智曜</span>
        <span class="text-ink">算力超市</span>
      </h1>

      <p class="relative z-[1] m-0 mb-11 max-w-[950px] text-[clamp(15px,2vw,22px)] leading-[1.75] text-hero-subtitle">
        一个接口，驾驭全部 AI 大模型，兼容 OpenAI 标准协议，无缝调用 DeepSeek、Kimi、MiniMax、豆包、千问、智谱等顶尖模型，一次集成，永久受益。
      </p>

      <div class="relative z-[1] mb-2 flex flex-wrap justify-center gap-[14px]">
        <NuxtLink
          to="/console"
          class="inline-flex min-h-11 items-center justify-center rounded-xl border-0 bg-gradient-to-br from-brand to-brand-soft px-[34px] py-[13px] text-[15px] font-bold leading-[normal] text-white shadow-[0_0_28px_rgba(255,133,0,0.38)] transition-all duration-[250ms] hover:-translate-y-[3px] hover:shadow-[0_8px_38px_rgba(255,133,0,0.5)]"
          :prefetch="false"
        >
          立即接入 →
        </NuxtLink>
        <NuxtLink
          to="/pricing"
          class="inline-flex min-h-11 items-center justify-center rounded-xl border border-[rgba(255,255,255,0.07)] bg-[rgba(255,133,0,0.15)] px-[34px] py-[13px] text-[15px] font-bold leading-[normal] text-brand transition-all duration-[250ms] hover:-translate-y-0.5 hover:border-brand hover:text-brand"
        >
          查看定价
        </NuxtLink>
      </div>

      <dl class="relative z-[1] m-0 mt-16 flex flex-wrap justify-center gap-[28px] p-0 min-[641px]:gap-[70px]">
        <div class="min-w-[92px]">
          <dt class="text-[38px] font-black leading-none text-brand">{{ modelCount }}+</dt>
          <dd class="m-0 mt-[5px] text-xs leading-[normal] text-stat-label">接入大模型</dd>
        </div>
        <div class="min-w-[92px]">
          <dt class="text-[38px] font-black leading-none text-brand">1</dt>
          <dd class="m-0 mt-[5px] text-xs leading-[normal] text-stat-label">统一 API 协议</dd>
        </div>
        <div class="min-w-[92px]">
          <dt class="text-[38px] font-black leading-none text-brand">99.9%</dt>
          <dd class="m-0 mt-[5px] text-xs leading-[normal] text-stat-label">服务可用率</dd>
        </div>
        <div class="min-w-[92px]">
          <dt class="text-[38px] font-black leading-none text-brand">&lt;50ms</dt>
          <dd class="m-0 mt-[5px] text-xs leading-[normal] text-stat-label">网关响应延迟</dd>
        </div>
      </dl>
    </div>
  </section>
</template>
