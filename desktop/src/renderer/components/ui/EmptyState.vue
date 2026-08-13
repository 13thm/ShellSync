<script setup lang="ts">
/** EmptyState — 空状态占位（克制、无图标炫技）。 */
interface Props {
  text?: string
}
defineProps<Props>()
</script>

<template>
  <div class="empty">
    <slot>{{ text ?? '暂无数据' }}</slot>
  </div>
</template>

<style scoped>
.empty {
  padding: 48px 16px;
  text-align: center;
  color: var(--color-text-tertiary);
  font-size: var(--font-size-sm);
}
</style>
