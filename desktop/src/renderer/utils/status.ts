/** Status → 文案/颜色映射。颜色用 tokens.css 里的状态色变量。 */

export function taskStatusMeta(status: string): { label: string; color: string } {
  switch (status) {
    case 'pending':
      return { label: '未开始', color: 'var(--color-status-done)' } // 灰，不抢眼
    case 'running':
      return { label: '进行中', color: 'var(--color-status-running)' }
    case 'paused':
      return { label: '已暂停', color: 'var(--color-status-paused)' }
    case 'done':
      return { label: '已完成', color: 'var(--color-status-done)' }
    default:
      return { label: status, color: 'var(--color-text-tertiary)' }
  }
}

export function terminalStatusMeta(status: string): { label: string; color: string } {
  switch (status) {
    case 'running':
      return { label: '运行中', color: 'var(--color-status-running)' }
    case 'exited':
      return { label: '已退出', color: 'var(--color-status-done)' }
    case 'crashed':
      return { label: '已崩溃', color: 'var(--color-status-error)' }
    default:
      return { label: status, color: 'var(--color-text-tertiary)' }
  }
}

/** Allowed next statuses for the state-machine action buttons. */
export function taskTransitions(status: string): { to: string; label: string }[] {
  switch (status) {
    case 'pending':
      return [{ to: 'running', label: '开始' }]
    case 'running':
      return [
        { to: 'paused', label: '暂停' },
        { to: 'done', label: '完成' },
      ]
    case 'paused':
      return [
        { to: 'running', label: '继续' },
        { to: 'done', label: '完成' },
      ]
    case 'done':
      return [{ to: 'running', label: '重新开始' }]
    default:
      return []
  }
}

export function formatTime(ms: number): string {
  if (!ms) return ''
  const d = new Date(ms)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(
    d.getHours(),
  )}:${pad(d.getMinutes())}`
}
