<script setup lang="ts">
import type { ManagedPlanGroup, PlanBenefit } from '@/types/api'
const props = defineProps<{ groups: ManagedPlanGroup[] }>()
const benefits = defineModel<PlanBenefit[]>({ required: true })

/** 新权益由管理员明确选择目标组，不自动推断高低等级。 */
function addBenefit() {
  benefits.value = [...benefits.value, { planGroup: props.groups.find(g => !benefits.value.some(b => b.planGroup === g.key))?.key || '', validityType: 'duration', durationDays: 30 }]
}

/** 切换期限类型时清除不再适用的天数。 */
function changeValidity(benefit: PlanBenefit) {
  benefit.durationDays = benefit.validityType === 'permanent' ? undefined : 30
}
</script>

<template>
  <fieldset class="space-y-3">
    <legend class="mb-2 text-sm font-medium text-gray-700">套餐权益</legend>
    <div v-for="(benefit, index) in benefits" :key="index" class="space-y-3 rounded-xl border border-gray-100 bg-gray-50 p-3">
      <el-form-item :label="`权益 ${index + 1} 分组`">
        <el-select v-model="benefit.planGroup" class="form-select w-full">
          <el-option v-for="group in groups" :key="group.key" :value="group.key" :label="group.name" />
        </el-select>
      </el-form-item>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <el-form-item label="有效期">
          <el-select v-model="benefit.validityType" class="form-select w-full" @change="changeValidity(benefit)">
            <el-option label="限时" value="duration" /><el-option label="永久" value="permanent" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="benefit.validityType === 'duration'" label="天数">
          <el-input-number v-model="benefit.durationDays" :min="1" :precision="0" class="form-number !w-full" />
        </el-form-item>
      </div>
      <button type="button" class="cursor-pointer rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-600" @click="benefits = benefits.filter((_, i) => i !== index)">移除权益 {{ index + 1 }}</button>
    </div>
    <button type="button" class="cursor-pointer rounded-xl border border-gray-200 bg-white px-4 py-2 text-sm" @click="addBenefit">添加权益</button>
  </fieldset>
</template>
