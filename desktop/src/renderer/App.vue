<script setup lang="ts">
import { onMounted, computed } from 'vue'
import { RouterView, useRouter } from 'vue-router'
import { useConnectionStore } from './stores/connection'
import { useRealtimeStore } from './stores/realtime'
import {
  ListChecks,
  TerminalSquare,
  CheckSquare,
  Settings as SettingsIcon,
} from 'lucide-vue-next'

const conn = useConnectionStore()
const realtime = useRealtimeStore()
const router = useRouter()

onMounted(async () => {
  const ok = await conn.init()
  if (ok) realtime.onState = (c) => (conn.wsConnected = c)
})

const nav = [
  { name: 'tasks', label: '任务', icon: ListChecks, to: '/tasks' },
  { name: 'terminals', label: '终端', icon: TerminalSquare, to: '/terminals' },
  { name: 'todos', label: '待办', icon: CheckSquare, to: '/todos' },
  { name: 'settings', label: '设置', icon: SettingsIcon, to: '/settings' },
]

const dotColor = computed(() => {
  if (conn.status !== 'ready') return 'var(--color-status-error)'
  return conn.wsConnected ? 'var(--color-status-running)' : 'var(--color-status-paused)'
})
const dotLabel = computed(() => {
  if (conn.status === 'connecting') return '连接中…'
  if (conn.status === 'error') return '连接失败'
  return conn.wsConnected ? '已同步' : '重连中…'
})
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <div class="sidebar__brand">
        <span class="sidebar__logo">◆</span>
        <span class="sidebar__title">ShellSync</span>
      </div>

      <nav class="sidebar__nav">
        <RouterLink
          v-for="item in nav"
          :key="item.name"
          :to="item.to"
          class="nav-item"
          active-class="is-active"
        >
          <component :is="item.icon" :size="18" :stroke-width="1.75" />
          <span>{{ item.label }}</span>
        </RouterLink>
      </nav>

      <div class="sidebar__foot">
        <span class="conn-dot" :style="{ background: dotColor }" />
        <span class="conn-label">{{ dotLabel }}</span>
      </div>
    </aside>

    <main class="main">
      <div v-if="conn.status === 'connecting'" class="overlay">正在连接 ShellSync daemon…</div>
      <div v-else-if="conn.status === 'error'" class="overlay overlay--err">
        <strong>无法连接 daemon</strong>
        <p>{{ conn.error }}</p>
        <p style="margin-top: 8px; font-size: var(--font-size-xs); color: var(--color-text-tertiary)">
          请确认 daemon 已编译（daemon/ 下 <code>go build -o bin/ssd</code>），
          或重启本应用。
        </p>
      </div>
      <RouterView v-else />
    </main>
  </div>
</template>

<style scoped>
.app-shell {
  display: flex;
  height: 100%;
}
.sidebar {
  width: 200px;
  flex-shrink: 0;
  background: var(--color-bg-card);
  border-right: 1px solid var(--color-border-light);
  display: flex;
  flex-direction: column;
  padding: 16px 0;
}
.sidebar__brand {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 20px 20px;
}
.sidebar__logo {
  color: var(--color-primary);
  font-size: var(--font-size-lg);
}
.sidebar__title {
  font-size: var(--font-size-md);
  font-weight: var(--font-weight-semibold);
}
.sidebar__nav {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 0 8px;
}
.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border-radius: var(--radius-sm);
  color: var(--color-text-secondary);
  font-size: var(--font-size-base);
  transition: background var(--motion-fast), color var(--motion-fast);
}
.nav-item:hover {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
}
.nav-item.is-active {
  background: var(--color-primary-bg);
  color: var(--color-primary);
  font-weight: var(--font-weight-medium);
}
.sidebar__foot {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 20px 4px;
  color: var(--color-text-tertiary);
  font-size: var(--font-size-xs);
}
.conn-dot {
  width: 8px;
  height: 8px;
  border-radius: var(--radius-pill);
  flex-shrink: 0;
}
.main {
  flex: 1;
  min-width: 0;
  overflow: auto;
}
.overlay {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  height: 100%;
  color: var(--color-text-secondary);
}
.overlay--err {
  text-align: center;
}
.overlay--err code {
  background: var(--color-bg-hover);
  padding: 1px 5px;
  border-radius: var(--radius-sm);
}
</style>
