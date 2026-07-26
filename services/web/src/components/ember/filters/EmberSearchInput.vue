<script setup lang="ts">
import type { Component } from 'vue'

const props = withDefaults(defineProps<{
  // 允许 undefined：各筛选面板的查询 DTO 普遍把关键字建模为可选字段。
  modelValue: string | undefined
  label: string
  placeholder?: string
  ariaLabel?: string
  icon?: Component
  type?: string
  // 与原生 input inputmode 属性取值对齐。
  inputmode?: 'text' | 'email' | 'search' | 'tel' | 'url' | 'none' | 'numeric' | 'decimal'
  autocomplete?: string
}>(), {
  placeholder: '',
  ariaLabel: '',
  icon: undefined,
  type: 'search',
  inputmode: 'search',
  autocomplete: 'off'
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
  enter: []
}>()
</script>

<template>
  <div class="space-y-1.5">
    <label class="text-xs font-semibold tracking-wide text-gray-500">{{ props.label }}</label>
    <div class="group relative w-full">
      <div v-if="props.icon" class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3">
        <el-icon class="text-gray-400 transition-colors group-focus-within:text-ember">
          <component :is="props.icon" />
        </el-icon>
      </div>
      <input
        :value="props.modelValue"
        :type="props.type"
        :inputmode="props.inputmode"
        :autocomplete="props.autocomplete"
        :aria-label="props.ariaLabel || props.label"
        :placeholder="props.placeholder"
        class="ember-filter-input w-full"
        :class="props.icon ? 'pl-10 pr-4' : 'px-4'"
        @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
        @keyup.enter="emit('enter')"
      />
    </div>
  </div>
</template>
