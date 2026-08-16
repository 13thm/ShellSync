<script setup lang="ts">
/**
 * XTerminal — xterm.js 封装，经 WS 接入 daemon 的实时终端流。
 * 进入即回填历史（terminal.history），实时接收 terminal.output，
 * 键盘输入转发 terminal.input，窗口变化转发 terminal.resize。
 */
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
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
// 回放链：history 可能分多页到达，且 term.write 是异步入队解析的，
// resize 标记必须等前面所有字节写入完成后才能应用，否则字节会在
// 错误的网格尺寸下解析。用 Promise 链把「写入→等解析完成→改尺寸」
// 严格串行化。
let replayChain: Promise<void> = Promise.resolve()
// resize 防抖：窗口拖拽时 ResizeObserver 每帧都触发，逐帧上报会把
// PTY 网格抖来抖去（ConPTY 反复重排，观感很乱）。只上报稳定后的尺寸。
let resizeTimer: number | null = null
// 历史回放进行中：此期间不上报本端 resize —— 回放还没写完，PTY 就被改
// 尺寸会让 ConPTY 重排输出，新字节与回放流交错，画面二次污染。
let awaitingReplay = false

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

/** 把剪贴板内容作为终端输入发送（粘贴） */
function sendPaste(text: string) {
  if (!realtime.ws || !text || !term) return
  // PTY 期望 \r 作为回车：CRLF / LF / CR 统一转为 \r
  const data = text.replace(/\r\n?/g, '\r')
  // 走 xterm 的 paste 通道而非直接发送：当远端程序（pi / claude code 等）
  // 开启 bracketed paste mode 时，xterm 会自动用 ESC[200~…ESC[201~ 把整段
  // 内容包裹起来，程序将其识别为「一次粘贴」而非逐字符按键——
  // 空格、换行不会被拆开逐段执行。
  term.paste(data)
}

function pasteFromClipboard() {
  navigator.clipboard
    .readText()
    .then(sendPaste)
    .catch(() => {
      /* 剪贴板权限被拒时静默失败，不打断终端 */
    })
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
  const { terminalId } = props
  const cols = term.cols
  const rows = term.rows
  if (resizeTimer) window.clearTimeout(resizeTimer)
  resizeTimer = window.setTimeout(() => {
    if (awaitingReplay) return // 回放中不上报；回放完成后会补报一次
    realtime.ws?.send('terminal.resize', { terminalId, cols, rows })
  }, 150)
}

/** 串行回放一页历史块：resize 标记在前面所有输出写入完成后应用 */
function replayChunksSequentially(chunks: any[]) {
  replayChain = replayChain.then(async () => {
    for (const c of chunks) {
      if (!term) return
      if (c.direction === 'resize') {
        try {
          const size = JSON.parse(new TextDecoder().decode(decode(c.contentB64)))
          if (term && size.cols > 0 && size.rows > 0) {
            suppressFitOnce = true // 回放期间不与本端 fit 抢夺
            term.resize(size.cols, size.rows)
          }
        } catch {
          /* 忽略非法 resize 标记 */
        }
        continue
      }
      if (c.direction && c.direction !== 'stdout') continue
      const data = decode(c.contentB64)
      // 等 xterm 解析完这批字节再继续，保证后续 resize 在正确的位置生效
      await new Promise<void>((resolve) => {
        if (!term) return resolve()
        term.write(data, () => resolve())
      })
    }
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
    // 回放整个会话历史时，默认 1000 行的 scrollback 会把较早的内容
    // 直接截掉，看不到之前发过的命令和输出 —— 加大回滚缓冲。
    scrollback: 10000,
  })
  fit = new FitAddon()
  term.loadAddon(fit)
  term.loadAddon(new WebLinksAddon())
  term.open(el)

  // 粘贴支持：拦截 Ctrl+V / Ctrl+Shift+V（mac 上 Cmd+V），交给剪贴板 API 处理。
  // 返回 false 阻止 xterm 继续处理，避免把 ^V 字面量写进终端。
  // 输入法合成中（isComposing，Windows 下 keyCode=229）不拦截，否则会打断
  // 中文拼音输入，导致候选词丢失、输入不进去。
  term.attachCustomKeyEventHandler((ev) => {
    if (ev.type !== 'keydown') return true
    if ((ev as KeyboardEvent).isComposing) return true
    const ctrl = ev.ctrlKey || ev.metaKey
    if (ctrl && !ev.altKey && ev.code === 'KeyV') {
      pasteFromClipboard()
      return false
    }
    return true
  })

  // 右键粘贴（终端软件惯例）
  el.addEventListener('contextmenu', (ev) => {
    ev.preventDefault()
    pasteFromClipboard()
  })

  const ws = realtime.ws
  if (!ws) return

  let pendingResub = false

  offs.push(
    ws.on('terminal.history', (p: any) => {
      if (p.terminalId !== props.terminalId) return
      // 只回放 stdout；stdin 已由 PTY 回显，直接写入会双显；
      // resize 是尺寸标记：按当时的网格重设 xterm（严格串行，见
      // replayChunksSequentially），否则 Windows ConPTY 的绝对光标定位
      // （ESC[r;cH，行号绑定发出时的屏幕尺寸）会在错误尺寸下落到屏幕
      // 中部，后续输出原地覆盖，滚动缓冲里大部分历史就丢了。
      replayChunksSequentially(p.chunks ?? [])
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

  // subscribe + report initial size。订阅 ack 在服务端回放完全部历史之后
  // 才返回 —— 等它再上报本端尺寸，避免回放中途改 PTY 网格造成交错污染。
  acquireLease(props.terminalId)
  awaitingReplay = true
  ws
    .request('terminal.subscribe', { terminalId: props.terminalId })
    .catch(() => {})
    .finally(() => {
      awaitingReplay = false
      sendResize() // 回放完成后对齐本端尺寸（当前实际 cols/rows）
    })
  sendResize()

  // WS 重连（wsSession 变化）后，服务端的订阅已随旧连接销毁 ——
  // 原地重新订阅而不是重挂载组件：重挂载会重建 DOM/丢焦点，且上万行
  // 历史全量重放会卡住主线程数秒（表现为「点不动/输入不了字」）。
  offs.push(
    watch(
      () => realtime.wsSession,
      () => {
        if (pendingResub) return
        pendingResub = true
        requestAnimationFrame(() => {
          pendingResub = false
          if (!term) return
          term.reset()
          replayChain = Promise.resolve()
          awaitingReplay = true
          realtime.ws
            ?.request('terminal.subscribe', { terminalId: props.terminalId })
            .catch(() => {})
            .finally(() => {
              awaitingReplay = false
              sendResize()
            })
        })
      },
    ),
  )
})

onBeforeUnmount(() => {
  offs.forEach((o) => o())
  offs = []
  if (resizeTimer) window.clearTimeout(resizeTimer)
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
