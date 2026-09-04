<script setup lang="ts">
const logoSrc = `${useRuntimeConfig().app.baseURL}logo.png`

withDefaults(
  defineProps<{
    compact?: boolean
  }>(),
  {
    compact: false,
  },
)
</script>

<template>
  <NuxtLink
    to="/"
    class="group flex items-center gap-header-brand-gap text-white"
    aria-label="智曜TokenHub 首页"
  >
    <img
      :src="logoSrc"
      alt=""
      class="h-header-logo w-header-logo shrink-0 rounded-brand-logo object-contain transition-transform duration-200 group-hover:scale-110"
    />
    <span v-if="!compact" class="whitespace-nowrap text-[15px] font-bold leading-none max-sm:hidden">
      智曜TokenHub
    </span>
  </NuxtLink>
</template>
