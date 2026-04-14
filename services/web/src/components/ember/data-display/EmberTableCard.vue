<script setup lang="ts">
import { computed, useSlots } from 'vue'

defineOptions({
  inheritAttrs: false
})

const props = withDefaults(defineProps<{
  data: unknown[]
  loading?: boolean
  headerCellStyle?: Record<string, string | number>
  footerClass?: string
}>(), {
  loading: false,
  headerCellStyle: () => ({
    background: '#f9fafb',
    color: '#6b7280',
    fontWeight: '600'
  }),
  footerClass: 'flex justify-end border-t border-gray-100 bg-gray-50/50 p-6'
})

const slots = useSlots()

const resolvedHeaderCellStyle = computed(() => props.headerCellStyle)
</script>

<template>
  <section class="overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm">
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
