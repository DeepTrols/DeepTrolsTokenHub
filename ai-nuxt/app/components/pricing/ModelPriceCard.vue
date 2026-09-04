<script setup lang="ts">
import { priceSummary, vendorGradient } from '~/data/catalog'
import type { EnrichedModelRecord } from '~/types/catalog'

const props = defineProps<{
  model: EnrichedModelRecord
}>()

const emit = defineEmits<{
  select: [model: EnrichedModelRecord]
}>()

const summary = computed(() => priceSummary(props.model))
const gradient = computed(() => vendorGradient(props.model.vendor_icon))
const vendorTag = computed(() => {
  const label = props.model.vendor_name || props.model.vendor_icon.split('.')[0] || props.model.model_name
  return /^[a-z0-9 -]+$/i.test(label) ? label.toUpperCase() : label
})

function selectModel() {
  emit('select', props.model)
}
</script>

<template>
  <button
    type="button"
    class="flex h-60 w-full cursor-pointer flex-col overflow-hidden rounded-2xl border border-semi-line bg-price-card text-left font-sans text-ink no-underline transition-all duration-300 hover:-translate-y-1 hover:border-brand-line-active hover:shadow-[0_12px_40px_rgba(255,133,0,0.18)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand"
    @click="selectModel"
  >
    <div class="relative flex min-h-[120px] flex-col justify-center gap-1.5 px-4 py-[14px]" :style="{ background: gradient }">
      <div class="relative flex w-full min-w-0 items-center gap-2">
        <ModelIcon :name="model.model_name" :icon="model.vendor_icon" :size="40" />
        <h2
          class="m-0 flex-1 truncate text-xl font-extrabold leading-tight text-white"
          :title="model.model_name"
        >
          {{ model.model_name }}
        </h2>
      </div>
      <span
        class="absolute right-3 top-3 inline-flex rounded-md bg-[rgba(0,0,0,0.1)] px-3 py-1 text-[10px] font-semibold uppercase leading-[1.5] tracking-[0.6px] text-[rgba(255,255,255,0.82)] backdrop-blur-[4px]"
      >
        {{ vendorTag }}
      </span>
    </div>

    <div class="flex flex-col gap-1 px-4 py-3">
      <dl class="m-0 text-xs leading-[normal] text-ink-muted">
        <div class="my-2 flex items-center justify-between gap-2.5">
          <dt class="m-0">输入价格</dt>
          <dd class="m-0 whitespace-nowrap font-semibold text-ink">{{ summary.inputPrice }}{{ summary.suffix }}</dd>
        </div>
        <div class="my-2 flex items-center justify-between gap-2.5">
          <dt class="m-0">补全价格</dt>
          <dd class="m-0 whitespace-nowrap font-semibold text-ink">{{ summary.completionPrice }}{{ summary.suffix }}</dd>
        </div>
      </dl>

      <p
        class="m-0 mt-1 overflow-hidden text-xs leading-tight text-ink-subtle [display:-webkit-box] [-webkit-box-orient:vertical] [-webkit-line-clamp:2]"
        :title="model.description"
      >
        {{ model.description }}
      </p>
    </div>
  </button>
</template>
