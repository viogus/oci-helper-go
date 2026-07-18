import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import './styles/theme.css'
import zhCn from 'element-plus/dist/locale/zh-cn.mjs'
import en from 'element-plus/dist/locale/en.mjs'
import App from './App.vue'
import router from './router'
import i18n from './locales/index.js'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(i18n)

// Pick Element Plus locale based on saved preference
const savedLocale = localStorage.getItem('locale') || 'zh-CN'
app.use(ElementPlus, { locale: savedLocale === 'en' ? en : zhCn })

app.mount('#app')
