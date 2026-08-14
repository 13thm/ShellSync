import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { tasksApi, type CreateTaskInput, type UpdateTaskInput } from '../api'
import type { Task } from '../types'

export const useTasksStore = defineStore('tasks', () => {
  const items = ref<Task[]>([])
  const loading = ref(false)

  const active = computed(() => items.value.filter((t) => !t.archived))
  const archived = computed(() => items.value.filter((t) => t.archived))

  async function load() {
    loading.value = true
    try {
      items.value = await tasksApi.list()
    } finally {
      loading.value = false
    }
  }

  function upsert(task: Task) {
    if (!task || !task.id) return
    const i = items.value.findIndex((t) => t.id === task.id)
    if (i >= 0) items.value[i] = task
    else items.value.push(task)
  }

  function removeLocal(id: string) {
    items.value = items.value.filter((t) => t.id !== id)
  }

  async function create(input: CreateTaskInput) {
    const t = await tasksApi.create(input)
    upsert(t)
    return t
  }

  async function update(id: string, input: UpdateTaskInput) {
    const t = await tasksApi.update(id, input)
    upsert(t)
    return t
  }

  async function remove(id: string) {
    await tasksApi.remove(id)
    removeLocal(id)
  }

  return { items, loading, active, archived, load, upsert, removeLocal, create, update, remove }
})
