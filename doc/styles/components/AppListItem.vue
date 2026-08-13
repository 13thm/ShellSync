<script setup lang="ts">
/**
 * AppListItem — 通用列表项
 * 规范：§八 各面板列表项；图标 + 主标题 + 灰色描述 + 右侧 extra 槽。
 * 用于：任务列表、终端列表、待办、设置项、配对设备。
 */
interface Props {
  title: string
  desc?: string
  icon?: string
  disabled?: boolean
  active?: boolean
}
defineProps<Props>()
const emit = defineEmits<{ (e: 'click'): void; (e: 'contextmenu'): void }>()
</script>

<template>
  <div
    class="list-item"
    :class="{ 'is-disabled': disabled, 'is-active': active }"
    @click="!disabled && emit('click')"
    @contextmenu.prevent="emit('contextmenu')"
  >
    <span v-if="icon" class="list-item__icon">
      <img :src="icon" alt="" />
    </span>
    <span class="list-item__main">
      <span class="list-item__title">{{ title }}</span>
      <span v-if="desc" class="list-item__desc">{{ desc }}</span>
    </span>
    <span class="list-item__extra"><slot name="extra" /></span>
  </div>
</template>

<style scoped>
.list-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 16px;
  cursor: pointer;
  transition: background var(--motion-fast);
}
.list-item:hover {
  background: var(--color-bg-hover);
}
.list-item.is-active {
  background: var(--color-bg-selected);
}
.list-item.is-disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.list-item__icon img {
  width: 24px;
  height: 24px;
  border-radius: var(--radius-sm);
  display: block;
}
.list-item__main {
  display: flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
}
.list-item__title {
  font-size: var(--font-size-base);
  color: var(--color-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.list-item__desc {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-top: 2px;
}
</style>
