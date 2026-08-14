<script setup lang="ts">
import { ref } from 'vue'
import { Plus, Trash2, Check, ListChecks } from 'lucide-vue-next'
import { useTodosStore } from '../stores/todos'
import { useTasksStore } from '../stores/tasks'
import AppButton from '../components/ui/AppButton.vue'
import AppInput from '../components/ui/AppInput.vue'
import AppCard from '../components/ui/AppCard.vue'
import EmptyState from '../components/ui/EmptyState.vue'

const todos = useTodosStore()
const tasks = useTasksStore()

const newTitle = ref('')
const newTaskId = ref('')
const focus = ref(false)

async function add() {
  const title = newTitle.value.trim()
  if (!title) return
  await todos.create({ title, taskId: newTaskId.value || undefined })
  newTitle.value = ''
}

function taskName(id: string) {
  return tasks.items.find((t) => t.id === id)?.name ?? ''
}
</script>

<template>
  <div class="page">
    <header class="page__head">
      <h1>待办</h1>
      <span class="muted">将待办与任务 / 终端绑定，跟踪开发计划</span>
    </header>

    <div class="composer" :class="{ 'is-focus': focus }">
      <Plus :size="18" :stroke-width="2" class="composer__icon" />
      <AppInput
        v-model="newTitle"
        placeholder="输入待办内容，回车或点击添加…"
        @keyup.enter="add"
        @focus="focus = true"
        @blur="focus = false"
      />
      <select v-model="newTaskId" class="composer__sel" title="关联任务">
        <option value="">不关联任务</option>
        <option v-for="t in tasks.active" :key="t.id" :value="t.id">{{ t.name }}</option>
      </select>
      <AppButton type="primary" class="composer__btn" :disabled="!newTitle.trim()" @click="add">
        添加
      </AppButton>
    </div>

    <AppCard title="待办">
      <EmptyState v-if="todos.pending.length === 0" text="没有待办，专注当下 🎯" />
      <div v-else>
        <div v-for="t in todos.pending" :key="t.id" class="todo-row">
          <button class="check" @click="todos.toggle(t.id, true)" title="标记完成" />
          <div class="todo-main">
            <div class="todo-title">{{ t.title }}</div>
            <div v-if="t.content || t.taskId" class="todo-sub">
              <span v-if="t.content">{{ t.content }}</span>
              <span v-if="t.taskId" class="chip">{{ taskName(t.taskId) }}</span>
            </div>
          </div>
          <AppButton type="text" size="small" danger @click="todos.remove(t.id)">
            <Trash2 :size="14" :stroke-width="1.75" />
          </AppButton>
        </div>
      </div>
    </AppCard>

    <AppCard v-if="todos.done.length > 0" title="已完成">
      <div v-for="t in todos.done" :key="t.id" class="todo-row is-done">
        <button class="check is-checked" @click="todos.toggle(t.id, false)">
          <Check :size="12" :stroke-width="3" />
        </button>
        <div class="todo-main">
          <div class="todo-title">{{ t.title }}</div>
        </div>
        <AppButton type="text" size="small" danger @click="todos.remove(t.id)">
          <Trash2 :size="14" :stroke-width="1.75" />
        </AppButton>
      </div>
    </AppCard>
  </div>
</template>

<style scoped>
.page { padding: 24px 28px; max-width: 860px; }
.page__head { display: flex; align-items: baseline; gap: 12px; margin-bottom: 20px; }
.page__head h1 { font-size: var(--font-size-2xl); font-weight: var(--font-weight-semibold); }
.muted { color: var(--color-text-tertiary); font-size: var(--font-size-sm); }
/* 新建待办：一体式输入卡片 */
.composer {
  display: flex;
  align-items: center;
  gap: 6px;
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
.composer__sel {
  height: 30px; flex-shrink: 0; max-width: 150px;
  border: 1px solid var(--color-border-base); border-radius: var(--radius-sm);
  background: var(--color-bg-card); color: var(--color-text-primary); font-size: var(--font-size-sm);
  padding: 0 6px;
}
.composer__btn { flex-shrink: 0; }
.todo-row { display: flex; align-items: flex-start; gap: 12px; padding: 10px 0; border-bottom: 1px solid var(--color-border-light); }
.todo-row:last-child { border-bottom: none; }
.check {
  width: 18px; height: 18px; margin-top: 2px; border-radius: var(--radius-sm);
  border: 1.5px solid var(--color-border-strong); background: transparent; cursor: pointer;
  display: inline-flex; align-items: center; justify-content: center; color: var(--color-text-inverse);
  flex-shrink: 0;
}
.check.is-checked { background: var(--color-primary); border-color: var(--color-primary); }
.todo-main { flex: 1; min-width: 0; }
.todo-title { font-size: var(--font-size-base); color: var(--color-text-primary); }
.is-done .todo-title { color: var(--color-text-tertiary); text-decoration: line-through; }
.todo-sub { font-size: var(--font-size-xs); color: var(--color-text-tertiary); margin-top: 3px; display: flex; gap: 8px; align-items: center; }
.chip { background: var(--color-bg-hover); padding: 1px 6px; border-radius: var(--radius-sm); }
</style>
