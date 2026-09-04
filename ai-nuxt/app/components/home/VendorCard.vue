<script setup lang="ts">
import type { EnrichedModelRecord } from '~/types/catalog'
import { vendorGradient } from '~/data/catalog'

const props = defineProps<{
  model: EnrichedModelRecord
}>()

const gradient = computed(() => vendorGradient(props.model.vendor_icon))
const vendorLabel = computed(() => props.model.tags?.split(/[,;，；|]/)[0]?.trim() || props.model.vendor_name || props.model.model_name)
</script>

<template>
  <NuxtLink
    to="/pricing"
    class="flex flex-col overflow-hidden rounded-2xl border border-semi-line bg-surface transition-all duration-300 hover:-translate-y-1 hover:border-brand-line-active hover:shadow-[0_8px_32px_rgba(255,133,0,0.1)]"
  >
    <div class="relative flex min-h-[200px] flex-col items-center justify-center gap-3 px-5 py-7" :style="{ background: gradient }">
      <div class="flex items-center justify-center gap-[14px]">
        <ModelIcon :name="model.model_name" :icon="model.vendor_icon" :size="50" />
        <span class="break-words text-center text-[clamp(20px,2.8vw,30px)] font-extrabold leading-[1.25] text-white [text-shadow:0_2px_10px_rgba(0,0,0,0.25)]">
          {{ vendorLabel }}
        </span>
      </div>
      <span
        v-if="model.vendor_name"
        class="absolute bottom-2.5 right-3 inline-flex rounded-md bg-[rgba(0,0,0,0.25)] px-3 py-1 text-[10px] font-bold uppercase leading-[1.5] tracking-[0.6px] text-[rgba(255,255,255,0.86)] backdrop-blur-[4px]"
      >
        {{ model.vendor_name }}
      </span>
    </div>
    <div class="flex flex-col gap-2 px-[22px] pb-6 pt-5">
      <p class="m-0 text-[15px] leading-[1.65] text-ink-muted">{{ model.description }}</p>
    </div>
  </NuxtLink>
</template>
