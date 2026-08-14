<script setup lang="ts">
/**
 * XTerminal — xterm.js 封装，经 WS 接入 daemon 的实时终端流。
 * 进入即回填历史（terminal.history），实时接收 terminal.output，
 * 键盘输入转发 terminal.input，窗口变化转发 terminal.resize。
 */
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'
import { useRealtimeStore } from '../stores/realtime'
import { useTerminalsStore } from '../stores/terminals'

/**
 * 同一 terminalId 的订阅租约：Vue 对带 key 的组件更新是「新挂载 → 旧卸载」，
 * 若旧组件卸载时立即 unsubscribe，会把新组件刚刚建立的订阅在服务端删掉。
 * 用计数器推迟卸载 unsubscribe：短窗口内有新租约接管则跳过。
 */
const subLeases = new Map<string, number>()
function acquireLease(id: string) {
  subLeases.set(id, (subLeases.get(id) ?? 0) + 1)
}
function releaseLease(id: string): boolean {
  const n = (subLeases.get(id) ?? 0) - 1
  if (n <= 0) {
    subLeases.delete(id)
    return n === 0 // 从 1 → 0：真正无人观看，需要 unsubscribe
  }
  subLeases.set(id, n)
  return false // 还有新租约在看，跳过 unsubscribe
}

const props = defineProps<{ terminalId: string }>()
const emit = defineEmits<{ (e: 'status', status: string): void }>()

const containerRef = ref<HTMLElement>()
const realtime = useRealtimeStore()
const terminals = useTerminalsStore()

let term: Terminal | null = null
let fit: FitAddon | null = null
let suppressFitOnce = false // 对齐远端尺寸后跳过一次本端 fit，避免来回抢
let ro: ResizeObserver | null = null
let offs: Array<() => void> = []

// 浅色主题，贴合设计系统（§二 坚持浅色，不搞暗黑科技感）
const LIGHT_THEME = {
  background: '#ffffff',
  foreground: '#1f2329',
  cursor: '#3ba776',
  cursorAccent: '#ffffff',
  selectionBackground: '#ebf5ef',
  black: '#1f2329',
  red: '#e45656',
  green: '#3ba776',
  yellow: '#e0a13c',
  blue: '#4b8af0',
  magenta: '#a05eb5',
  cyan: '#3ba776',
  white: '#c0c4cc',
  brightBlack: '#8a909a',
  brightRed: '#e45656',
  brightGreen: '#3ba776',
  brightYellow: '#e0a13c',
  brightBlue: '#4b8af0',
  brightMagenta: '#a05eb5',
  brightCyan: '#3ba776',
  brightWhite: '#1f2329',
}

function decode(b64: string): Uint8Array {
  const bin = atob(b64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  return bytes
}

function encode(s: string): string {
  const bytes = new TextEncoder().encode(s)
  let bin = ''
  for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i])
  return btoa(bin)
}

function sendResize() {
  if (!term || !realtime.ws) return
  try {
    if (suppressFitOnce) {
      suppressFitOnce = false // 远端对齐触发的一次 ResizeObserver 回调，跳过 fit
    } else {
      fit?.fit()
    }
  } catch {
    /* ignore */
  }
  realtime.ws.send('terminal.resize', {
    terminalId: props.terminalId,
    cols: term.cols,
    rows: term.rows,
  })
}

onMounted(() => {
  const el = containerRef.value!
  term = new Terminal({
    fontFamily: "'JetBrains Mono', 'SF Mono', 'Cascadia Code', Consolas, monospace",
    fontSize: 13,
    lineHeight: 1.3,
    cursorBlink: true,
    theme: LIGHT_THEME,
    allowProposedApi: true,
  })
  fit = new FitAddon()
  term.loadAddon(fit)
  term.loadAddon(new WebLinksAddon())
  term.open(el)

  const ws = realtime.ws
  if (!ws) return

  offs.push(
    ws.on('terminal.history', (p: any) => {
      if (p.terminalId !== props.terminalId) return
      // 只回放 stdout；stdin 已由 PTY 回显，直接写入会双显
      for (const c of p.chunks ?? []) {
        if (c.direction && c.direction !== 'stdout') continue
        term?.write(decode(c.contentB64))
      }
    }),
  )
  offs.push(
    ws.on('terminal.output', (p: any) => {
      if (p.terminalId !== props.terminalId) return
      if (p.direction && p.direction !== 'stdout') return
      term?.write(decode(p.contentB64))
    }),
  )
  offs.push(
    ws.on('terminal.status', (p: any) => {
      if (p.terminalId !== props.terminalId) return
      // 同步到 store：列表状态点/终端面板横幅立刻反映
      terminals.patchStatus(props.terminalId, p.status)
      emit('status', p.status)
    }),
  )
  offs.push(
    ws.on('terminal.size', (p: any) => {
      if (p.terminalId !== props.terminalId) return
      // tmux attach 语义：另一端（手机）接管了 PTY 尺寸时，本地对齐相同网格，
      // 保证两端渲染坐标系一致（内容居左，右侧留白）。
      if (
        term &&
        p.cols > 0 &&
        p.rows > 0 &&
        (term.cols !== p.cols || term.rows !== p.rows)
      ) {
        suppressFitOnce = true // 对齐期间不与本端 fit 抢夺
        term.resize(p.cols, p.rows)
      }
    }),
  )

  term.onData((d) => {
    ws.send('terminal.input', { terminalId: props.terminalId, dataB64: encode(d) })
  })

  ro = new ResizeObserver(() => sendResize())
  ro.observe(el)

  // subscribe + report initial size (server replies with history + live output)
  acquireLease(props.terminalId)
  ws.request('terminal.subscribe', { terminalId: props.terminalId }).catch(() => {})
  sendResize()
})

onBeforeUnmount(() => {
  offs.forEach((o) => o())
  offs = []
  // 推迟到宏任务之后：Vue 同一次渲染中「旧卸载→新挂载」几乎同时发生，
  // 延迟足以让新组件的 subscribe 先到达服务端；若新租约已接管则直接跳过
  const id = props.terminalId
  window.setTimeout(() => {
    if (releaseLease(id)) {
      realtime.ws?.send('terminal.unsubscribe', { terminalId: id })
    }
  }, 120)
  ro?.disconnect()
  term?.dispose()
  term = null
})
</script>

<template>
  <div ref="containerRef" class="xterm-host selectable" />
</template>

<style scoped>
.xterm-host {
  height: 100%;
  width: 100%;
  padding: 8px;
  background: var(--color-bg-card);
  overflow: hidden;
}
.xterm-host :deep(.xterm) {
  padding: 0;
  height: 100%;
}
.xterm-host :deep(.xterm-viewport) {
  /* 浅色滚动条 */
  background-color: transparent !important;
}
</style>
