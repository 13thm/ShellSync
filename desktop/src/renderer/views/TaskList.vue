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
const focus = ref(false)

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

    <div class="composer" :class="{ 'is-focus': focus }">
      <Plus :size="18" :stroke-width="2" class="composer__icon" />
      <AppInput
        v-model="newName"
        placeholder="输入任务名称，回车或点击创建…"
        @keyup.enter="createTask"
        @focus="focus = true"
        @blur="focus = false"
      />
      <AppButton
        type="primary"
        class="composer__btn"
        :disabled="!newName.trim() || creating"
        @click="createTask"
      >
        创建任务
      </AppButton>
    </div>

    <div v-if="store.active.length === 0">
      <EmptyState text="还没有任务，先创建一个吧" />
    </div>

    <div v-else class="list">
      <AppListItem
        v-for="t in store.active"
        :key="t.id"
        :title="t.name || '未命名任务'"
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

/* 新建任务：一体式输入卡片，聚焦时高亮描边 */
.composer {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 6px 6px 14px;
  margin-bottom: 20px;
  background: var(--color-bg-card);
  border: 1px solid var(--color-border-base);
  border-radius: var(--radius-md);
  transition: border-color var(--motion-fast), box-shadow var(--motion-fast);
}
.composer.is-focus {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px var(--color-primary-bg);
}
.composer__icon {
  color: var(--color-text-tertiary);
  flex-shrink: 0;
}
.composer.is-focus .composer__icon {
  color: var(--color-primary);
}
.composer :deep(.app-input) {
  border: none;
  box-shadow: none;
  background: transparent;
  flex: 1;
}
.composer :deep(.app-input:focus) {
  box-shadow: none;
}
.composer__btn {
  flex-shrink: 0;
  margin-left: 8px;
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
