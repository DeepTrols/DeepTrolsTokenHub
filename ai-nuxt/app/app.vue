<script setup lang="ts">
useHead({
  htmlAttrs: {
    class: 'dark',
  },
})
</script>

<template>
  <div class="h-screen w-full min-h-screen min-w-[320px] overflow-y-auto overflow-x-hidden bg-page font-sans leading-[normal] text-ink antialiased [scrollbar-width:none] [-ms-overflow-style:none] [&::-webkit-scrollbar]:hidden">
    <NuxtRouteAnnouncer />
    <AppHeader />
    <NuxtPage />
  </div>
</template>
