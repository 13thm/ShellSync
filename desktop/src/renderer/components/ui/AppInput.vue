<script setup lang="ts">
/**
 * AppInput — 标准输入框
 * 规范：§六/§八 白底 1px 边框，聚焦主色边框 + 3px 淡光圈（不用浓重 glow）。
 */
interface Props {
  modelValue?: string
  placeholder?: string
  type?: string
  disabled?: boolean
}
const props = defineProps<Props>()
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()
</script>

<template>
  <input
    class="app-input"
    :class="{ 'is-disabled': disabled }"
    :type="type ?? 'text'"
    :value="modelValue"
    :placeholder="placeholder"
    :disabled="disabled"
    @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
  />
</template>

<style scoped>
.app-input {
  width: 100%;
  height: 32px;
  padding: 0 12px;
  font-family: var(--font-family);
  font-size: var(--font-size-base);
  color: var(--color-text-primary);
  background: var(--color-bg-card);
  border: 1px solid var(--color-border-base);
  border-radius: var(--radius-sm);
  outline: none;
  transition: border-color var(--motion-fast), box-shadow var(--motion-fast);
}
.app-input::placeholder {
  color: var(--color-text-placeholder);
}
.app-input:hover {
  border-color: var(--color-border-strong);
}
.app-input:focus {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px var(--color-primary-bg); /* 淡光圈，非浓重 glow */
}
.is-disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
