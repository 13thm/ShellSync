<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { Plus, RotateCcw, RefreshCw, X, Trash2, Search } from 'lucide-vue-next'
import { useTerminalsStore } from '../stores/terminals'
import { useRealtimeStore } from '../stores/realtime'
import AppButton from '../components/ui/AppButton.vue'
import AppListItem from '../components/ui/AppListItem.vue'
import StatusDot from '../components/ui/StatusDot.vue'
import EmptyState from '../components/ui/EmptyState.vue'
import XTerminal from '../components/XTerminal.vue'
import { terminalStatusMeta } from '../utils/status'

const store = useTerminalsStore()
const realtime = useRealtimeStore()

const activeId = ref<string>('')
const search = ref('')
// 刷新会话：自增后作为 XTerminal 的 key，强制重挂载组件，
// 等同「离开终端页再重新进入」——重新订阅、重拉历史、重建本地终端。
const viewEpoch = ref(0)
function refreshTerminal() {
  viewEpoch.value++
}

const availableShells = computed(() => store.shells.filter((s) => s.available))
const active = computed(() => store.items.find((t) => t.id === activeId.value))

/** 按名称/shell 类型过滤（不区分大小写），搜索词为空时显示全部。 */
const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return store.items
  return store.items.filter(
    (t) =>
      t.name?.toLowerCase().includes(q) ||
      t.shellType?.toLowerCase().includes(q) ||
      terminalStatusMeta(t.status).label.includes(q),
  )
})

// ── 新建终端弹窗 ────────────────────────────────────────────
const dialogVisible = ref(false)
const dialogName = ref('')
const dialogShell = ref('')

