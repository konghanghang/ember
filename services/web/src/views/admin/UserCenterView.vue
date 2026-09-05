<script setup lang="ts">
import EmberPageHeaderCard from '@/components/ember/layout/EmberPageHeaderCard.vue'
import UsersView from './UsersView.vue'
</script>

<template>
  <div class="space-y-6">
    <EmberPageHeaderCard title="用户中心" />
    <div class="min-w-0">
      <UsersView embedded />
    </div>
  </div>
</template>
