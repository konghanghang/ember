<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch, type Component } from 'vue'
import { useRoute } from 'vue-router'
import { Bell, ChatDotRound, Close, QuestionFilled, Reading } from '@element-plus/icons-vue'
import { useConsoleStore } from '@/store/console'
import type { ConsoleAccountLink, ConsoleAccountLinkIcon } from '@/types/api'

const route = useRoute()
const consoleStore = useConsoleStore()
const widgetRef = ref<HTMLElement | null>(null)
const panelOpen = ref(false)

const resourceIconMap: Record<ConsoleAccountLinkIcon, Component> = {
  notify: Bell,
  group: ChatDotRound,
  wiki: Reading
}

const resourceTagMap: Record<ConsoleAccountLinkIcon, string> = {
  notify: '必看',
  group: '交流',
  wiki: '文档'
}

const resourceActionMap: Record<ConsoleAccountLinkIcon, string> = {
  notify: '查看公告',
  group: '加入群组',
  wiki: '打开文档'
}

const resourcePriorityMap: Record<ConsoleAccountLinkIcon, number> = {
  notify: 0,
  group: 1,
  wiki: 2
}

const resourceToneMap: Record<ConsoleAccountLinkIcon, { icon: string; badge: string; card: string }> = {
  notify: {
    icon: 'bg-ember/10 text-ember',
    badge: 'bg-ember/10 text-ember',
    card: 'border-ember/20 bg-ember/[0.04] hover:border-ember/30 hover:bg-white'
  },
  group: {
    icon: 'bg-sky-50 text-sky-700',
    badge: 'bg-sky-50 text-sky-700',
    card: 'border-slate-200 bg-slate-50 hover:border-sky-200 hover:bg-white'
  },
  wiki: {
    icon: 'bg-amber-50 text-amber-700',
    badge: 'bg-amber-50 text-amber-700',
    card: 'border-slate-200 bg-slate-50 hover:border-amber-200 hover:bg-white'
  }
}

const sortedLinks = computed<ConsoleAccountLink[]>(() => {
  return [...consoleStore.accountLinks].sort((a, b) => {
    const priorityDiff = resourcePriorityMap[a.icon] - resourcePriorityMap[b.icon]
    if (priorityDiff !== 0) return priorityDiff
    return a.sortOrder - b.sortOrder
  })
})

const primaryLink = computed(() => sortedLinks.value[0] ?? null)
const secondaryLinks = computed(() => sortedLinks.value.slice(1))

const closePanel = () => {
  panelOpen.value = false
}

const togglePanel = () => {
  panelOpen.value = !panelOpen.value
}

const handleClickOutside = (event: MouseEvent) => {
  if (!panelOpen.value || !widgetRef.value) return
  if (widgetRef.value.contains(event.target as Node)) return
  closePanel()
}

const handleKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') closePanel()
}

watch(
  () => route.fullPath,
  () => {
    closePanel()
  }
)

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  document.addEventListener('keydown', handleKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
  document.removeEventListener('keydown', handleKeydown)
})
</script>

