<script setup lang="ts">
import { computed, ref } from 'vue'
import { Plus, RotateCcw, X } from 'lucide-vue-next'
import { useTerminalsStore } from '../stores/terminals'
import AppButton from '../components/ui/AppButton.vue'
import AppListItem from '../components/ui/AppListItem.vue'
import StatusDot from '../components/ui/StatusDot.vue'
import EmptyState from '../components/ui/EmptyState.vue'
import XTerminal from '../components/XTerminal.vue'
import { terminalStatusMeta } from '../utils/status'

const store = useTerminalsStore()

const activeId = ref<string>('')
const newShell = ref('')

const availableShells = computed(() => store.shells.filter((s) => s.available))
const active = computed(() => store.items.find((t) => t.id === activeId.value))

async function createTerminal() {
  const shellType = newShell.value || availableShells.value[0]?.type || 'cmd'
  const t = await store.create({ shellType })
  activeId.value = t.id
}
function select(id: string) {
  activeId.value = id
}
async function restart() {
  if (!active.value) return
  await store.restart(active.value.id)
}
async function closeTerminal() {
  if (!active.value) return
  await store.remove(active.value.id)
  activeId.value = ''
}
</script>

<template>
  <div class="term-page">
    <aside class="term-list">
      <header class="term-list__head">
        <h1>终端</h1>
      </header>

      <div class="create">
        <select v-model="newShell" class="sel">
          <option v-for="s in availableShells" :key="s.type" :value="s.type">{{ s.type }}</option>
        </select>
        <AppButton type="primary" @click="createTerminal">
          <Plus :size="16" :stroke-width="2" /> 新建
        </AppButton>
      </div>

      <div class="list">
        <EmptyState v-if="store.items.length === 0" text="还没有终端，新建一个开始" />
        <AppListItem
          v-for="t in store.items"
          :key="t.id"
          :title="t.name"
          :desc="t.shellType + ' · ' + terminalStatusMeta(t.status).label"
          :active="t.id === activeId"
          @click="select(t.id)"
        >
          <template #extra>
            <StatusDot :color="terminalStatusMeta(t.status).color" />
          </template>
        </AppListItem>
      </div>
    </aside>

    <section class="term-host">
      <header v-if="active" class="term-host__head">
        <div class="term-host__title">
          <StatusDot :color="terminalStatusMeta(active.status).color" />
          <span>{{ active.name }}</span>
          <span class="muted">· {{ active.shellType }}</span>
        </div>
        <div class="term-host__actions">
          <AppButton type="text" size="small" @click="restart">
            <RotateCcw :size="14" :stroke-width="1.75" /> 重启
          </AppButton>
          <AppButton type="text" size="small" danger @click="closeTerminal">
            <X :size="14" :stroke-width="1.75" /> 关闭
          </AppButton>
        </div>
      </header>
      <div class="term-host__body">
        <EmptyState v-if="!active" text="选择左侧终端，或新建一个" />
        <XTerminal v-else :key="active.id" :terminal-id="active.id" />
      </div>
    </section>
  </div>
</template>

<style scoped>
.term-page { display: flex; height: 100%; }
.term-list {
  width: 240px; flex-shrink: 0; border-right: 1px solid var(--color-border-light);
  background: var(--color-bg-card); display: flex; flex-direction: column;
}
.term-list__head { padding: 20px 16px 12px; }
.term-list__head h1 { font-size: var(--font-size-xl); font-weight: var(--font-weight-semibold); }
.create { display: flex; gap: 6px; padding: 0 12px 12px; }
.sel {
  flex: 1; height: 32px; border: 1px solid var(--color-border-base); border-radius: var(--radius-sm);
  background: var(--color-bg-card); color: var(--color-text-primary); font-size: var(--font-size-sm); padding: 0 8px;
}
.list { flex: 1; overflow: auto; padding: 0 8px 12px; }
.term-host { flex: 1; min-width: 0; display: flex; flex-direction: column; background: var(--color-bg-page); }
.term-host__head {
  height: 44px; flex-shrink: 0; display: flex; align-items: center; justify-content: space-between;
  padding: 0 16px; border-bottom: 1px solid var(--color-border-light); background: var(--color-bg-card);
}
.term-host__title { display: flex; align-items: center; gap: 8px; font-size: var(--font-size-base); font-weight: var(--font-weight-medium); }
.term-host__actions { display: flex; gap: 4px; }
.muted { color: var(--color-text-tertiary); font-weight: var(--font-weight-regular); font-size: var(--font-size-sm); }
.term-host__body { flex: 1; min-height: 0; }
</style>
