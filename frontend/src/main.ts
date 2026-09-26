import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import App from './App.vue'
import router from './router'
import { useAuthStore } from './stores/authStore'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(ElementPlus, { locale: zhCn })

// Restore the profile on reload so role-based guards/menu work right away.
const auth = useAuthStore()
if (auth.token) {
  auth.fetchProfile().catch(() => auth.logout())
}

app.mount('#app')
