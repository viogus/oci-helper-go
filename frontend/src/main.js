import { createApp } from 'vue'
import { createPinia } from 'pinia'
import 'element-plus/theme-chalk/base.css'
import './styles/theme.css'
import { ElLoading } from 'element-plus'
import App from './App.vue'
import router from './router'
import i18n from './locales/index.js'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(i18n)

// v-loading directive still needs global registration (used in 10+ views)
app.directive('loading', ElLoading.directive)

app.mount('#app')
