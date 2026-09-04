<script setup lang="ts">
const props = defineProps<{
  currentPage: number
  pageSize: number
  total: number
}>()

const emit = defineEmits<{
  pageChange: [page: number]
  pageSizeChange: [pageSize: number]
}>()

const pageSizeOptions = [10, 20, 50, 100]
const pageCount = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))
const pagerButtonClass =
  'flex h-8 w-7 cursor-pointer items-center justify-center rounded-[10px] border-0 bg-control text-[30px] leading-[normal] text-ink-subtle disabled:cursor-default disabled:opacity-[0.35]'

function previous() {
  if (props.currentPage > 1) {
    emit('pageChange', props.currentPage - 1)
  }
}

function next() {
  if (props.currentPage < pageCount.value) {
    emit('pageChange', props.currentPage + 1)
  }
}

function changePageSize(event: Event) {
  emit('pageSizeChange', Number((event.target as HTMLSelectElement).value))
}
</script>

<template>
  <div class="mt-6 flex w-full items-center justify-center gap-3 border-t border-semi-line py-4 text-ink-muted">
    <button type="button" :class="pagerButtonClass" :disabled="currentPage <= 1" aria-label="上一页" @click="previous">‹</button>
    <span class="flex h-8 min-w-8 items-center justify-center rounded-[10px] border-0 bg-[rgba(255,133,0,0.2)] font-extrabold leading-[normal] text-brand">
      {{ currentPage }}
    </span>
    <button type="button" :class="pagerButtonClass" :disabled="currentPage >= pageCount" aria-label="下一页" @click="next">›</button>

    <label class="flex h-8 items-center justify-center gap-1 rounded-[10px] border-0 bg-control px-2.5 text-sm leading-[normal] text-ink">
      <span>每页条数：</span>
      <select class="border-0 bg-transparent text-ink outline-none" :value="pageSize" @change="changePageSize">
        <option v-for="option in pageSizeOptions" :key="option" :value="option">{{ option }}</option>
      </select>
    </label>
  </div>
</template>
