<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { CollectionTag, Delete, EditPen, Plus, Refresh } from '@element-plus/icons-vue'
import { createPlanGroup, deletePlanGroup, getPlanGroups, updatePlanGroup } from '@/api/admin'
import type { CreatePlanGroupRequest, ManagedPlanGroup, UpdatePlanGroupRequest } from '@/types/api'

const props = withDefaults(defineProps<{
  embedded?: boolean
}>(), {
  embedded: false
})

const loading = ref(false)
const creating = ref(false)
const updating = ref(false)
const groups = ref<ManagedPlanGroup[]>([])

const dialogVisible = ref(false)
const editDialogVisible = ref(false)

const createForm = ref({
  key: '',
  name: '',
  description: '',
  isDefault: false,
  sortOrder: 0
})

const editForm = ref({
  key: '',
  name: '',
  description: '',
  isDefault: false,
  sortOrder: 0
})

const defaultGroup = computed(() => groups.value.find(group => group.isDefault) ?? null)

const fetchData = async () => {
  loading.value = true
  try {
    const res = await getPlanGroups()
    groups.value = res.data ?? []
  } finally {
    loading.value = false
  }
}

const resetCreateForm = () => {
  createForm.value = {
    key: '',
    name: '',
    description: '',
    isDefault: false,
    sortOrder: 0
  }
}

const openEditDialog = (group: ManagedPlanGroup) => {
  editForm.value = {
    key: group.key,
    name: group.name,
    description: group.description ?? '',
    isDefault: group.isDefault,
    sortOrder: group.sortOrder
  }
  editDialogVisible.value = true
}

const handleCreate = async () => {
  if (!createForm.value.key.trim() || !createForm.value.name.trim()) {
    ElMessage.warning('请填写分组标识和分组名称')
    return
  }

  const payload: CreatePlanGroupRequest = {
    key: createForm.value.key.trim(),
    name: createForm.value.name.trim(),
    description: createForm.value.description.trim(),
    isDefault: createForm.value.isDefault,
    sortOrder: createForm.value.sortOrder
  }

  creating.value = true
  try {
    await createPlanGroup(payload)
    ElMessage.success('套餐分组创建成功')
    dialogVisible.value = false
    resetCreateForm()
    await fetchData()
  } finally {
    creating.value = false
  }
}

const handleUpdate = async () => {
  if (!editForm.value.name.trim()) {
    ElMessage.warning('请输入分组名称')
    return
  }

  const payload: UpdatePlanGroupRequest = {
    name: editForm.value.name.trim(),
    description: editForm.value.description.trim(),
    isDefault: editForm.value.isDefault,
    sortOrder: editForm.value.sortOrder
  }

  updating.value = true
  try {
    await updatePlanGroup(editForm.value.key, payload)
    ElMessage.success('套餐分组更新成功')
    editDialogVisible.value = false
    await fetchData()
  } finally {
    updating.value = false
  }
}

const handleDelete = async (group: ManagedPlanGroup) => {
  try {
    await ElMessageBox.confirm(
      `确定删除分组 ${group.name} 吗？若仍有用户或套餐引用，后端会直接拒绝。`,
      '删除确认',
      {
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        type: 'warning',
        confirmButtonClass: 'el-button--danger'
      }
    )

    await deletePlanGroup(group.key)
    ElMessage.success('套餐分组删除成功')
    await fetchData()
  } catch {
    // cancelled
  }
}

onMounted(fetchData)
</script>

