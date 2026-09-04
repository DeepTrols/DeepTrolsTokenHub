<script setup lang="ts">
defineProps<{
  icon: string
  title: string
  description: string
}>()
</script>

<template>
  <article
    class="flex gap-[18px] rounded-2xl border border-brand-line-hover bg-transparent p-6 transition-all duration-300 hover:-translate-y-1 hover:border-brand-line-active hover:shadow-[0_8px_32px_rgba(255,133,0,0.1)]"
  >
    <div class="flex h-[60px] w-[60px] shrink-0 items-center justify-center rounded-[10px] bg-brand-light text-[22px] font-extrabold leading-[normal] text-brand">
      {{ icon }}
    </div>
    <div>
      <h3 class="m-0 text-lg font-extrabold leading-[normal] text-brand">{{ title }}</h3>
      <p class="m-0 text-[15px] leading-[1.65] text-ink-muted">{{ description }}</p>
    </div>
  </article>
</template>
