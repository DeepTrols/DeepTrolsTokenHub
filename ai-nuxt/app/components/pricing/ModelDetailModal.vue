<script setup lang="ts">
import { modelEndpointItems, modelPricingGroups, modelTags, vendorGradient } from '~/data/catalog'
import type { EnrichedModelRecord } from '~/types/catalog'

const props = defineProps<{
  open: boolean
  model: EnrichedModelRecord | null
}>()

const emit = defineEmits<{
  close: []
}>()

const gradient = computed(() => (props.model ? vendorGradient(props.model.vendor_icon) : ''))
const tags = computed(() => (props.model ? modelTags(props.model) : []))
const endpoints = computed(() => (props.model ? modelEndpointItems(props.model) : []))
const pricingGroups = computed(() => (props.model ? modelPricingGroups(props.model) : []))
const copied = ref(false)
let copyResetTimer: ReturnType<typeof setTimeout> | undefined

const description = computed(() => {
  if (!props.model) {
    return '暂无模型描述'
  }

  if (props.model.description) {
    return props.model.description
  }

  return props.model.vendor_description ? `供应商信息：${props.model.vendor_description}` : '暂无模型描述'
})

function close() {
  emit('close')
}

async function copyModelName() {
  if (!props.model?.model_name || typeof navigator === 'undefined') {
    return
  }

  try {
    await navigator.clipboard?.writeText(props.model.model_name)
    copied.value = true
    if (copyResetTimer) {
      clearTimeout(copyResetTimer)
    }
    copyResetTimer = setTimeout(() => {
      copied.value = false
    }, 1200)
  } catch {}
}

function tagTone(tag: string) {
  const tones = [
    'border-amber-400/20 bg-amber-500/15 text-amber-100',
    'border-sky-400/20 bg-sky-500/15 text-sky-100',
    'border-emerald-400/20 bg-emerald-500/15 text-emerald-100',
    'border-violet-400/20 bg-violet-500/15 text-violet-100',
    'border-pink-400/20 bg-pink-500/15 text-pink-100',
    'border-cyan-400/20 bg-cyan-500/15 text-cyan-100',
  ]
  const index = [...tag].reduce((sum, char) => sum + char.charCodeAt(0), 0) % tones.length

  return tones[index]
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && props.open) {
    close()
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeydown)
  if (copyResetTimer) {
    clearTimeout(copyResetTimer)
  }
})
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="fixed inset-0 z-[1000] flex overflow-auto bg-[var(--semi-color-overlay-bg)] px-4 py-8"
      @click.self="close"
    >
      <section
        role="dialog"
        aria-modal="true"
        :aria-label="model?.model_name || '模型详情'"
        class="relative m-auto max-h-[calc(100vh-64px)] w-full max-w-[560px] overflow-hidden rounded-[var(--semi-border-radius-large)] border border-semi-line bg-[var(--semi-color-bg-2)] text-ink shadow-[var(--semi-shadow-elevated)]"
      >
        <button
          type="button"
          class="absolute right-2.5 top-2.5 z-[1] flex h-8 w-8 items-center justify-center rounded-full border-0 bg-transparent text-2xl leading-none text-white transition-colors hover:bg-white/10 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white"
          aria-label="关闭"
          title="关闭"
          @click="close"
        >
          ×
        </button>

        <div class="flex min-h-[160px] flex-col items-center justify-center px-6 py-6 text-center" :style="{ background: gradient }">
          <div class="mb-3 flex h-16 w-16 items-center justify-center rounded-2xl bg-white/20">
            <ModelIcon v-if="model" :name="model.model_name" :icon="model.icon || model.vendor_icon" :size="48" />
          </div>

          <button
            type="button"
            class="group inline-flex max-w-full items-center justify-center gap-1.5 border-0 bg-transparent p-0 text-xl font-bold leading-[normal] text-white [text-shadow:0_2px_8px_rgba(0,0,0,0.2)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white"
            :aria-label="model?.model_name ? `复制 ${model.model_name}` : '未知模型'"
            :title="model?.model_name ? '复制模型名称' : undefined"
            @click.stop="copyModelName"
          >
            <span class="truncate">{{ model?.model_name || '未知模型' }}</span>
            <span v-if="model?.model_name" class="shrink-0 text-[13px] leading-none text-white/75 transition-colors group-hover:text-white">
              {{ copied ? '✓' : '⧉' }}
            </span>
          </button>

          <span
            v-if="model?.vendor_name"
            class="mt-2 rounded-full bg-black/15 px-3 py-1 text-xs font-medium leading-[normal] text-white/90 backdrop-blur-[4px]"
          >
            {{ model.vendor_name }}
          </span>
        </div>

        <div class="max-h-[calc(100vh-224px)] overflow-y-auto p-4">
          <section class="pb-2 pt-4 text-ink-muted">
            <p class="m-0 mb-3 text-sm leading-[1.6]">{{ description }}</p>
            <div v-if="tags.length" class="flex flex-wrap gap-1.5">
              <span v-for="tag in tags" :key="tag" :class="['rounded-full border px-2.5 py-1 text-xs leading-[normal]', tagTone(tag)]">
                {{ tag }}
              </span>
            </div>
          </section>

          <section class="border-t border-dashed border-semi-line py-4">
            <div v-if="endpoints.length" class="text-sm leading-[1.5] text-ink">
              <div
                v-for="endpoint in endpoints"
                :key="endpoint.key"
                class="flex justify-between gap-4 border-b border-dashed border-semi-line py-2 last:border-b-0 last:pb-0"
              >
                <span class="flex min-w-0 items-center pr-5">
                  <span class="mr-2 h-1.5 w-1.5 shrink-0 rounded-full bg-[#52c41a]" />
                  <span class="shrink-0">{{ endpoint.label }}</span>
                  <span v-if="endpoint.path">：</span>
                  <span v-if="endpoint.path" class="break-all text-ink-subtle">{{ endpoint.path }}</span>
                </span>
                <span class="shrink-0 text-xs leading-[normal] text-ink-subtle">{{ endpoint.method }}</span>
              </div>
            </div>
            <p v-else class="m-0 text-sm leading-[1.6] text-ink-muted">暂无接口端点信息</p>
          </section>

          <section class="pb-2 pt-4">
            <div class="overflow-hidden">
              <table class="w-full border-collapse text-left text-sm leading-[1.5]">
                <thead class="text-ink">
                  <tr class="border-b border-semi-line">
                    <th class="px-3 py-2 font-semibold">分组</th>
                    <th class="px-3 py-2 font-semibold">计费类型</th>
                    <th class="px-3 py-2 font-semibold">价格摘要</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="group in pricingGroups" :key="group.key" class="border-b border-semi-line last:border-b-0">
                    <td class="px-3 py-3 align-top">
                      <span class="inline-flex rounded-full border border-white/10 bg-white/5 px-2.5 py-1 text-xs leading-[normal] text-ink">{{ group.label }}</span>
                    </td>
                    <td class="px-3 py-3 align-top">
                      <span class="inline-flex rounded-full bg-brand-light px-2.5 py-1 text-xs leading-[normal] text-brand">{{ group.billingType }}</span>
                    </td>
                    <td class="space-y-1 px-3 py-3 align-top">
                      <div v-for="item in group.priceItems" :key="item.key">
                        <div class="font-semibold text-[var(--semi-color-warning)]">{{ item.label }} {{ item.value }}</div>
                        <div class="text-xs leading-[normal] text-ink-subtle">{{ item.suffix }}</div>
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>
        </div>
      </section>
    </div>
  </Teleport>
</template>
