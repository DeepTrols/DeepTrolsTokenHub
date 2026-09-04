<script setup lang="ts">
const route = useRoute()

const navItems = [
  { label: '首页', to: '/' },
  { label: '模型超市', to: '/pricing' },
  { label: '控制中心', to: '/console' },
] as const

const navLinkClass =
  'flex shrink-0 items-center gap-1 rounded-md p-2 text-base font-semibold transition-colors duration-200 ease-in-out hover:text-brand max-sm:text-sm'

function isActive(path: string) {
  if (path === '/') {
    return route.path === '/'
  }

  return route.path.startsWith(path)
}

function navLinkTone(item: (typeof navItems)[number]) {
  return isActive(item.to) ? 'text-brand' : 'text-white'
}
</script>

<template>
  <header
    class="fixed left-0 top-0 z-[100] w-full bg-transparent p-0 leading-[normal] text-ink transition-all duration-500"
  >
    <div class="w-full px-3 md:px-4">
      <div class="relative flex h-10 items-center justify-between md:h-16">
        <div class="flex min-w-0 items-center">
          <BrandMark />
        </div>

        <nav
          class="mx-2 flex min-w-0 flex-1 items-center justify-center gap-2 overflow-x-auto whitespace-nowrap sm:gap-4 md:mx-4 lg:gap-8"
          aria-label="主导航"
        >
          <NuxtLink
            v-for="item in navItems"
            :key="item.label"
            :to="item.to"
            :class="[navLinkClass, navLinkTone(item)]"
            :prefetch="false"
          >
            <span>{{ item.label }}</span>
          </NuxtLink>
        </nav>

        <div class="flex min-w-0 items-center gap-2 md:gap-3">
          <NuxtLink
            to="/console"
            class="flex h-8 min-w-[76px] items-center justify-center rounded-md border-0 bg-gradient-to-br from-brand to-brand-soft px-3 text-[13px] font-bold leading-[normal] text-white shadow-[0_6px_18px_rgba(255,133,0,0.32)] transition-colors duration-200 hover:text-white sm:min-w-[88px] sm:px-4 sm:text-sm"
            :prefetch="false"
          >
            控制中心
          </NuxtLink>
        </div>
      </div>
    </div>
  </header>
</template>
