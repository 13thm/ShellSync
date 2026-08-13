<script setup lang="ts">
/**
 * AppButton — 三档按钮（主 / 次要 / 文字）
 * 规范：§九 按钮。hover 只改色/底，不放大不加阴影；危险操作用红色文字按钮。
 */
interface Props {
  type?: 'primary' | 'default' | 'text'
  danger?: boolean
  size?: 'small' | 'default'
  disabled?: boolean
}
const { type = 'default', danger = false, size = 'default', disabled = false } =
  defineProps<Props>()
</script>

<template>
  <button
    class="app-btn"
    :class="[
      `app-btn--${type}`,
      { 'is-danger': danger, 'is-small': size === 'small', 'is-disabled': disabled },
    ]"
    :disabled="disabled"
  >
    <slot />
  </button>
</template>

<style scoped>
.app-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  height: 32px;
  padding: 0 16px;
  font-family: var(--font-family);
  font-size: var(--font-size-base);
  font-weight: var(--font-weight-regular);
  line-height: 1;
  border: 1px solid var(--color-border-base);
  border-radius: var(--radius-sm);
  background: var(--color-bg-card);
  color: var(--color-text-primary);
  cursor: pointer;
  transition: background var(--motion-fast), border-color var(--motion-fast),
    color var(--motion-fast);
}
.app-btn:hover {
  background: var(--color-bg-hover);
}
.app-btn:active {
  background: var(--color-bg-active);
}

/* 主按钮：主色实心 + 反白字（每屏最多 1 个） */
.app-btn--primary {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: var(--color-text-inverse);
}
.app-btn--primary:hover {
  background: var(--color-primary-hover);
  border-color: var(--color-primary-hover);
}
.app-btn--primary:active {
  background: var(--color-primary-active);
}

/* 文字按钮：无背景无边框，主色字 */
.app-btn--text {
  border: none;
  background: transparent;
  color: var(--color-primary);
  padding: 0 8px;
}
.app-btn--text:hover {
  background: var(--color-bg-hover);
  color: var(--color-primary-hover);
}

/* 危险：红色文字按钮，不加红底（避免误触视觉压迫） */
.is-danger {
  color: var(--color-danger);
  border-color: transparent;
  background: transparent;
}
.is-danger:hover {
  background: var(--color-danger-bg);
  color: var(--color-danger-hover);
}

.is-small {
  height: 26px;
  padding: 0 10px;
  font-size: var(--font-size-sm);
}

.is-disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.is-disabled:hover {
  background: var(--color-bg-card);
}
</style>
