<script setup lang="ts">
defineProps<{
  icon: string
  title: string
  description: string
}>()
</script>

<template>
  <article class="group flex cursor-default items-start gap-3">
    <span
      class="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-auth-feature-icon bg-auth-feature-icon-bg text-[15px] font-extrabold leading-[normal] text-white transition-transform duration-300 group-hover:scale-110"
    >
      {{ icon }}
    </span>
    <div>
      <h3 class="m-0 mb-1 text-[15px] font-extrabold leading-[normal] text-white">{{ title }}</h3>
      <p class="m-0 text-sm leading-[1.45] text-auth-feature-desc">{{ description }}</p>
    </div>
  </article>
</template>
