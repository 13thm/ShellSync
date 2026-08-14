<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowLeft, Trash2, Plus, Check, Link2 } from 'lucide-vue-next'
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

/** 未归属其它任务的终端，可供绑定。 */
const bindableTerminals = computed(() => terminals.items.filter((t) => !t.taskId))
const bindTerminalId = ref('')
const newTodoTitle = ref('')
const creatingTerm = ref(false)

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

/** 终端 ↔ 任务 关联 */
async function bindTerminal() {
  if (!bindTerminalId.value) return
  await terminals.bindTask(bindTerminalId.value, props.id)
  bindTerminalId.value = ''
}
async function unbindTerminal(termId: string) {
  await terminals.bindTask(termId, '')
}
async function createTerminalForTask() {
  const PREFERRED = ['pwsh', 'powershell', 'cmd', 'bash', 'zsh']
  const list = terminals.shells.filter((s) => s.available)
  let shellType = list[0]?.type ?? 'cmd'
  for (const p of PREFERRED) {
    if (list.some((s) => s.type === p)) {
      shellType = p
      break
    }
  }
  creatingTerm.value = true
  try {
    await terminals.create({ shellType, taskId: props.id, name: (task.value?.name || '任务') + ' · 终端' })
  } finally {
    creatingTerm.value = false
  }
}

/** 任务内待办 */
async function addTodo() {
  const title = newTodoTitle.value.trim()
  if (!title) return
  await todos.create({ title, taskId: props.id })
  newTodoTitle.value = ''
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
      <AppInput v-model="editingName" :placeholder="task.name ? '' : '未命名任务，点击重命名…'" @blur="saveName" class="title-input" />
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
      <EmptyState v-if="linkedTerminals.length === 0" text="未关联终端：绑定已有终端，或为本任务新建一个" />
      <AppListItem
        v-for="term in linkedTerminals"
        :key="term.id"
        :title="term.name + ' · ' + term.shellType"
        :desc="terminalLabel(term.status)"
        @click="router.push('/terminals')"
      >
        <template #extra>
          <div class="row-actions" @click.stop>
            <StatusDot :color="terminalColor(term.status)" />
            <AppButton type="text" size="small" @click="unbindTerminal(term.id)">解除</AppButton>
          </div>
        </template>
      </AppListItem>
      <div class="bind-row">
        <select v-model="bindTerminalId" class="sel">
          <option value="">选择未归属的终端…</option>
          <option v-for="term in bindableTerminals" :key="term.id" :value="term.id">
            {{ term.name }} · {{ term.shellType }}
          </option>
        </select>
        <AppButton type="default" :disabled="!bindTerminalId" @click="bindTerminal">
          <Link2 :size="14" :stroke-width="1.75" /> 绑定
        </AppButton>
        <AppButton type="primary" :disabled="creatingTerm || terminals.shells.length === 0" @click="createTerminalForTask">
          <Plus :size="14" :stroke-width="2" /> 新建终端
        </AppButton>
      </div>
    </AppCard>

    <AppCard title="关联待办">
      <EmptyState v-if="linkedTodos.length === 0" text="无待办" />
      <div v-for="td in linkedTodos" :key="td.id" class="todo-row" :class="{ 'is-done': td.status === 'done' }">
        <button
          class="check"
          :class="{ 'is-checked': td.status === 'done' }"
          @click="todos.toggle(td.id, td.status !== 'done')"
          :title="td.status === 'done' ? '恢复为待办' : '标记完成'"
        >
          <Check v-if="td.status === 'done'" :size="12" :stroke-width="3" />
        </button>
        <div class="todo-title">{{ td.title }}</div>
      </div>
      <div class="bind-row">
        <AppInput
          v-model="newTodoTitle"
          placeholder="给本任务添加待办…"
          @keyup.enter="addTodo"
        />
        <AppButton type="primary" :disabled="!newTodoTitle.trim()" @click="addTodo">
          <Plus :size="14" :stroke-width="2" /> 添加
        </AppButton>
      </div>
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
.bind-row {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px dashed var(--color-border-light);
}
.bind-row .sel {
  flex: 1;
  min-width: 0;
  height: 32px;
  border: 1px solid var(--color-border-base);
  border-radius: var(--radius-sm);
  background: var(--color-bg-card);
  color: var(--color-text-primary);
  font-size: var(--font-size-sm);
  padding: 0 8px;
}
.row-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}
.todo-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 0;
  border-bottom: 1px solid var(--color-border-light);
}
.todo-row:last-of-type {
  border-bottom: none;
}
.todo-row .check {
  width: 18px;
  height: 18px;
  border-radius: var(--radius-sm);
  border: 1.5px solid var(--color-border-strong);
  background: transparent;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--color-text-inverse);
  flex-shrink: 0;
}
.todo-row .check.is-checked {
  background: var(--color-primary);
  border-color: var(--color-primary);
}
.todo-title {
  font-size: var(--font-size-base);
  color: var(--color-text-primary);
}
.is-done .todo-title {
  color: var(--color-text-tertiary);
  text-decoration: line-through;
}
</style>
