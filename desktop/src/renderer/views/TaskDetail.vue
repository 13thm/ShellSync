<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowLeft, Trash2 } from 'lucide-vue-next'
import { useTasksStore } from '../stores/tasks'
import { useTerminalsStore } from '../stores/terminals'
import { useTodosStore } from '../stores/todos'
import AppButton from '../components/ui/AppButton.vue'
import AppInput from '../components/ui/AppInput.vue'
import AppCard from '../components/ui/AppCard.vue'
import AppListItem from '../components/ui/AppListItem.vue'
import StatusDot from '../components/ui/StatusDot.vue'
import EmptyState from '../components/ui/EmptyState.vue'
import { taskStatusMeta, taskTransitions, terminalStatusMeta, formatTime } from '../utils/status'

const props = defineProps<{ id: string }>()
const router = useRouter()
const tasks = useTasksStore()
const terminals = useTerminalsStore()
const todos = useTodosStore()

const task = computed(() => tasks.items.find((t) => t.id === props.id))
const linkedTerminals = computed(() => terminals.items.filter((t) => t.taskId === props.id))
const linkedTodos = computed(() => todos.items.filter((t) => t.taskId === props.id))

const editingName = ref('')
const editingDesc = ref('')
watch(
  task,
  (t) => {
    if (t) {
      editingName.value = t.name
      editingDesc.value = t.description
    }
  },
  { immediate: true },
)

function terminalLabel(status: string) {
  return terminalStatusMeta(status).label
}
function terminalColor(status: string) {
  return terminalStatusMeta(status).color
}

async function saveName() {
  if (!task.value || editingName.value === task.value.name) return
  await tasks.update(props.id, { name: editingName.value.trim() })
}
async function saveDesc() {
  if (!task.value || editingDesc.value === task.value.description) return
  await tasks.update(props.id, { description: editingDesc.value })
}
async function transition(to: string) {
  await tasks.update(props.id, { status: to })
}
async function remove() {
  await tasks.remove(props.id)
  router.push('/tasks')
}
</script>

<template>
  <div class="page" v-if="task">
    <header class="page__head">
      <AppButton type="text" size="small" @click="router.push('/tasks')">
        <ArrowLeft :size="16" :stroke-width="1.75" /> 任务
      </AppButton>
    </header>

    <div class="title-row">
      <StatusDot :color="taskStatusMeta(task.status).color" :size="10" />
      <AppInput v-model="editingName" @blur="saveName" class="title-input" />
    </div>
    <div class="status-line">
      <span class="muted">{{ taskStatusMeta(task.status).label }}</span>
      <span class="muted">· 更新于 {{ formatTime(task.updatedAt) }}</span>
    </div>

    <div class="actions">
      <AppButton
        v-for="tr in taskTransitions(task.status)"
        :key="tr.to"
        type="primary"
        @click="transition(tr.to)"
      >
        {{ tr.label }}
      </AppButton>
      <AppButton type="text" danger @click="remove">
        <Trash2 :size="15" :stroke-width="1.75" /> 删除任务
      </AppButton>
    </div>

    <AppCard title="描述">
      <textarea
        v-model="editingDesc"
        class="desc-area"
        placeholder="补充任务说明…"
        @blur="saveDesc"
      />
    </AppCard>

    <AppCard title="关联终端">
      <EmptyState v-if="linkedTerminals.length === 0" text="未关联终端（在终端页可将终端归属本任务）" />
      <AppListItem
        v-for="term in linkedTerminals"
        :key="term.id"
        :title="term.name + ' · ' + term.shellType"
        :desc="terminalLabel(term.status)"
        @click="router.push('/terminals')"
      >
        <template #extra>
          <StatusDot :color="terminalColor(term.status)" />
        </template>
      </AppListItem>
    </AppCard>

    <AppCard title="关联待办">
      <EmptyState v-if="linkedTodos.length === 0" text="无待办" />
      <AppListItem
        v-for="td in linkedTodos"
        :key="td.id"
        :title="td.title"
        :desc="td.status === 'done' ? '已完成' : '待办'"
      >
        <template #extra>
          <StatusDot :color="td.status === 'done' ? 'var(--color-status-done)' : 'var(--color-status-running)'" />
        </template>
      </AppListItem>
    </AppCard>
  </div>

  <EmptyState v-else text="任务不存在" />
</template>

<style scoped>
.page {
  padding: 16px 28px 32px;
  max-width: 860px;
}
.page__head {
  margin-bottom: 8px;
}
.title-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 4px;
}
.title-input :deep(input) {
  height: 36px;
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-semibold);
  border: none;
  padding: 0;
}
.title-input :deep(input):focus {
  box-shadow: none;
}
.status-line {
  display: flex;
  gap: 8px;
  margin-bottom: 20px;
  font-size: var(--font-size-sm);
}
.muted {
  color: var(--color-text-tertiary);
}
.actions {
  display: flex;
  gap: 8px;
  margin-bottom: 24px;
}
.desc-area {
  width: 100%;
  min-height: 80px;
  resize: vertical;
  border: none;
  outline: none;
  font-family: var(--font-family);
  font-size: var(--font-size-base);
  color: var(--color-text-primary);
  background: transparent;
  line-height: var(--line-height-base);
}
.desc-area::placeholder {
  color: var(--color-text-placeholder);
}
:deep(.app-card) {
  margin-bottom: 16px;
}
</style>
