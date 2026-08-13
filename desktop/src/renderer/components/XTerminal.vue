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

const props = defineProps<{ terminalId: string }>()
const emit = defineEmits<{ (e: 'status', status: string): void }>()

const containerRef = ref<HTMLElement>()
const realtime = useRealtimeStore()

let term: Terminal | null = null
let fit: FitAddon | null = null
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
    fit?.fit()
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
      for (const c of p.chunks ?? []) term?.write(decode(c.contentB64))
    }),
  )
  offs.push(
    ws.on('terminal.output', (p: any) => {
      if (p.terminalId !== props.terminalId) return
      term?.write(decode(p.contentB64))
    }),
  )
  offs.push(
    ws.on('terminal.status', (p: any) => {
      if (p.terminalId !== props.terminalId) return
      emit('status', p.status)
    }),
  )

  term.onData((d) => {
    ws.send('terminal.input', { terminalId: props.terminalId, dataB64: encode(d) })
  })

  ro = new ResizeObserver(() => sendResize())
  ro.observe(el)

  // subscribe + report initial size (server replies with history + live output)
  ws.request('terminal.subscribe', { terminalId: props.terminalId }).catch(() => {})
  sendResize()
})

onBeforeUnmount(() => {
  offs.forEach((o) => o())
  offs = []
  realtime.ws?.send('terminal.unsubscribe', { terminalId: props.terminalId })
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
