<script setup lang="ts">
import { computed, useSlots } from 'vue'

defineOptions({
  inheritAttrs: false
})

const props = withDefaults(defineProps<{
  data: unknown[]
  loading?: boolean
  headerCellStyle?: Record<string, string | number>
  headerClass?: string
  footerClass?: string
}>(), {
  loading: false,
  headerCellStyle: () => ({
    background: '#f9fafb',
    color: '#6b7280',
    fontWeight: '600'
  }),
  headerClass: 'border-b border-gray-100 px-5 py-4',
  footerClass: 'flex justify-end border-t border-gray-100 bg-gray-50/50 p-6'
})

const slots = useSlots()

const hasHeader = computed(() => Boolean(slots.header))
const resolvedHeaderCellStyle = computed(() => props.headerCellStyle)
</script>

<template>
  <section class="overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm">
    <div v-if="hasHeader" :class="props.headerClass">
      <slot name="header" />
    </div>

    <el-table
      :data="props.data"
      v-loading="props.loading"
      class="w-full"
      :header-cell-style="resolvedHeaderCellStyle"
      v-bind="$attrs"
    >
      <slot />
    </el-table>

    <div v-if="slots.pagination" :class="props.footerClass">
      <slot name="pagination" />
    </div>
  </section>
</template>
