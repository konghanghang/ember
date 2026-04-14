<script setup lang="ts">
defineOptions({
  inheritAttrs: false
})

const props = withDefaults(defineProps<{
  modelValue: boolean
  title: string
  width?: string
}>(), {
  width: '520px'
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()
</script>

<template>
  <el-dialog
    :model-value="props.modelValue"
    :title="props.title"
    :width="props.width"
    align-center
    append-to-body
    v-bind="$attrs"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <slot />

    <template v-if="$slots.footer" #footer>
      <slot name="footer" />
    </template>
  </el-dialog>
</template>

<style scoped>
:deep(.el-dialog__body) {
  padding-top: 1rem;
}

:deep(.el-dialog__footer) {
  padding-top: 0;
}
</style>