<template>
  <div class="space-y-6">
    <div class="flex flex-col gap-4 rounded-2xl border border-gray-100 bg-white p-6 shadow-sm md:flex-row md:items-center md:justify-between">
      <div>
        <h1 class="flex items-center gap-2 text-2xl font-bold text-gray-900">
          套餐分组管理
          <span class="rounded-full bg-gray-100 px-2 py-1 text-xs font-normal text-gray-500">{{ groups.length }} 个分组</span>
        </h1>
        <p class="mt-1 text-sm text-gray-500">先维护分组，再把套餐和用户挂到合适的分组；未显式绑定的用户会跟随默认分组。</p>
      </div>

      <div class="flex items-center gap-3">
        <button
          @click="fetchData"
          class="cursor-pointer rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900"
          aria-label="刷新套餐分组列表"
          title="刷新列表"
        >
          <el-icon :size="20"><Refresh /></el-icon>
        </button>
        <button
          @click="dialogVisible = true"
          class="flex items-center gap-2 rounded-lg bg-ember px-4 py-2 font-bold text-white shadow-md transition-colors hover:bg-red-700 hover:shadow-lg active:scale-95"
        >
          <el-icon><Plus /></el-icon>
          <span>新建分组</span>
        </button>
      </div>
    </div>

    <div class="grid gap-4 lg:grid-cols-3">
      <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
        <div class="text-sm font-semibold text-gray-500">默认分组</div>
        <div class="mt-3 flex items-center gap-3">
          <div class="flex h-11 w-11 items-center justify-center rounded-2xl bg-amber-50 text-amber-500">
            <el-icon :size="22"><CollectionTag /></el-icon>
          </div>
          <div>
            <div class="text-lg font-semibold text-gray-900">{{ defaultGroup?.name || '未设置' }}</div>
            <div class="text-sm text-gray-500">{{ defaultGroup?.key || '暂无默认分组' }}</div>
          </div>
        </div>
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
        <div class="text-sm font-semibold text-gray-500">跟随默认用户</div>
        <div class="mt-3 text-3xl font-bold text-gray-900">
          {{ defaultGroup?.followingUserCount ?? 0 }}
        </div>
        <div class="mt-1 text-sm text-gray-500">这批用户切默认分组时会整体切换可购套餐。</div>
      </div>

      <div class="rounded-2xl border border-gray-100 bg-white p-5 shadow-sm">
        <div class="text-sm font-semibold text-gray-500">分组引用总量</div>
        <div class="mt-3 text-3xl font-bold text-gray-900">
          {{ groups.reduce((sum, item) => sum + (item.planCount ?? 0) + (item.userCount ?? 0), 0) }}
        </div>
        <div class="mt-1 text-sm text-gray-500">包含套餐绑定数和显式用户绑定数。</div>
      </div>
    </div>

    <div class="overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-sm">
      <el-table
        :data="groups"
        v-loading="loading"
        style="width: 100%"
        :header-cell-style="{ background: '#f9fafb', color: '#6b7280', fontWeight: '600' }"
      >
        <el-table-column label="分组" min-width="240">
          <template #default="{ row }">
            <div class="flex items-center gap-3">
              <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-orange-50 text-orange-500">
                <el-icon><CollectionTag /></el-icon>
              </div>
              <div>
                <div class="flex items-center gap-2">
                  <span class="font-semibold text-gray-900">{{ row.name }}</span>
                  <el-tag v-if="row.isDefault" type="warning" effect="light" round size="small">默认</el-tag>
                </div>
                <div class="text-xs text-gray-500">{{ row.key }}</div>
              </div>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="说明" min-width="220">
          <template #default="{ row }">
            <span class="text-sm text-gray-600">{{ row.description || '无说明' }}</span>
          </template>
        </el-table-column>

        <el-table-column label="套餐数" width="100">
          <template #default="{ row }">
            <span class="font-medium text-gray-700">{{ row.planCount ?? 0 }}</span>
          </template>
        </el-table-column>

        <el-table-column label="显式用户" width="110">
          <template #default="{ row }">
            <span class="font-medium text-gray-700">{{ row.userCount ?? 0 }}</span>
          </template>
        </el-table-column>

        <el-table-column label="跟随默认" width="110">
          <template #default="{ row }">
            <span class="font-medium text-gray-700">{{ row.followingUserCount ?? 0 }}</span>
          </template>
        </el-table-column>

        <el-table-column label="排序" width="90">
          <template #default="{ row }">
            <span class="text-gray-600">{{ row.sortOrder }}</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <button
              @click="openEditDialog(row)"
              class="cursor-pointer rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-blue-50 hover:text-blue-600"
              aria-label="编辑套餐分组"
              title="编辑"
            >
              <el-icon :size="18"><EditPen /></el-icon>
            </button>
            <button
              @click="handleDelete(row)"
              class="cursor-pointer rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600"
              aria-label="删除套餐分组"
              title="删除"
            >
              <el-icon :size="18"><Delete /></el-icon>
            </button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="dialogVisible" title="新建套餐分组" width="520px" align-center append-to-body>
      <div class="p-6 pt-2">
        <el-form label-position="top" class="space-y-4">
          <el-form-item label="分组标识">
            <el-input v-model="createForm.key" placeholder="例如：VIP_A" />
            <p class="mt-1 text-xs text-gray-500">分组标识会作为稳定引用保存，创建后不支持修改，建议使用大写字母、数字、下划线或连字符。</p>
          </el-form-item>

          <el-form-item label="分组名称">
            <el-input v-model="createForm.name" placeholder="例如：新客优惠组" />
          </el-form-item>

          <el-form-item label="说明">
            <el-input v-model="createForm.description" type="textarea" :rows="3" placeholder="可选，说明这个分组给谁用" />
          </el-form-item>

          <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
            <el-form-item label="排序">
              <el-input-number v-model="createForm.sortOrder" :min="0" class="w-full !w-full" />
            </el-form-item>

            <el-form-item label="默认分组">
              <div class="flex h-10 items-center">
                <el-switch v-model="createForm.isDefault" active-text="是" inactive-text="否" />
              </div>
            </el-form-item>
          </div>
        </el-form>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3 px-6 pb-6 pt-0">
          <button
            @click="dialogVisible = false"
            class="rounded-lg px-4 py-2 font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900"
          >
            取消
          </button>
          <button
            @click="handleCreate"
            :disabled="creating"
            class="rounded-lg bg-ember px-6 py-2 font-bold text-white shadow-md transition-colors hover:bg-red-700 hover:shadow-lg disabled:opacity-70"
          >
            {{ creating ? '创建中...' : '确认创建' }}
          </button>
        </div>
      </template>
    </el-dialog>

    <el-dialog v-model="editDialogVisible" title="编辑套餐分组" width="520px" align-center append-to-body>
      <div class="p-6 pt-2">
        <el-form label-position="top" class="space-y-4">
          <el-form-item label="分组标识">
            <el-input :model-value="editForm.key" disabled />
          </el-form-item>

          <el-form-item label="分组名称">
            <el-input v-model="editForm.name" placeholder="例如：新客优惠组" />
          </el-form-item>

          <el-form-item label="说明">
            <el-input v-model="editForm.description" type="textarea" :rows="3" placeholder="可选，说明这个分组给谁用" />
          </el-form-item>

          <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
            <el-form-item label="排序">
              <el-input-number v-model="editForm.sortOrder" :min="0" class="w-full !w-full" />
            </el-form-item>

            <el-form-item label="默认分组">
              <div class="flex h-10 items-center">
                <el-switch v-model="editForm.isDefault" active-text="是" inactive-text="否" />
              </div>
            </el-form-item>
          </div>
        </el-form>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3 px-6 pb-6 pt-0">
          <button
            @click="editDialogVisible = false"
            class="rounded-lg px-4 py-2 font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900"
          >
            取消
          </button>
          <button
            @click="handleUpdate"
            :disabled="updating"
            class="rounded-lg bg-ember px-6 py-2 font-bold text-white shadow-md transition-colors hover:bg-red-700 hover:shadow-lg disabled:opacity-70"
          >
            {{ updating ? '保存中...' : '保存修改' }}
          </button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>
