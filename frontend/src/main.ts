import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { api } from '@/services/api'
import { initAppI18n } from '@/modules/i18n/appI18n'
import { installClientErrorLogging } from '@/modules/logging/clientErrors'
import './style.css'

initAppI18n()
installClientErrorLogging((kind, message, detail) => api.recordClientError(kind, message, detail))

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')
