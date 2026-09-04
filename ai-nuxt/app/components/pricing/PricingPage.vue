<script setup lang="ts">
import { getUniqueModels } from '~/data/catalog'
import type { EnrichedModelRecord } from '~/types/catalog'

const models = getUniqueModels()
const currentPage = ref(1)
const pageSize = ref(50)
const selectedModel = ref<EnrichedModelRecord | null>(null)

const pagedModels = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return models.slice(start, start + pageSize.value)
})

function setPage(page: number) {
  currentPage.value = page
}

function setPageSize(size: number) {
  pageSize.value = size
  currentPage.value = 1
}

function openModelDetail(model: EnrichedModelRecord) {
  selectedModel.value = model
}

function closeModelDetail() {
  selectedModel.value = null
}
</script>

<template>
  <main class="w-full min-h-screen overflow-x-hidden bg-page pt-16 text-ink">
    <div class="w-full px-4 pb-14 pt-4">
      <section
        class="grid w-full grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4"
        aria-label="模型定价"
      >
        <ModelPriceCard v-for="model in pagedModels" :key="model.model_name" :model="model" @select="openModelDetail" />
      </section>

      <PricingPagination
        :current-page="currentPage"
        :page-size="pageSize"
        :total="models.length"
        @page-change="setPage"
        @page-size-change="setPageSize"
      />
    </div>

    <ModelDetailModal :open="selectedModel !== null" :model="selectedModel" @close="closeModelDetail" />
  </main>
</template>
