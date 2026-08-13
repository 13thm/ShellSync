import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { router } from './router'

import './styles/tokens.css' // ① must be first
import './styles/base.css' // ② global base

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')
