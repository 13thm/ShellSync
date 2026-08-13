import { defineStore } from 'pinia'
import { ref } from 'vue'
import { systemApi, terminalsApi, type CreateTerminalInput } from '../api'
import type { Shell, Terminal } from '../types'

export const useTerminalsStore = defineStore('terminals', () => {
  const items = ref<Terminal[]>([])
  const shells = ref<Shell[]>([])
  const loading = ref(false)

  async function load() {
    loading.value = true
    try {
      items.value = await terminalsApi.list()
    } finally {
      loading.value = false
    }
  }

  async function loadShells() {
    shells.value = await systemApi.shells()
  }

  function upsert(term: Terminal) {
    const i = items.value.findIndex((t) => t.id === term.id)
    if (i >= 0) items.value[i] = term
    else items.value.unshift(term)
  }

  function removeLocal(id: string) {
    items.value = items.value.filter((t) => t.id !== id)
  }

  async function create(input: CreateTerminalInput) {
    const t = await terminalsApi.create(input)
    upsert(t)
    return t
  }

  async function restart(id: string) {
    const t = await terminalsApi.restart(id)
    upsert(t)
    return t
  }

  async function remove(id: string) {
    await terminalsApi.remove(id)
    removeLocal(id)
  }

  async function rename(id: string, name: string) {
    const t = await terminalsApi.update(id, { name })
    upsert(t)
    return t
  }

  return { items, shells, loading, load, loadShells, upsert, removeLocal, create, restart, remove, rename }
})
