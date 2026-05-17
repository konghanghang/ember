<script setup lang="ts">
import { computed } from 'vue'
import { buildInfo } from '@/utils/buildInfo'

withDefaults(defineProps<{
  showCommit?: boolean
}>(), {
  showCommit: true,
})

const commitLinkLabel = computed(() => (
  buildInfo.commitSha
    ? `查看当前构建提交 ${buildInfo.shortCommitSha}`
    : '打开 GitHub 仓库'
))
</script>

<template>
  <div class="inline-flex items-center gap-2 text-xs font-medium text-gray-500">
    <a
      :href="buildInfo.repositoryUrl"
      target="_blank"
      rel="noreferrer"
      aria-label="打开 GitHub 仓库"
      title="GitHub 仓库"
      class="inline-flex h-9 w-9 items-center justify-center rounded-full text-gray-500 transition-colors hover:bg-gray-100 hover:text-ember cursor-pointer"
    >
      <svg class="h-4 w-4" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
        <path d="M8 0C3.58 0 0 3.67 0 8.2c0 3.63 2.29 6.7 5.47 7.78.4.08.55-.18.55-.4 0-.2-.01-.86-.01-1.56-2.01.38-2.53-.5-2.69-.96-.09-.24-.48-.96-.82-1.16-.28-.16-.68-.56-.01-.57.63-.01 1.08.59 1.23.84.72 1.24 1.87.89 2.33.68.07-.53.28-.89.51-1.09-1.78-.21-3.64-.91-3.64-4.04 0-.89.31-1.62.82-2.19-.08-.21-.36-1.04.08-2.16 0 0 .67-.22 2.2.84A7.42 7.42 0 0 1 8 3.92c.68 0 1.36.09 2 .27 1.52-1.06 2.19-.84 2.19-.84.44 1.12.16 1.95.08 2.16.51.57.82 1.3.82 2.19 0 3.14-1.87 3.83-3.65 4.04.29.26.54.75.54 1.52 0 1.09-.01 1.97-.01 2.24 0 .22.15.48.55.4A8.16 8.16 0 0 0 16 8.2C16 3.67 12.42 0 8 0Z" />
      </svg>
      <span class="sr-only">GitHub</span>
    </a>

    <a
      v-if="showCommit"
      :href="buildInfo.commitUrl"
      target="_blank"
      rel="noreferrer"
      :aria-label="commitLinkLabel"
      :title="commitLinkLabel"
      class="inline-flex items-center gap-1.5 rounded-md px-1.5 py-1 text-[11px] text-gray-400 transition-colors hover:bg-gray-100 hover:text-ember cursor-pointer"
    >
      <span class="font-medium tracking-normal">build</span>
      <span class="font-mono">{{ buildInfo.shortCommitSha }}</span>
    </a>
  </div>
</template>
