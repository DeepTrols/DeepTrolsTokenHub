<script setup lang="ts">
import type { CSSProperties } from 'vue'

const props = withDefaults(
  defineProps<{
    icon?: string
    name: string
    size?: number
  }>(),
  {
    icon: '',
    size: 40,
  },
)

const baseURL = useRuntimeConfig().app.baseURL

const vendorIconFileByKey: Record<string, string> = {
  deepseek: 'deepseek-color.svg',
  doubao: 'doubao-color.svg',
  bytedance: 'doubao-color.svg',
  qwen: 'qwen-color.svg',
  tongyi: 'qwen-color.svg',
  minimax: 'minimax-color.svg',
  hunyuan: 'hunyuan-color.svg',
  tencent: 'hunyuan-color.svg',
  zhipu: 'zhipu-color.svg',
  glm: 'zhipu-color.svg',
  kimi: 'kimi-color.svg',
}

const monochromeIconFileByKey: Record<string, string> = {
  moonshot: 'moonshot.svg',
}

function normalizedIconKey(input?: string) {
  return (input || '').trim().split('.')[0]?.toLowerCase() || ''
}

const vendorKey = computed(() => normalizedIconKey(props.icon) || normalizedIconKey(props.name))
const initials = computed(() =>
  props.name
    .replace(/[^a-zA-Z0-9]/g, ' ')
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0] ?? '')
    .join('')
    .toUpperCase()
    .slice(0, 2),
)

const iconSrc = computed(() => {
  const file = vendorIconFileByKey[vendorKey.value]
  return file ? `${baseURL}vendor-icons/${file}` : ''
})

const monochromeIconSrc = computed(() => {
  const file = monochromeIconFileByKey[vendorKey.value]
  return file ? `${baseURL}vendor-icons/${file}` : ''
})

const rootStyle = computed<CSSProperties>(() => ({
  width: `${props.size}px`,
  height: `${props.size}px`,
  flex: `0 0 ${props.size}px`,
}))

const glyphStyle = computed<CSSProperties>(() => ({ fontSize: `${props.size * 0.38}px` }))
</script>

<template>
  <span class="inline-flex items-center justify-center text-white" :style="rootStyle" aria-hidden="true">
    <img v-if="iconSrc" :src="iconSrc" alt="" class="h-full w-full object-contain" draggable="false" />
    <img
      v-else-if="monochromeIconSrc"
      :src="monochromeIconSrc"
      alt=""
      class="h-full w-full object-contain opacity-95 brightness-0 invert"
      draggable="false"
    />
    <span v-else class="inline-flex h-full w-full items-center justify-center rounded-full bg-white/20 font-black text-white" :style="glyphStyle">
      {{ initials || '?' }}
    </span>
  </span>
</template>
