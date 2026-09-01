import { createApp } from 'vue'

import App from './App.vue'
import router from './router/index.js'
import './styles/main.css'

const savedTheme = globalThis.localStorage?.getItem('ai_gateway_theme')
if (savedTheme === 'dark' || savedTheme === 'light') {
  document.documentElement.dataset.theme = savedTheme
}

createApp(App).use(router).mount('#app')
