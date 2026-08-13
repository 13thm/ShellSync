import { createRouter, createWebHashHistory } from 'vue-router'

export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/tasks' },
    { path: '/tasks', name: 'tasks', component: () => import('../views/TaskList.vue') },
    { path: '/tasks/:id', name: 'task-detail', component: () => import('../views/TaskDetail.vue'), props: true },
    { path: '/terminals', name: 'terminals', component: () => import('../views/TerminalView.vue') },
    { path: '/todos', name: 'todos', component: () => import('../views/TodoView.vue') },
    { path: '/settings', name: 'settings', component: () => import('../views/SettingsView.vue') },
  ],
})
