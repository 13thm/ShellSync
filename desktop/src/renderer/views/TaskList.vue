<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { Plus } from 'lucide-vue-next'
import { useTasksStore } from '../stores/tasks'
import AppButton from '../components/ui/AppButton.vue'
import AppInput from '../components/ui/AppInput.vue'
import AppListItem from '../components/ui/AppListItem.vue'
import StatusDot from '../components/ui/StatusDot.vue'
import EmptyState from '../components/ui/EmptyState.vue'
import { taskStatusMeta } from '../utils/status'

const store = useTasksStore()
const router = useRouter()

const newName = ref('')
const creating = ref(false)

async function createTask() {
  const name = newName.value.trim()
  if (!name) return
  creating.value = true
  try {
    const t = await store.create({ name })
    newName.value = ''
    router.push(`/tasks/${t.id}`)
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <div class="page">
    <header class="page__head">
      <h1>任务</h1>
      <div class="page__head-right">
        <span class="muted">管理业务任务，绑定终端与待办</span>
      </div>
    </header>

    <div class="create-row">
      <AppInput
        v-model="newName"
        placeholder="新建任务…"
        @keyup.enter="createTask"
      />
      <AppButton type="primary" :disabled="!newName.trim() || creating" @click="createTask">
        <Plus :size="16" :stroke-width="2" /> 新建
      </AppButton>
    </div>

    <div v-if="store.active.length === 0">
      <EmptyState text="还没有任务，先创建一个吧" />
    </div>

    <div v-else class="list">
      <AppListItem
        v-for="t in store.active"
        :key="t.id"
        :title="t.name"
        :desc="t.description || taskStatusMeta(t.status).label"
        @click="router.push(`/tasks/${t.id}`)"
      >
        <template #extra>
          <StatusDot :color="taskStatusMeta(t.status).color" />
        </template>
      </AppListItem>
    </div>
  </div>
</template>

<style scoped>
.page {
  padding: 24px 28px;
  max-width: 860px;
}
.page__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin-bottom: 20px;
}
.page__head h1 {
  font-size: var(--font-size-2xl);
  font-weight: var(--font-weight-semibold);
}
.muted {
  color: var(--color-text-tertiary);
  font-size: var(--font-size-sm);
}
.create-row {
  display: flex;
  gap: 8px;
  margin-bottom: 16px;
}
.list {
  background: var(--color-bg-card);
  border: 1px solid var(--color-border-base);
  border-radius: var(--radius-md);
  overflow: hidden;
}
.list :deep(.list-item) {
  border-bottom: 1px solid var(--color-border-light);
}
.list :deep(.list-item:last-child) {
  border-bottom: none;
}
</style>
