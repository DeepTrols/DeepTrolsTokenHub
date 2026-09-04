<script setup lang="ts">
import type { EnrichedModelRecord } from '~/types/catalog'

const props = defineProps<{
  models: EnrichedModelRecord[]
}>()

const firstRow = computed(() => props.models.slice(0, Math.ceil(props.models.length / 2)))
const secondRow = computed(() => props.models.slice(Math.ceil(props.models.length / 2)))
</script>

<template>
  <section
    class="overflow-hidden border-y border-[rgba(255,133,0,0.18)] bg-[linear-gradient(180deg,transparent,rgba(255,133,0,0.02),transparent)] py-[52px]"
  >
    <div class="mb-7 text-center text-base leading-[normal] tracking-[2px] text-ink-muted">✦ 接入全球顶尖大模型厂商，持续增加中 ✦</div>

    <div class="group mb-[14px] overflow-hidden">
      <div class="flex w-max gap-4 animate-[marquee-left_32s_linear_infinite] group-hover:[animation-play-state:paused]">
        <NuxtLink
          v-for="(model, index) in [...firstRow, ...firstRow]"
          :key="`first-${model.model_name}-${index}`"
          to="/pricing"
          class="flex items-center gap-2.5 whitespace-nowrap rounded-full border border-[rgba(255,133,0,0.35)] bg-transparent px-[18px] py-[9px] text-[13px] font-semibold leading-[normal] text-ink transition-all duration-300 hover:scale-105 hover:border-brand hover:bg-[rgba(255,133,0,0.1)] hover:text-brand hover:shadow-[0_0_20px_rgba(255,133,0,0.35)]"
        >
          <ModelIcon :name="model.model_name" :icon="model.vendor_icon" :size="18" />
          <span>{{ model.model_name }}</span>
        </NuxtLink>
      </div>
    </div>

    <div class="group mb-[14px] overflow-hidden">
      <div class="flex w-max gap-4 animate-[marquee-right_38s_linear_infinite] group-hover:[animation-play-state:paused]">
        <NuxtLink
          v-for="(model, index) in [...secondRow, ...secondRow]"
          :key="`second-${model.model_name}-${index}`"
          to="/pricing"
          class="flex items-center gap-2.5 whitespace-nowrap rounded-full border border-[rgba(255,133,0,0.35)] bg-transparent px-[18px] py-[9px] text-[13px] font-semibold leading-[normal] text-ink transition-all duration-300 hover:scale-105 hover:border-brand hover:bg-[rgba(255,133,0,0.1)] hover:text-brand hover:shadow-[0_0_20px_rgba(255,133,0,0.35)]"
        >
          <ModelIcon :name="model.model_name" :icon="model.vendor_icon" :size="18" />
          <span>{{ model.model_name }}</span>
        </NuxtLink>
      </div>
    </div>
  </section>
</template>
