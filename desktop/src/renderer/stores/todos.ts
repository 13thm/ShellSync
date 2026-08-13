import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { todosApi, type CreateTodoInput, type UpdateTodoInput } from '../api'
import type { Todo } from '../types'

export const useTodosStore = defineStore('todos', () => {
  const items = ref<Todo[]>([])
  const loading = ref(false)

  const pending = computed(() =>
    items.value.filter((t) => t.status === 'pending').sort((a, b) => a.sortOrder - b.sortOrder),
  )
  const done = computed(() => items.value.filter((t) => t.status === 'done'))

  async function load() {
    loading.value = true
    try {
      items.value = await todosApi.list()
    } finally {
      loading.value = false
    }
  }

  function upsert(todo: Todo) {
    const i = items.value.findIndex((t) => t.id === todo.id)
    if (i >= 0) items.value[i] = todo
    else items.value.push(todo)
  }

  function removeLocal(id: string) {
    items.value = items.value.filter((t) => t.id !== id)
  }

  async function create(input: CreateTodoInput) {
    const t = await todosApi.create(input)
    upsert(t)
    return t
  }

  async function update(id: string, input: UpdateTodoInput) {
    const t = await todosApi.update(id, input)
    upsert(t)
    return t
  }

  async function toggle(id: string, done: boolean) {
    return update(id, { status: done ? 'done' : 'pending' })
  }

  async function remove(id: string) {
    await todosApi.remove(id)
    removeLocal(id)
  }

  return { items, loading, pending, done, load, upsert, removeLocal, create, update, toggle, remove }
})
