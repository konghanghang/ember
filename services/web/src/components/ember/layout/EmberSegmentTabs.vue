<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import type { Component } from 'vue'

type SegmentTab = {
  key: string
  label: string
  icon?: Component
}

const props = withDefaults(defineProps<{
  modelValue: string
  tabs: SegmentTab[]
  fullWidth?: boolean
  ariaLabel: string
}>(), {
  fullWidth: true
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
  change: [value: string]
}>()

const tabRefs = ref<Array<HTMLButtonElement | null>>([])

const activeIndex = computed(() => {
  const index = props.tabs.findIndex((tab) => tab.key === props.modelValue)
  return index >= 0 ? index : 0
})

const handleSelect = (value: string) => {
  if (value === props.modelValue) return
  emit('update:modelValue', value)
  emit('change', value)
}

const setTabRef = (index: number, element: Element | null) => {
  tabRefs.value[index] = element instanceof HTMLButtonElement ? element : null
}

const focusTab = (index: number) => {
  nextTick(() => {
    tabRefs.value[index]?.focus()
  })
}

const selectTabByIndex = (index: number, options?: { focus?: boolean }) => {
  const target = props.tabs[index]
  if (!target) return
  handleSelect(target.key)
  if (options?.focus) {
    focusTab(index)
  }
}

const moveFocus = (offset: number) => {
  if (props.tabs.length === 0) return
  const nextIndex = (activeIndex.value + offset + props.tabs.length) % props.tabs.length
  selectTabByIndex(nextIndex, { focus: true })
}

const handleKeydown = (event: KeyboardEvent) => {
  switch (event.key) {
    case 'ArrowRight':
      event.preventDefault()
      moveFocus(1)
      break
    case 'ArrowLeft':
      event.preventDefault()
      moveFocus(-1)
      break
    case 'Home':
      event.preventDefault()
      selectTabByIndex(0, { focus: true })
      break
    case 'End':
      event.preventDefault()
      selectTabByIndex(props.tabs.length - 1, { focus: true })
      break
    default:
      break
  }
}
</script>

<template>
  <div
    :class="['inline-flex rounded-2xl bg-slate-100 p-1', props.fullWidth ? 'w-full lg:w-auto' : 'w-auto']"
    role="radiogroup"
    aria-orientation="horizontal"
    :aria-label="props.ariaLabel"
  >
    <button
      v-for="(tab, index) in props.tabs"
      :key="tab.key"
      :ref="(element) => setTabRef(index, element)"
      type="button"
      role="radio"
      :aria-checked="props.modelValue === tab.key"
      :tabindex="activeIndex === index ? 0 : -1"
      class="flex cursor-pointer items-center justify-center gap-2 rounded-xl px-4 py-2.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-ember/10"
      :class="[
        props.fullWidth ? 'flex-1 lg:flex-none' : '',
        props.modelValue === tab.key ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-500 hover:text-slate-700'
      ]"
      @click="handleSelect(tab.key)"
      @keydown="handleKeydown"
    >
      <el-icon v-if="tab.icon"><component :is="tab.icon" /></el-icon>
      <span>{{ tab.label }}</span>
    </button>
  </div>
</template>
