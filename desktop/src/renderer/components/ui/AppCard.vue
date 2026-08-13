<script setup lang="ts">
/**
 * AppCard — 信息分组卡片
 * 规范：§六 静态元素用边框不加阴影；卡片 radius-md + 1px 常规边框。
 */
interface Props {
  title?: string
  flat?: boolean
}
defineProps<Props>()
</script>

<template>
  <section class="app-card" :class="{ 'is-flat': flat }">
    <header v-if="title || $slots.header" class="app-card__header">
      <slot name="header"><h3>{{ title }}</h3></slot>
    </header>
    <div class="app-card__body"><slot /></div>
  </section>
</template>

<style scoped>
.app-card {
  background: var(--color-bg-card);
  border: 1px solid var(--color-border-base);
  border-radius: var(--radius-md);
}
.app-card.is-flat {
  border: none;
  background: transparent;
}
.app-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--color-border-light);
}
.app-card__header h3 {
  margin: 0;
  font-size: var(--font-size-lg);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-primary);
}
.app-card__body {
  padding: 16px;
}
</style>
