<script setup lang="ts">
defineProps<{
  step: string
  title: string
  description: string
}>()
</script>

<template>
  <article
    class="min-w-[220px] flex-1 rounded-2xl border border-brand-line-hover bg-transparent px-7 py-8 text-center transition-all duration-300 hover:-translate-y-1 hover:border-brand-line-active hover:shadow-[0_8px_32px_rgba(255,133,0,0.1)] max-[640px]:max-w-full max-[640px]:min-w-full"
  >
    <div class="bg-gradient-to-br from-brand to-brand-soft bg-clip-text text-[42px] font-black leading-[normal] text-transparent [-webkit-text-fill-color:transparent]">
      {{ step }}
    </div>
    <h3 class="m-0 text-lg font-extrabold leading-[normal] text-brand">{{ title }}</h3>
    <p class="m-0 text-[15px] leading-[1.65] text-ink-muted">{{ description }}</p>
  </article>
</template>
