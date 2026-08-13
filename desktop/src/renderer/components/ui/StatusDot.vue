<script setup lang="ts">
/** StatusDot — 状态点，颜色用功能/状态色 token。 */
interface Props {
  color: string // a CSS color / var()
  size?: number
}
defineProps<Props>()
</script>

<template>
  <span class="status-dot" :style="{ background: color, width: (size ?? 8) + 'px', height: (size ?? 8) + 'px' }" />
</template>

<style scoped>
.status-dot {
  display: inline-block;
  border-radius: var(--radius-pill);
  flex-shrink: 0;
}
</style>
