<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import QRCode from 'qrcode'
import { Plus, Trash2 } from 'lucide-vue-next'
import { systemApi, devicesApi } from '../api'
import { useSettingsStore } from '../stores/settings'
import { useTerminalsStore } from '../stores/terminals'
import AppButton from '../components/ui/AppButton.vue'
import AppCard from '../components/ui/AppCard.vue'
import AppListItem from '../components/ui/AppListItem.vue'
import EmptyState from '../components/ui/EmptyState.vue'
import { formatTime } from '../utils/status'

const settings = useSettingsStore()
const terminals = useTerminalsStore()

// pairing
const pairCode = ref('')
const qrDataUrl = ref('')
const qrExpires = ref(0)
const pairError = ref('')
let pairTimer: number | null = null

async function genPair() {
  pairError.value = ''
  try {
    const res = await systemApi.pairInit()
    pairCode.value = res.pairingCode
    qrExpires.value = res.expiresAt
    qrDataUrl.value = await QRCode.toDataURL(res.qrPayload, { margin: 1, width: 220 })
  } catch (e: any) {
    pairError.value = e?.message ?? String(e)
  }
}
function pairCountdown(): string {
  if (!qrExpires.value) return ''
  const s = Math.max(0, Math.floor((qrExpires.value - Date.now()) / 1000))
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}
onBeforeUnmount(() => {
  if (pairTimer) window.clearInterval(pairTimer)
})

// devices
const devices = ref<Awaited<ReturnType<typeof devicesApi.list>>>([])
async function loadDevices() {
  devices.value = await devicesApi.list().catch(() => [])
}
onMounted(loadDevices)
async function revoke(id: string) {
  await devicesApi.revoke(id)
  await loadDevices()
}

// general settings
const defaultShell = ref(settings.get('default_shell') || (terminals.shells[0]?.type ?? 'cmd'))
const logRetention = ref(Number(settings.get('log_retention_days') || 7))
async function saveGeneral() {
  await settings.patch({
    default_shell: defaultShell.value,
    log_retention_days: String(logRetention.value),
  })
}
</script>

<template>
  <div class="page">
    <header class="page__head">
      <h1>设置</h1>
    </header>

    <AppCard title="同步 · 配对手机">
      <p class="card-desc">用手机端 ShellSync 扫描二维码，配对后即可远程查看与操控本机终端（需同一局域网）。</p>
      <div class="pair">
        <div v-if="qrDataUrl" class="pair__qr">
          <img :src="qrDataUrl" alt="配对二维码" />
          <div class="pair__code">配对码：<strong>{{ pairCode }}</strong></div>
          <div class="pair__ttl">剩余 {{ pairCountdown() }}</div>
        </div>
        <div class="pair__actions">
          <AppButton type="primary" @click="genPair">
            <Plus :size="16" :stroke-width="2" /> {{ qrDataUrl ? '重新生成' : '生成配对码' }}
          </AppButton>
          <div v-if="pairError" class="err">{{ pairError }}</div>
        </div>
      </div>
    </AppCard>

    <AppCard title="已配对设备">
      <EmptyState v-if="devices.length === 0" text="还没有配对的设备" />
      <AppListItem
        v-for="d in devices"
        :key="d.id"
        :title="d.name + ' · ' + d.platform"
        :desc="d.revoked ? '已吊销' : '最后在线 ' + formatTime(d.lastSeenAt)"
        :disabled="d.revoked"
      >
        <template #extra>
          <AppButton v-if="!d.revoked" type="text" size="small" danger @click="revoke(d.id)">
            <Trash2 :size="14" :stroke-width="1.75" /> 吊销
          </AppButton>
        </template>
      </AppListItem>
    </AppCard>

    <AppCard title="终端">
      <div class="form-row">
        <label>默认 Shell</label>
        <select v-model="defaultShell" class="sel" @change="saveGeneral">
          <option v-for="s in terminals.shells" :key="s.type" :value="s.type" :disabled="!s.available">
            {{ s.type }}{{ s.available ? '' : '（不可用）' }}
          </option>
        </select>
      </div>
      <div class="form-row">
        <label>日志保留（天）</label>
        <input
          v-model.number="logRetention"
          type="number"
          min="0"
          class="num"
          @change="saveGeneral"
        />
        <span class="muted">0 = 永久保留</span>
      </div>
    </AppCard>

    <AppCard title="关于" flat>
      <div class="about">ShellSync Desktop · v0.1.0</div>
    </AppCard>
  </div>
</template>

<style scoped>
.page { padding: 24px 28px; max-width: 720px; }
.page__head { margin-bottom: 20px; }
.page__head h1 { font-size: var(--font-size-2xl); font-weight: var(--font-weight-semibold); }
:deep(.app-card) { margin-bottom: 16px; }
.card-desc { color: var(--color-text-secondary); font-size: var(--font-size-sm); margin-bottom: 16px; }
.pair { display: flex; gap: 24px; align-items: center; flex-wrap: wrap; }
.pair__qr { text-align: center; }
.pair__qr img { border: 1px solid var(--color-border-base); border-radius: var(--radius-md); }
.pair__code { margin-top: 8px; font-size: var(--font-size-sm); }
.pair__code strong { color: var(--color-primary); letter-spacing: 2px; }
.pair__ttl { font-size: var(--font-size-xs); color: var(--color-text-tertiary); margin-top: 2px; }
.err { color: var(--color-danger); font-size: var(--font-size-sm); margin-top: 8px; }
.form-row { display: flex; align-items: center; gap: 12px; padding: 10px 0; }
.form-row label { width: 120px; color: var(--color-text-secondary); font-size: var(--font-size-sm); }
.sel, .num {
  height: 32px; border: 1px solid var(--color-border-base); border-radius: var(--radius-sm);
  background: var(--color-bg-card); color: var(--color-text-primary); font-size: var(--font-size-sm); padding: 0 10px;
}
.num { width: 90px; }
.muted { color: var(--color-text-tertiary); font-size: var(--font-size-xs); }
.about { color: var(--color-text-tertiary); font-size: var(--font-size-sm); }
</style>
