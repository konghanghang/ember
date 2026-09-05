<script setup lang="ts">
import { computed, ref } from 'vue'
import { User, CollectionTag } from '@element-plus/icons-vue'
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import EmberSegmentTabs from '@/components/ember/layout/EmberSegmentTabs.vue'
import UsersView from './UsersView.vue'
import PlanGroupsView from './PlanGroupsView.vue'

type Tab = 'users' | 'groups'
const activeTab = ref<Tab>('users')
const tabs = [
  { key: 'users', label: '用户管理', icon: User },
  { key: 'groups', label: '用户分组', icon: CollectionTag },
]
const activeComponent = computed(() => activeTab.value === 'users' ? UsersView : PlanGroupsView)
</script>

<template>
  <div class="space-y-6">
    <EmberPageHeaderCard title="用户中心" description="统一管理用户账号、有效期、分组和访问策略。">
      <template #actions>
        <div class="max-w-full overflow-x-auto">
          <EmberSegmentTabs v-model="activeTab" :tabs="tabs" aria-label="用户中心分段" ariaLabel="用户中心分段" />
        </div>
      </template>
    </EmberPageHeaderCard>
    <div class="min-w-0">
      <component :is="activeComponent" embedded />
    </div>
  </div>
</template>