const PREFERRED = ['pwsh', 'powershell', 'cmd', 'bash', 'zsh']
function defaultShell(): string {
  const list = availableShells.value
  for (const p of PREFERRED) {
    if (list.some((s) => s.type === p)) return p
  }
  return list[0]?.type ?? ''
}
/** 默认名：shell + 时间，如 pwsh 21:35。 */
function defaultName(shellType: string): string {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${shellType} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// shells 是异步加载的：到货后设置弹窗默认选项
watch(availableShells, () => {
  if (!dialogShell.value) dialogShell.value = defaultShell()
}, { immediate: true })

const nameInputRef = ref<HTMLInputElement>()
async function openCreateDialog() {
  dialogShell.value = dialogShell.value || defaultShell()
  dialogName.value = defaultName(dialogShell.value) // 预填默认名，可改
  dialogVisible.value = true
  await nextTick()
  nameInputRef.value?.focus()
  nameInputRef.value?.select()
}
function closeDialog() {
  dialogVisible.value = false
}
async function confirmCreate() {
  const shellType = dialogShell.value || defaultShell() || 'cmd'
  const name = dialogName.value.trim() || defaultName(shellType)
  closeDialog()
  const t = await store.create({ shellType, name })
  activeId.value = t.id
}

// ── 列表操作 ────────────────────────────────────────────────
function select(id: string) {
  activeId.value = id
}
async function restart() {
  if (!active.value) return
  await store.restart(active.value.id)
}
async function closeTerminal() {
  if (!active.value) return
  if (!window.confirm(`删除终端「${active.value.name}」？
进程将被结束，历史日志也会一并删除。`)) return
  await store.remove(active.value.id)
  activeId.value = ''
}
async function deleteItem(id: string) {
  const t = store.items.find((x) => x.id === id)
  if (!t) return
  if (!window.confirm(`删除终端「${t.name}」？
进程将被结束，历史日志也会一并删除。`)) return
  await store.remove(id)
  if (activeId.value === id) activeId.value = ''
}

// ── 重命名（头部标题输入框） ────────────────────────────────
const originalName = ref('')
watch(
  active,
  (t) => {
    originalName.value = t?.name ?? ''
  },
  { immediate: true },
)
async function renameActive() {
  if (!active.value) return
  const name = active.value.name.trim()
  if (!name) {
    active.value.name = originalName.value // 不允许空名，回退
    return
  }
  if (name === originalName.value) return
  await store.rename(active.value.id, name)
  originalName.value = name
}
</script>

<template>
  <div class="term-page">
    <aside class="term-list">
      <header class="term-list__head">
        <h1>终端</h1>
      </header>

      <div class="toolbar">
        <div class="search">
          <Search :size="14" :stroke-width="1.75" class="search__icon" />
          <input v-model="search" placeholder="搜索终端…" />
          <button v-if="search" class="search__clear" title="清空" @click="search = ''">
            <X :size="12" :stroke-width="2" />
          </button>
        </div>
        <AppButton type="primary" @click="openCreateDialog">
          <Plus :size="16" :stroke-width="2" /> 新建
        </AppButton>
      </div>

      <div class="list">
        <EmptyState
          v-if="store.items.length === 0"
          text="还没有终端，新建一个开始"
        />
        <EmptyState
          v-else-if="filtered.length === 0"
          text="没有匹配的终端"
        />
        <AppListItem
          v-for="t in filtered"
          :key="t.id"
          :title="t.name || '未命名终端'"
          :desc="t.shellType + ' · ' + terminalStatusMeta(t.status).label"
          :active="t.id === activeId"
          @click="select(t.id)"
        >
          <template #extra>
            <div class="item-actions">
              <StatusDot :color="terminalStatusMeta(t.status).color" />
              <button class="item-del" title="删除" @click.stop="deleteItem(t.id)">
                <X :size="14" :stroke-width="1.75" />
              </button>
            </div>
          </template>
        </AppListItem>
      </div>
    </aside>

    <section class="term-host">
      <!-- 未选中终端时只显示提示，不显示任何操作按钮 -->
      <div v-if="!active" class="term-host__empty">
        <EmptyState text="选择左侧终端，或新建一个" />
      </div>
      <template v-else>
        <div class="term-host__head">
          <div class="term-host__title">
            <StatusDot :color="terminalStatusMeta(active.status).color" />
            <input
              v-model="active.name"
              class="title-edit"
              :title="active.name"
              @change="renameActive"
              @keyup.enter="($event.target as HTMLInputElement).blur()"
            />
            <span class="muted">· {{ active.shellType }}</span>
          </div>
          <div class="term-host__actions">
            <AppButton type="text" size="small" @click="refreshTerminal">
              <RefreshCw :size="14" :stroke-width="1.75" /> 刷新
            </AppButton>
            <AppButton type="text" size="small" @click="restart">
              <RotateCcw :size="14" :stroke-width="1.75" /> 重启
            </AppButton>
            <AppButton type="text" size="small" danger @click="closeTerminal">
              <Trash2 :size="14" :stroke-width="1.75" /> 删除
            </AppButton>
          </div>
        </div>
        <div class="term-host__body">
          <!-- 会话已退出/崩溃：醒目横幅，避免打字被静默丢弃 -->
          <div v-if="active.status !== 'running'" class="dead-banner">
            <span>
              此终端已{{ active.status === 'crashed' ? '崩溃' : '退出' }}，输入不会被发送。
              可重启后继续，或删除它。
            </span>
            <AppButton type="primary" size="small" @click="restart">重启</AppButton>
            <AppButton type="text" size="small" danger @click="closeTerminal">删除</AppButton>
          </div>
          <XTerminal :key="active.id + ':' + active.status + ':' + active.updatedAt + ':' + viewEpoch" :terminal-id="active.id" />
        </div>
      </template>
    </section>

    <!-- 新建终端弹窗 -->
    <Teleport to="body">
      <div v-if="dialogVisible" class="modal-mask" @click.self="closeDialog">
        <div class="modal">
          <header class="modal__head">
            <h3>新建终端</h3>
            <button class="modal__close" title="取消" @click="closeDialog">
              <X :size="16" :stroke-width="2" />
            </button>
          </header>
          <div class="modal__body">
            <label class="field">
              <span class="field__label">名称</span>
              <input
                ref="nameInputRef"
                v-model="dialogName"
                class="field__input"
                placeholder="终端名称"
                @keyup.enter="confirmCreate"
              />
            </label>
            <label class="field">
              <span class="field__label">Shell</span>
              <select v-model="dialogShell" class="field__input">
                <option v-for="s in availableShells" :key="s.type" :value="s.type">
                  {{ s.type }}{{ s.path ? `（${s.path}）` : '' }}
                </option>
              </select>
            </label>
          </div>
          <footer class="modal__foot">
            <AppButton @click="closeDialog">取消</AppButton>
            <AppButton type="primary" @click="confirmCreate">创建</AppButton>
          </footer>
        </div>
      </div>
    </Teleport>
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

/* 工具栏：搜索 + 新建 */
.toolbar { display: flex; gap: 6px; padding: 0 12px 12px; }
.search {
  flex: 1; min-width: 0; display: flex; align-items: center; gap: 6px;
  height: 32px; padding: 0 8px;
  border: 1px solid var(--color-border-base); border-radius: var(--radius-sm);
  background: var(--color-bg-card);
  transition: border-color var(--motion-fast);
}
.search:focus-within { border-color: var(--color-primary); }
.search__icon { color: var(--color-text-tertiary); flex-shrink: 0; }
.search input {
  flex: 1; min-width: 0; border: none; outline: none; background: transparent;
  font-size: var(--font-size-sm); color: var(--color-text-primary);
  font-family: var(--font-family);
}
.search input::placeholder { color: var(--color-text-placeholder); }
.search__clear {
  display: inline-flex; align-items: center; justify-content: center;
  width: 16px; height: 16px; border: none; border-radius: var(--radius-sm);
  background: transparent; color: var(--color-text-tertiary); cursor: pointer;
  flex-shrink: 0;
}
.search__clear:hover { color: var(--color-text-primary); background: var(--color-bg-hover); }

.list { flex: 1; overflow: auto; padding: 0 8px 12px; }
.item-actions { display: flex; align-items: center; gap: 4px; }
.item-del {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px; height: 22px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  opacity: 0;
  transition: opacity var(--motion-fast), background var(--motion-fast), color var(--motion-fast);
}
.list :deep(.list-item):hover .item-del { opacity: 1; }
.item-del:hover {
  background: var(--color-danger-bg);
  color: var(--color-danger-hover);
}

.term-host { flex: 1; min-width: 0; display: flex; flex-direction: column; background: var(--color-bg-page); }
.term-host__empty { flex: 1; display: flex; align-items: center; justify-content: center; }
.term-host__head {
  height: 44px; flex-shrink: 0; display: flex; align-items: center; justify-content: space-between;
  padding: 0 16px; border-bottom: 1px solid var(--color-border-light); background: var(--color-bg-card);
}
.term-host__title { display: flex; align-items: center; gap: 8px; font-size: var(--font-size-base); font-weight: var(--font-weight-medium); }
.term-host__actions { display: flex; gap: 4px; }
.muted { color: var(--color-text-tertiary); font-weight: var(--font-weight-regular); font-size: var(--font-size-sm); }
.title-edit {
  width: 220px; height: 26px; padding: 0 6px;
  border: 1px solid transparent; border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-primary);
  font-size: var(--font-size-base);
  font-weight: var(--font-weight-medium);
  font-family: var(--font-family);
}
.title-edit:hover { border-color: var(--color-border-base); }
.title-edit:focus {
  outline: none; border-color: var(--color-primary);
  background: var(--color-bg-card);
}
.term-host__body { flex: 1; min-height: 0; display: flex; flex-direction: column; }
.dead-banner {
  display: flex; align-items: center; gap: 10px;
  padding: 8px 16px;
  background: var(--color-danger-bg, #fdf0ef);
  color: var(--color-danger, #e45656);
  font-size: var(--font-size-sm);
  border-bottom: 1px solid var(--color-border-light);
  flex-shrink: 0;
}
.dead-banner span { flex: 1; }
.term-host__body :deep(.xterm-host) { flex: 1; }

/* 新建终端弹窗 */
.modal-mask {
  position: fixed; inset: 0; z-index: 100;
  background: rgba(31, 35, 41, 0.35);
  display: flex; align-items: center; justify-content: center;
}
.modal {
  width: 380px; max-width: calc(100vw - 48px);
  background: var(--color-bg-card);
  border-radius: var(--radius-md);
  border: 1px solid var(--color-border-light);
  box-shadow: 0 12px 32px rgba(31, 35, 41, 0.16);
  overflow: hidden;
}
.modal__head {
  display: flex; align-items: center; justify-content: space-between;
  padding: 14px 16px 10px;
}
.modal__head h3 { font-size: var(--font-size-md); font-weight: var(--font-weight-semibold); }
.modal__close {
  display: inline-flex; align-items: center; justify-content: center;
  width: 24px; height: 24px; border: none; border-radius: var(--radius-sm);
  background: transparent; color: var(--color-text-tertiary); cursor: pointer;
}
.modal__close:hover { background: var(--color-bg-hover); color: var(--color-text-primary); }
.modal__body { padding: 4px 16px 8px; display: flex; flex-direction: column; gap: 12px; }
.field { display: flex; flex-direction: column; gap: 6px; }
.field__label { font-size: var(--font-size-sm); color: var(--color-text-secondary); }
.field__input {
  height: 34px; padding: 0 10px;
  border: 1px solid var(--color-border-base); border-radius: var(--radius-sm);
  background: var(--color-bg-card); color: var(--color-text-primary);
  font-size: var(--font-size-base); font-family: var(--font-family);
}
.field__input:focus { outline: none; border-color: var(--color-primary); }
.modal__foot {
  display: flex; justify-content: flex-end; gap: 8px;
  padding: 12px 16px 14px;
}
</style>
