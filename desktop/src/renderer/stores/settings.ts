import { defineStore } from 'pinia'
import { ref } from 'vue'
import { settingsApi } from '../api'

export const useSettingsStore = defineStore('settings', () => {
  const kv = ref<Record<string, string>>({})

  async function load() {
    kv.value = await settingsApi.getAll()
  }

  async function patch(entries: Record<string, string>) {
    await settingsApi.patch(entries)
    kv.value = { ...kv.value, ...entries }
  }

  function get(key: string, fallback = ''): string {
    return kv.value[key] ?? fallback
  }

  return { kv, load, patch, get }
})