<template>
  <div
    v-if="sortedLinks.length > 0"
    ref="widgetRef"
    class="fixed bottom-4 right-4 z-30 sm:bottom-auto sm:right-0 sm:top-1/2 sm:-translate-y-1/2"
  >
    <transition name="help-panel">
      <section
        v-if="panelOpen"
        id="console-help-panel"
        class="mb-3 w-[min(22rem,calc(100vw-2rem))] rounded-3xl border border-slate-200 bg-white p-4 shadow-2xl shadow-slate-900/10 sm:mb-4 sm:mr-3"
      >
        <div class="flex items-start justify-between gap-4">
          <h2 class="text-base font-semibold text-slate-900">帮助与资源</h2>
          <button
            aria-label="关闭帮助面板"
            class="flex h-9 w-9 items-center justify-center rounded-2xl text-slate-400 transition-colors hover:bg-slate-100 hover:text-slate-700 cursor-pointer"
            @click="closePanel"
          >
            <el-icon :size="18"><Close /></el-icon>
          </button>
        </div>

        <a
          v-if="primaryLink"
          :href="primaryLink.url"
          target="_blank"
          rel="noreferrer"
          class="group mt-4 block rounded-2xl border p-4 no-underline transition-colors cursor-pointer"
          :class="resourceToneMap[primaryLink.icon].card"
        >
          <div class="flex items-start gap-3">
            <div
              class="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl"
              :class="resourceToneMap[primaryLink.icon].icon"
            >
              <el-icon :size="20">
                <component :is="resourceIconMap[primaryLink.icon]" />
              </el-icon>
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <p class="text-sm font-semibold text-slate-900">{{ primaryLink.title }}</p>
                <span
                  class="inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-semibold"
                  :class="resourceToneMap[primaryLink.icon].badge"
                >
                  {{ resourceTagMap[primaryLink.icon] }}
                </span>
              </div>
              <p class="mt-1 text-xs leading-5 text-slate-600">{{ primaryLink.description }}</p>
            </div>
          </div>
          <div class="mt-4 flex items-center justify-end border-t border-current/10 pt-3">
            <span class="text-sm font-semibold text-ember">
              {{ resourceActionMap[primaryLink.icon] }}
            </span>
          </div>
        </a>

        <div v-if="secondaryLinks.length > 0" class="mt-3 space-y-3">
          <a
            v-for="item in secondaryLinks"
            :key="item.key"
            :href="item.url"
            target="_blank"
            rel="noreferrer"
            class="group flex items-start gap-3 rounded-2xl border p-4 no-underline transition-colors cursor-pointer"
            :class="resourceToneMap[item.icon].card"
          >
            <div
              class="flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl"
              :class="resourceToneMap[item.icon].icon"
            >
              <el-icon :size="18">
                <component :is="resourceIconMap[item.icon]" />
              </el-icon>
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <p class="text-sm font-semibold text-slate-900">{{ item.title }}</p>
                <span
                  class="inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-semibold"
                  :class="resourceToneMap[item.icon].badge"
                >
                  {{ resourceTagMap[item.icon] }}
                </span>
              </div>
              <p class="mt-1 text-xs leading-5 text-slate-500">{{ item.description }}</p>
              <p class="mt-3 text-sm font-semibold text-slate-700">
                {{ resourceActionMap[item.icon] }}
              </p>
            </div>
          </a>
        </div>
      </section>
    </transition>

    <div class="flex justify-end">
      <button
        aria-controls="console-help-panel"
        :aria-expanded="panelOpen"
        :aria-label="panelOpen ? '收起帮助与资源' : '打开帮助与资源'"
        class="group flex h-14 w-14 items-center justify-center overflow-hidden rounded-full border border-slate-200 bg-white text-slate-700 shadow-lg shadow-slate-900/10 transition-all duration-200 hover:border-ember/30 sm:h-14 sm:w-[3.5rem] sm:justify-end sm:rounded-l-full sm:rounded-r-none sm:border-r-0 sm:pl-4 sm:pr-3"
        :class="panelOpen ? 'border-ember/40 sm:w-[7.5rem]' : 'sm:hover:w-[7.5rem]'"
        @click="togglePanel"
      >
        <span
          class="hidden overflow-hidden whitespace-nowrap text-sm font-semibold transition-all duration-200 sm:block"
          :class="panelOpen ? 'mr-2 max-w-[3rem] opacity-100' : 'mr-0 max-w-0 opacity-0 group-hover:mr-2 group-hover:max-w-[3rem] group-hover:opacity-100'"
        >
          帮助
        </span>
        <span
          class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-ember/10 text-ember ring-1 ring-ember/15 transition-all duration-200 group-hover:bg-ember/15 group-hover:ring-ember/25"
          :class="panelOpen ? 'bg-ember/15 ring-ember/25' : ''"
        >
          <el-icon :size="20">
            <QuestionFilled />
          </el-icon>
        </span>
        <span class="sr-only">帮助与资源</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.help-panel-enter-active,
.help-panel-leave-active {
  transition: opacity 0.18s ease, transform 0.18s ease;
}

.help-panel-enter-from,
.help-panel-leave-to {
  opacity: 0;
  transform: translateY(10px);
}
</style>
