# oci-helper 1:1 Rewrite — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 1:1 rewrite of Yohann0617/oci-helper features and frontend — Vue.js 3 SPA + extended Go backend, 17 views, ~25 new API endpoints, 7 phases.

**Architecture:** Vue 3 + Vite builds to `frontend/dist/` → copied to `internal/handler/dist/` → `//go:embed` embeds into Go binary. Backend stays `net/http` standard library, no framework. New handler files split by domain: `handler_security.go`, `handler_traffic.go`, `handler_tasks.go`, `handler_instance.go`. Worker extended for batch-create background jobs. OCI client gets network security, limits, traffic, and load balancer methods.

**Tech Stack:** Vue 3 (Composition API, `<script setup>`), Vue Router 4 (hash mode), Pinia, Element Plus, ECharts + vue-echarts, axios, Vite 5. Go 1.26, `net/http`, modernc.org/sqlite, OCI Go SDK v65.

---

## File Structure

```
frontend/                              # NEW — created by `npm create vue@latest`
├── index.html
├── package.json
├── vite.config.js                     # proxy /api → localhost:8818 in dev
└── src/
    ├── main.js                        # createApp + router + pinia + ElementPlus
    ├── App.vue                        # <router-view> only (layout handled per-route)
    ├── router/index.js                # 20 routes, beforeEach guard
    ├── api/
    │   ├── index.js                   # axios instance, interceptors, base helpers
    │   ├── auth.js                    # login, logout, mfaSetup, mfaVerify, mfaDisable
    │   ├── tenants.js                 # CRUD + sync
    │   ├── instances.js               # list, actions, create, batch, mutations
    │   ├── securityRules.js           # page, addIngress, addEgress, remove, release
    │   ├── traffic.js                 # data, conditions, fetchInstances
    │   ├── tasks.js                   # mem-tasks, create-tasks CRUD
    │   ├── cloudflare.js              # DNS CRUD
    │   └── settings.js               # config get/set
    ├── stores/
    │   ├── auth.js                    # user session, login/logout actions
    │   ├── tenants.js                 # cached tenant list
    │   └── app.js                     # sidebar collapse, dark mode, global state
    ├── views/
    │   ├── Login.vue                  # username/pw + MFA + Google OAuth
    │   ├── Home.vue                   # Phase 7 (stub: stats cards only)
    │   ├── Tenants.vue                # Phase 2
    │   ├── Instances.vue              # Phase 2
    │   ├── InstanceCreate.vue         # Phase 2
    │   ├── InstanceBatchCreate.vue    # Phase 4
    │   ├── CreateTasks.vue            # Phase 4
    │   ├── SecurityRules.vue          # Phase 3
    │   ├── Traffic.vue                # Phase 3
    │   ├── Limits.vue                 # Phase 3
    │   ├── PublicIPs.vue              # Phase 5
    │   ├── BootVolumes.vue            # Phase 5
    │   ├── Cloudflare.vue             # Phase 5
    │   ├── InMemoryTasks.vue          # Phase 5
    │   ├── AiChat.vue                 # Phase 6
    │   ├── Backup.vue                 # Phase 6
    │   ├── Logs.vue                   # Phase 6
    │   ├── Settings.vue               # Phase 6
    │   ├── VncConsole.vue             # Phase 7
    │   └── IpInfo.vue                 # Phase 7
    └── components/
        ├── AppLayout.vue              # sidebar + topbar shell
        ├── Pagination.vue             # el-pagination wrapper
        └── SearchFilter.vue           # el-input + debounce
```

---

## Phase 1: Foundation

### Task 1.1: Scaffold Vue project

**Files:**
- Create: `frontend/package.json`
- Create: `frontend/vite.config.js`
- Create: `frontend/index.html`
- Create: `frontend/src/main.js`
- Create: `frontend/src/App.vue`

- [ ] **Step 1: Create `frontend/package.json`**

```json
{
  "name": "oci-helper-frontend",
  "version": "1.0.0",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "vue": "^3.5.0",
    "vue-router": "^4.4.0",
    "pinia": "^2.2.0",
    "element-plus": "^2.9.0",
    "@element-plus/icons-vue": "^2.3.0",
    "axios": "^1.7.0",
    "echarts": "^5.6.0",
    "vue-echarts": "^7.0.0"
  },
  "devDependencies": {
    "@vitejs/plugin-vue": "^5.2.0",
    "vite": "^5.4.0"
  }
}
```

- [ ] **Step 2: Install dependencies**

```bash
cd frontend && npm install
```

- [ ] **Step 3: Create `frontend/vite.config.js`**

```js
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8818'
    }
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true
  }
})
```

- [ ] **Step 4: Create `frontend/index.html`**

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>oci-helper</title>
</head>
<body>
  <div id="app"></div>
  <script type="module" src="/src/main.js"></script>
</body>
</html>
```

- [ ] **Step 5: Create `frontend/src/main.js`**

```js
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import zhCn from 'element-plus/dist/locale/zh-cn.mjs'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import App from './App.vue'
import router from './router'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(ElementPlus, { locale: zhCn })
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}
app.mount('#app')
```

- [ ] **Step 6: Create `frontend/src/App.vue`**

```vue
<template>
  <router-view />
</template>
```

- [ ] **Step 7: Verify dev server starts**

```bash
cd frontend && npm run dev
```

Expected: Vite dev server on localhost:5173. Blank page with no errors in console.

- [ ] **Step 8: Commit**

```bash
git add frontend/package.json frontend/package-lock.json frontend/vite.config.js \
        frontend/index.html frontend/src/main.js frontend/src/App.vue
git commit -m "feat: scaffold Vue 3 + Vite + Element Plus project"
```

### Task 1.2: API client layer

**Files:**
- Create: `frontend/src/api/index.js`
- Create: `frontend/src/api/auth.js`

- [ ] **Step 1: Create `frontend/src/api/index.js`**

```js
import axios from 'axios'

const api = axios.create({
  baseURL: '/api',
  timeout: 30000,
  headers: { 'Content-Type': 'application/json' }
})

// response interceptor: unwrap data, handle 401
api.interceptors.response.use(
  res => res,
  err => {
    if (err.response?.status === 401) {
      const router = (window.__router)
      if (router && router.currentRoute?.value?.path !== '/login') {
        router.push('/login')
      }
    }
    return Promise.reject(err)
  }
)

// helper: GET JSON
export async function get(path, params = {}) {
  const res = await api.get(path, { params })
  return res.data
}

// helper: POST JSON
export async function post(path, data = {}) {
  const res = await api.post(path, data)
  return res.data
}

// helper: POST FormData
export async function upload(path, formData) {
  const res = await api.post(path, formData, {
    headers: { 'Content-Type': 'multipart/form-data' }
  })
  return res.data
}

// helper: DELETE
export async function del(path) {
  const res = await api.delete(path)
  return res.data
}

export default api
```

- [ ] **Step 2: Create `frontend/src/api/auth.js`**

```js
import { get, post } from './index.js'

export function login(username, password, totp) {
  return post('/login', {}, {
    headers: {
      'Authorization': 'Basic ' + btoa(username + ':' + password),
      ...(totp ? { 'X-TOTP': totp } : {})
    }
  })
}

export function logout() {
  return post('/logout')
}

export function getConfig() {
  return get('/config')
}

export function saveConfig(key, value) {
  return post('/config', { key, value })
}

export function mfaSetup() {
  return get('/mfa/setup')
}

export function mfaVerify(code) {
  return post('/mfa/verify', { code })
}

export function mfaDisable(code) {
  return post('/mfa/disable', { code })
}
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/api/
git commit -m "feat: add API client layer with axios interceptors"
```

### Task 1.3: Router + Auth Store

**Files:**
- Create: `frontend/src/router/index.js`
- Create: `frontend/src/stores/auth.js`

- [ ] **Step 1: Create `frontend/src/stores/auth.js`**

```js
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import * as authApi from '../api/auth.js'

export const useAuthStore = defineStore('auth', () => {
  const user = ref(null)
  const isAuthenticated = computed(() => !!user.value)

  async function checkSession() {
    try {
      const cfg = await authApi.getConfig()
      user.value = { name: cfg.username || 'admin' }
      return true
    } catch {
      user.value = null
      return false
    }
  }

  async function doLogin(username, password, totp) {
    await authApi.login(username, password, totp)
    await checkSession()
  }

  async function doLogout() {
    try { await authApi.logout() } catch {}
    user.value = null
  }

  return { user, isAuthenticated, checkSession, doLogin, doLogout }
})
```

- [ ] **Step 2: Create `frontend/src/router/index.js`**

```js
import { createRouter, createWebHashHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth.js'

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('../views/Login.vue'),
    meta: { guest: true }
  },
  {
    path: '/',
    redirect: '/home'
  },
  {
    path: '/home',
    name: 'Home',
    component: () => import('../views/Home.vue'),
    meta: { title: 'Home', icon: 'HomeFilled' }
  },
  {
    path: '/tenants',
    name: 'Tenants',
    component: () => import('../views/Tenants.vue'),
    meta: { title: 'Tenants', icon: 'User' }
  },
  {
    path: '/instances',
    name: 'Instances',
    component: () => import('../views/Instances.vue'),
    meta: { title: 'Instances', icon: 'Monitor' }
  },
  {
    path: '/instances/create',
    name: 'InstanceCreate',
    component: () => import('../views/InstanceCreate.vue'),
    meta: { title: 'Create Instance', icon: 'Plus' }
  },
  {
    path: '/instances/batch-create',
    name: 'InstanceBatchCreate',
    component: () => import('../views/InstanceBatchCreate.vue'),
    meta: { title: 'Batch Create', icon: 'Grid' }
  },
  {
    path: '/create-tasks',
    name: 'CreateTasks',
    component: () => import('../views/CreateTasks.vue'),
    meta: { title: 'Create Tasks', icon: 'List' }
  },
  {
    path: '/security-rules',
    name: 'SecurityRules',
    component: () => import('../views/SecurityRules.vue'),
    meta: { title: 'Security Rules', icon: 'Lock' }
  },
  {
    path: '/traffic',
    name: 'Traffic',
    component: () => import('../views/Traffic.vue'),
    meta: { title: 'Traffic', icon: 'TrendCharts' }
  },
  {
    path: '/limits',
    name: 'Limits',
    component: () => import('../views/Limits.vue'),
    meta: { title: 'Limits', icon: 'DataAnalysis' }
  },
  {
    path: '/ips',
    name: 'PublicIPs',
    component: () => import('../views/PublicIPs.vue'),
    meta: { title: 'Public IPs', icon: 'Connection' }
  },
  {
    path: '/volumes',
    name: 'BootVolumes',
    component: () => import('../views/BootVolumes.vue'),
    meta: { title: 'Boot Volumes', icon: 'FolderOpened' }
  },
  {
    path: '/cloudflare',
    name: 'Cloudflare',
    component: () => import('../views/Cloudflare.vue'),
    meta: { title: 'Cloudflare', icon: 'Cloudy' }
  },
  {
    path: '/ai-chat',
    name: 'AiChat',
    component: () => import('../views/AiChat.vue'),
    meta: { title: 'AI Chat', icon: 'ChatDotRound' }
  },
  {
    path: '/backup',
    name: 'Backup',
    component: () => import('../views/Backup.vue'),
    meta: { title: 'Backup', icon: 'Upload' }
  },
  {
    path: '/logs',
    name: 'Logs',
    component: () => import('../views/Logs.vue'),
    meta: { title: 'Logs', icon: 'Document' }
  },
  {
    path: '/mem-tasks',
    name: 'InMemoryTasks',
    component: () => import('../views/InMemoryTasks.vue'),
    meta: { title: 'Memory Tasks', icon: 'Timer' }
  },
  {
    path: '/settings',
    name: 'Settings',
    component: () => import('../views/Settings.vue'),
    meta: { title: 'Settings', icon: 'Setting' }
  },
  {
    path: '/vnc',
    name: 'VncConsole',
    component: () => import('../views/VncConsole.vue'),
    meta: { title: 'VNC Console', icon: 'Monitor' }
  },
  {
    path: '/ip-info',
    name: 'IpInfo',
    component: () => import('../views/IpInfo.vue'),
    meta: { title: 'IP Info', icon: 'InfoFilled', guest: true }
  }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes
})

// expose router for axios interceptor
window.__router = router

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (to.meta.guest) return true
  if (!auth.isAuthenticated) {
    const ok = await auth.checkSession()
    if (!ok) return '/login'
  }
  return true
})

export default router
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/router/ frontend/src/stores/
git commit -m "feat: add Vue Router with 20 routes + Pinia auth store"
```

### Task 1.4: AppLayout component

**Files:**
- Create: `frontend/src/components/AppLayout.vue`

- [ ] **Step 1: Create `frontend/src/components/AppLayout.vue`**

```vue
<template>
  <el-container class="app-layout">
    <el-aside :width="sidebarCollapsed ? '64px' : '220px'" class="app-sidebar">
      <div class="logo" @click="$router.push('/home')">
        <span v-if="!sidebarCollapsed">oci-helper</span>
        <span v-else>O</span>
      </div>
      <el-menu
        :default-active="route.path"
        :collapse="sidebarCollapsed"
        :router="true"
        background-color="#1a1f2e"
        text-color="#bfcbd9"
        active-text-color="#409eff"
      >
        <el-menu-item index="/home">
          <el-icon><HomeFilled /></el-icon>
          <span>Home</span>
        </el-menu-item>
        <el-sub-menu index="resources">
          <template #title>
            <el-icon><Monitor /></el-icon>
            <span>Resources</span>
          </template>
          <el-menu-item index="/instances">Instances</el-menu-item>
          <el-menu-item index="/instances/create">Create Instance</el-menu-item>
          <el-menu-item index="/ips">Public IPs</el-menu-item>
          <el-menu-item index="/volumes">Boot Volumes</el-menu-item>
        </el-sub-menu>
        <el-sub-menu index="network">
          <template #title>
            <el-icon><Connection /></el-icon>
            <span>Network</span>
          </template>
          <el-menu-item index="/security-rules">Security Rules</el-menu-item>
          <el-menu-item index="/traffic">Traffic</el-menu-item>
          <el-menu-item index="/cloudflare">Cloudflare</el-menu-item>
        </el-sub-menu>
        <el-sub-menu index="tasks-sub">
          <template #title>
            <el-icon><Timer /></el-icon>
            <span>Tasks</span>
          </template>
          <el-menu-item index="/create-tasks">Create Tasks</el-menu-item>
          <el-menu-item index="/instances/batch-create">Batch Create</el-menu-item>
          <el-menu-item index="/mem-tasks">Memory Tasks</el-menu-item>
        </el-sub-menu>
        <el-sub-menu index="tools">
          <template #title>
            <el-icon><Tools /></el-icon>
            <span>Tools</span>
          </template>
          <el-menu-item index="/limits">Limits</el-menu-item>
          <el-menu-item index="/ai-chat">AI Chat</el-menu-item>
          <el-menu-item index="/logs">Logs</el-menu-item>
          <el-menu-item index="/vnc">VNC Console</el-menu-item>
        </el-sub-menu>
        <el-menu-item index="/settings">
          <el-icon><Setting /></el-icon>
          <span>Settings</span>
        </el-menu-item>
        <el-menu-item index="/backup">
          <el-icon><Upload /></el-icon>
          <span>Backup</span>
        </el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header class="app-header">
        <div class="header-left">
          <el-button :icon="Fold" text @click="sidebarCollapsed = !sidebarCollapsed" />
          <el-breadcrumb separator="/">
            <el-breadcrumb-item :to="{ path: '/home' }">Home</el-breadcrumb-item>
            <el-breadcrumb-item v-if="route.meta.title">{{ route.meta.title }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        <div class="header-right">
          <el-switch v-model="isDark" inline-prompt :active-icon="Moon" :inactive-icon="Sunny" @change="toggleDark" />
          <span style="margin:0 12px;color:#bfcbd9">{{ auth.user?.name }}</span>
          <el-button text @click="handleLogout">Logout</el-button>
        </div>
      </el-header>
      <el-main>
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth.js'
import { Fold, Moon, Sunny } from '@element-plus/icons-vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const sidebarCollapsed = ref(false)
const isDark = ref(localStorage.getItem('theme') === 'dark')

function toggleDark(v) {
  localStorage.setItem('theme', v ? 'dark' : 'light')
  document.documentElement.classList.toggle('dark', v)
}

async function handleLogout() {
  await auth.doLogout()
  router.push('/login')
}
</script>

<style>
html, body, #app { margin: 0; padding: 0; height: 100%; }
.app-layout { height: 100vh; }
.app-sidebar {
  background-color: #1a1f2e;
  overflow-y: auto;
  transition: width 0.3s;
}
.app-sidebar .logo {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 20px;
  font-weight: bold;
  cursor: pointer;
  border-bottom: 1px solid rgba(255,255,255,0.1);
}
.app-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
  height: 60px;
  padding: 0 20px;
}
.dark .app-header { background: #1a1f2e; border-color: rgba(255,255,255,0.1); }
.header-left, .header-right { display: flex; align-items: center; gap: 8px; }
.el-main { background: #f5f7fa; }
.dark .el-main { background: #141414; }
.el-menu { border-right: none !important; }
</style>
```

- [ ] **Step 2: Update router to use AppLayout for authenticated routes**

Edit `frontend/src/router/index.js` — wrap authenticated routes as children of a layout route:

```js
const routes = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('../views/Login.vue'),
    meta: { guest: true }
  },
  {
    path: '/ip-info',
    name: 'IpInfo',
    component: () => import('../views/IpInfo.vue'),
    meta: { title: 'IP Info', guest: true }
  },
  {
    path: '/',
    component: () => import('../components/AppLayout.vue'),
    children: [
      { path: '', redirect: '/home' },
      { path: 'home', name: 'Home', component: () => import('../views/Home.vue'), meta: { title: 'Home', icon: 'HomeFilled' } },
      { path: 'tenants', name: 'Tenants', component: () => import('../views/Tenants.vue'), meta: { title: 'Tenants', icon: 'User' } },
      { path: 'instances', name: 'Instances', component: () => import('../views/Instances.vue'), meta: { title: 'Instances', icon: 'Monitor' } },
      { path: 'instances/create', name: 'InstanceCreate', component: () => import('../views/InstanceCreate.vue'), meta: { title: 'Create Instance', icon: 'Plus' } },
      { path: 'instances/batch-create', name: 'InstanceBatchCreate', component: () => import('../views/InstanceBatchCreate.vue'), meta: { title: 'Batch Create', icon: 'Grid' } },
      { path: 'create-tasks', name: 'CreateTasks', component: () => import('../views/CreateTasks.vue'), meta: { title: 'Create Tasks', icon: 'List' } },
      { path: 'security-rules', name: 'SecurityRules', component: () => import('../views/SecurityRules.vue'), meta: { title: 'Security Rules', icon: 'Lock' } },
      { path: 'traffic', name: 'Traffic', component: () => import('../views/Traffic.vue'), meta: { title: 'Traffic', icon: 'TrendCharts' } },
      { path: 'limits', name: 'Limits', component: () => import('../views/Limits.vue'), meta: { title: 'Limits', icon: 'DataAnalysis' } },
      { path: 'ips', name: 'PublicIPs', component: () => import('../views/PublicIPs.vue'), meta: { title: 'Public IPs', icon: 'Connection' } },
      { path: 'volumes', name: 'BootVolumes', component: () => import('../views/BootVolumes.vue'), meta: { title: 'Boot Volumes', icon: 'FolderOpened' } },
      { path: 'cloudflare', name: 'Cloudflare', component: () => import('../views/Cloudflare.vue'), meta: { title: 'Cloudflare', icon: 'Cloudy' } },
      { path: 'ai-chat', name: 'AiChat', component: () => import('../views/AiChat.vue'), meta: { title: 'AI Chat', icon: 'ChatDotRound' } },
      { path: 'backup', name: 'Backup', component: () => import('../views/Backup.vue'), meta: { title: 'Backup', icon: 'Upload' } },
      { path: 'logs', name: 'Logs', component: () => import('../views/Logs.vue'), meta: { title: 'Logs', icon: 'Document' } },
      { path: 'mem-tasks', name: 'InMemoryTasks', component: () => import('../views/InMemoryTasks.vue'), meta: { title: 'Memory Tasks', icon: 'Timer' } },
      { path: 'settings', name: 'Settings', component: () => import('../views/Settings.vue'), meta: { title: 'Settings', icon: 'Setting' } },
      { path: 'vnc', name: 'VncConsole', component: () => import('../views/VncConsole.vue'), meta: { title: 'VNC Console', icon: 'Monitor' } },
    ]
  }
]
```

Also redirect `/` to `/home`:

Remove old flat route `{ path: '/', redirect: '/home' }` if present. Already handled above.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/AppLayout.vue frontend/src/router/index.js
git commit -m "feat: add AppLayout with sidebar navigation and dark mode"
```

### Task 1.5: Login page

**Files:**
- Create: `frontend/src/views/Login.vue`

- [ ] **Step 1: Create `frontend/src/views/Login.vue`**

```vue
<template>
  <div class="login-page">
    <el-card class="login-card" shadow="always">
      <h2 style="text-align:center;margin-bottom:24px">oci-helper</h2>
      <el-form @submit.prevent="handleLogin" label-position="top">
        <el-form-item label="Username">
          <el-input v-model="username" placeholder="admin" />
        </el-form-item>
        <el-form-item label="Password">
          <el-input v-model="password" type="password" show-password placeholder="Password" />
        </el-form-item>
        <el-form-item v-if="needMfa" label="MFA Code">
          <el-input v-model="totp" placeholder="6-digit code" maxlength="6" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" native-type="submit" :loading="loading" style="width:100%">
            Login
          </el-button>
        </el-form-item>
        <div v-if="error" style="color:var(--el-color-danger);text-align:center;margin-bottom:12px">
          {{ error }}
        </div>
        <div style="text-align:center">
          <el-button link type="primary" @click="googleLogin">Google Login</el-button>
        </div>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '../stores/auth.js'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()

const username = ref('admin')
const password = ref('')
const totp = ref('')
const needMfa = ref(false)
const loading = ref(false)
const error = ref('')

async function handleLogin() {
  loading.value = true
  error.value = ''
  try {
    await auth.doLogin(username.value, password.value, totp.value || undefined)
    router.push(route.query.redirect || '/')
  } catch (e) {
    const status = e.response?.status
    if (status === 401) {
      if (!needMfa.value) {
        needMfa.value = true
        error.value = 'MFA code required'
      } else {
        error.value = 'Invalid credentials or MFA code'
      }
    } else {
      error.value = e.response?.data?.error || 'Login failed'
    }
  }
  loading.value = false
}

function googleLogin() {
  window.location.href = '/api/oauth/google/login'
}
</script>

<style scoped>
.login-page {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100vh;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}
.dark .login-page { background: linear-gradient(135deg, #1a1f2e 0%, #2d1b69 100%); }
.login-card {
  width: 400px;
  padding: 20px;
}
</style>
```

- [ ] **Step 2: Handle Google OAuth callback**

The OAuth callback redirects to `/#/home`. No separate `GoogleCallback.vue` needed — the router guard in `beforeEach` will check session and log in automatically since the session cookie is already set.

Add to `router/index.js` a catch for the `/oauth/callback` path:

```js
{
  path: '/oauth/callback',
  redirect: '/home'
}
```

- [ ] **Step 3: Create placeholder Home.vue**

Create `frontend/src/views/Home.vue`:

```vue
<template>
  <div>
    <h3>Dashboard</h3>
    <el-row :gutter="16">
      <el-col :span="8" v-for="stat in stats" :key="stat.label">
        <el-card shadow="hover">
          <div style="text-align:center">
            <div style="font-size:32px;font-weight:bold;color:#409eff">{{ stat.value }}</div>
            <div style="color:#909399;margin-top:8px">{{ stat.label }}</div>
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { reactive } from 'vue'

const stats = reactive([
  { label: 'Tenants', value: 0 },
  { label: 'Instances', value: 0 },
  { label: 'Running Tasks', value: 0 },
])
</script>
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/Login.vue frontend/src/views/Home.vue frontend/src/router/index.js
git commit -m "feat: add login page with MFA support and OAuth"
```

### Task 1.6: Backend route stubs for all new endpoints

**Files:**
- Modify: `internal/handler/handler.go`

- [ ] **Step 1: Register new routes in `routes()`**

After existing route `s.mux.HandleFunc("/api/keys/", s.withAuth(s.handleKeyByID))`, add:

```go
	// instance mutations
	s.mux.HandleFunc("/api/instances/change-shape", s.withAuth(s.handleChangeShape))
	s.mux.HandleFunc("/api/instances/change-boot-volume", s.withAuth(s.handleChangeBootVolume))
	s.mux.HandleFunc("/api/instances/attach-ipv6", s.withAuth(s.handleAttachIPv6))
	s.mux.HandleFunc("/api/instances/update-name", s.withAuth(s.handleUpdateInstanceName))
	s.mux.HandleFunc("/api/instances/change-ip", s.withAuth(s.handleChangeIP))
	s.mux.HandleFunc("/api/instances/check-alive", s.withAuth(s.handleCheckAlive))
	s.mux.HandleFunc("/api/instances/one-click-500m", s.withAuth(s.handleOneClick500M))
	s.mux.HandleFunc("/api/instances/one-click-close-500m", s.withAuth(s.handleOneClickClose500M))
	s.mux.HandleFunc("/api/instances/auto-rescue", s.withAuth(s.handleAutoRescue))
	s.mux.HandleFunc("/api/instances/update-shape", s.withAuth(s.handleUpdateShape))

	// security rules
	s.mux.HandleFunc("/api/security-rules", s.withAuth(s.handleSecurityRules))

	// traffic & monitoring
	s.mux.HandleFunc("/api/traffic", s.withAuth(s.handleTraffic))
	s.mux.HandleFunc("/api/limits", s.withAuth(s.handleLimits))
	s.mux.HandleFunc("/api/logs", s.withAuth(s.handleLogs))

	// batch create tasks
	s.mux.HandleFunc("/api/instances/batch-create", s.withAuth(s.handleBatchCreate))
	s.mux.HandleFunc("/api/create-tasks", s.withAuth(s.handleCreateTasks))

	// in-memory tasks
	s.mux.HandleFunc("/api/mem-tasks/change-ip", s.withAuth(s.handleMemTasksChangeIP))
	s.mux.HandleFunc("/api/mem-tasks/update-cfg", s.withAuth(s.handleMemTasksUpdateCfg))

	// ip-info (no auth)
	s.mux.HandleFunc("/api/ip-info", s.handleIPInfo)
```

Note: `/api/instances/batch-start` already exists. The new batch-create is different — it saves a task in DB and lets the worker run it. The path `/api/instances/batch-create` is distinct.

Note: The wildcard routes (`/api/instances/`) already exist for instance actions. The new mutation routes are exact paths and must be registered BEFORE the wildcard routes. Move the existing wildcard routes AFTER all new exact paths.

IMPORTANT: Reorder route registration so wildcard paths come LAST:

```go
func (s *Server) routes() {
    // API — exact paths first
    s.mux.HandleFunc("/api/login", s.handleLogin)
    // ... all exact paths ...
    s.mux.HandleFunc("/api/instances/batch-start", s.withAuth(s.handleBatchStart))
    s.mux.HandleFunc("/api/instances/batch-create", s.withAuth(s.handleBatchCreate))
    // ... more exact paths ...
    s.mux.HandleFunc("/api/instances/change-shape", s.withAuth(s.handleChangeShape))
    // ... etc ...

    // Wildcard paths LAST
    s.mux.HandleFunc("/api/tenants/", s.withAuth(s.handleTenantByID))
    s.mux.HandleFunc("/api/instances/", s.withAuth(s.handleInstanceAction))
    s.mux.HandleFunc("/api/public-ips/", s.withAuth(s.handlePublicIPByID))
    s.mux.HandleFunc("/api/boot-volumes/", s.withAuth(s.handleBootVolumeByID))
    s.mux.HandleFunc("/api/keys/", s.withAuth(s.handleKeyByID))
    s.mux.HandleFunc("/api/sync/", s.withAuth(s.handleSync))
    // ... cloudflare, shell wildcards ...
}
```

- [ ] **Step 2: Add stub handler implementations**

Add at the end of `handler.go`:

```go
// --- Phase 1 stubs (implemented in later phases) ---

func (s *Server) handleChangeShape(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleChangeBootVolume(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleAttachIPv6(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleUpdateInstanceName(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleChangeIP(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleCheckAlive(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleOneClick500M(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleOneClickClose500M(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleAutoRescue(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleUpdateShape(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleSecurityRules(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleTraffic(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleLimits(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleBatchCreate(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleCreateTasks(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleMemTasksChangeIP(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleMemTasksUpdateCfg(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleIPInfo(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"ip": r.RemoteAddr})
}
```

- [ ] **Step 3: Verify Go build still compiles**

```bash
CGO_ENABLED=0 go build -o /dev/null ./cmd/server
```

Expected: BUILD OK

- [ ] **Step 4: Commit**

```bash
git add internal/handler/handler.go
git commit -m "feat: add route stubs for all new API endpoints (Phase 1)"
```

### Task 1.7: Build integration

**Files:**
- Modify: `cmd/server/main.go` (verify `//go:embed` path, no changes expected)
- Create: `Makefile`

- [ ] **Step 1: Create `Makefile`**

```makefile
.PHONY: dev build clean

dev:
	cd frontend && npm run dev

build:
	cd frontend && npm run build
	rm -rf internal/handler/dist
	cp -r frontend/dist internal/handler/dist
	CGO_ENABLED=0 go build -ldflags="-s -w" -o oci-helper ./cmd/server

clean:
	rm -rf oci-helper frontend/dist internal/handler/dist frontend/node_modules
```

- [ ] **Step 2: Verify full build**

```bash
make build
./oci-helper health
```

Expected: binary builds, healthcheck exits 0 if server running on 8818.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "feat: add Makefile with frontend+backend build pipeline"
```

### Task 1.8: Update Dockerfile

**Files:**
- Read: `Dockerfile`
- Modify: `Dockerfile`

- [ ] **Step 1: Read existing Dockerfile**

```bash
cat Dockerfile
```

- [ ] **Step 2: Add Node build stage**

The Dockerfile must be updated to:
1. Stage 1: `node:22-alpine` — `npm ci && npm run build` in `frontend/`
2. Stage 2: `golang:1.26-alpine` — copy frontend dist, build Go binary
3. Stage 3: `FROM scratch` — unchanged

```dockerfile
FROM node:22-alpine AS frontend
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/dist internal/handler/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o oci-helper ./cmd/server

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /app/oci-helper /oci-helper
RUN mkdir -p /app/oci-helper/keys && chmod 777 /app/oci-helper /app/oci-helper/keys
USER nobody
EXPOSE 8818
CMD ["/oci-helper"]
```

- [ ] **Step 3: Commit**

```bash
git add Dockerfile
git commit -m "feat: add Node.js frontend build stage to Dockerfile"
```

---

## Phase 2: Tenant & Instance Core

### Task 2.1: Paginated DB queries

**Files:**
- Modify: `internal/db/queries.go`

- [ ] **Step 1: Add `ListInstancesPaginated`**

```go
func (s *Store) ListInstancesPaginated(tenantID int64, keyword string, page, size int) ([]Instance, int64, error) {
	kw := "%" + keyword + "%"
	var total int64
	s.db.QueryRow(`SELECT COUNT(*) FROM instances WHERE (tenant_id=? OR ?=0) AND (name LIKE ? OR ocid LIKE ? OR public_ip LIKE ?)`,
		tenantID, tenantID, kw, kw, kw).Scan(&total)

	offset := (page - 1) * size
	rows, err := s.db.Query(`SELECT id, tenant_id, name, ocid, shape, ocpu, memory_gb, boot_volume_gb,
		public_ip, private_ip, state, availability_domain, fault_domain, image_id, subnet_id,
		created_at, synced_at FROM instances
		WHERE (tenant_id=? OR ?=0) AND (name LIKE ? OR ocid LIKE ? OR public_ip LIKE ?)
		ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		tenantID, tenantID, kw, kw, kw, size, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []Instance
	for rows.Next() {
		var i Instance
		if err := rows.Scan(&i.ID, &i.TenantID, &i.Name, &i.OCID, &i.Shape, &i.OCPU, &i.MemoryGB, &i.BootVolumeGB,
			&i.PublicIP, &i.PrivateIP, &i.State, &i.AvailabilityDomain, &i.FaultDomain, &i.ImageID, &i.SubnetID,
			&i.CreatedAt, &i.SyncedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, i)
	}
	return list, total, rows.Err()
}
```

- [ ] **Step 2: Add `ListTenantsPaginated`**

```go
func (s *Store) ListTenantsPaginated(keyword string, page, size int) ([]Tenant, int64, error) {
	kw := "%" + keyword + "%"
	var total int64
	s.db.QueryRow(`SELECT COUNT(*) FROM tenants WHERE name LIKE ? OR region LIKE ?`, kw, kw).Scan(&total)

	offset := (page - 1) * size
	rows, err := s.db.Query(`SELECT id, name, user_ocid, tenancy_ocid, region, fingerprint, key_file,
		status, coalesce(home_region,''), coalesce(subscribed,''), created_at, updated_at FROM tenants
		WHERE name LIKE ? OR region LIKE ?
		ORDER BY id DESC LIMIT ? OFFSET ?`,
		kw, kw, size, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.UserOCID, &t.TenancyOCID, &t.Region, &t.Fingerprint, &t.KeyFile,
			&t.Status, &t.HomeRegion, &t.Subscribed, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, t)
	}
	return list, total, rows.Err()
}
```

- [ ] **Step 3: Add `ListTasksPaginated`**

```go
func (s *Store) ListTasksPaginated(keyword string, page, size int) ([]Task, int64, error) {
	kw := "%" + keyword + "%"
	var total int64
	s.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE type LIKE ? OR message LIKE ?`, kw, kw).Scan(&total)

	offset := (page - 1) * size
	rows, err := s.db.Query(`SELECT id, tenant_id, type, status, progress, message, payload,
		coalesce(result,''), created_at, started_at, finished_at FROM tasks
		WHERE type LIKE ? OR message LIKE ?
		ORDER BY id DESC LIMIT ? OFFSET ?`,
		kw, kw, size, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Type, &t.Status, &t.Progress, &t.Message,
			&t.Payload, &t.Result, &t.CreatedAt, &t.StartedAt, &t.FinishedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, t)
	}
	return list, total, rows.Err()
}
```

- [ ] **Step 4: Add `UpdateTaskPayload`**

```go
func (s *Store) UpdateTaskPayload(id int64, payload string) error {
	_, err := s.db.Exec(`UPDATE tasks SET payload=? WHERE id=?`, payload, id)
	return err
}
```

- [ ] **Step 5: Build and commit**

```bash
CGO_ENABLED=0 go build -o /dev/null ./cmd/server
git add internal/db/queries.go
git commit -m "feat: add paginated DB queries with keyword search"
```

### Task 2.2: Update existing handlers for pagination

**Files:**
- Modify: `internal/handler/handler.go`

- [ ] **Step 1: Update `handleInstances` GET to support pagination**

In `handleInstances`, change the GET case from `ListInstances(tenantID)` to `ListInstancesPaginated`:

```go
case http.MethodGet:
    tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
    keyword := r.URL.Query().Get("keyword")
    page, _ := strconv.Atoi(r.URL.Query().Get("page"))
    if page < 1 { page = 1 }
    size, _ := strconv.Atoi(r.URL.Query().Get("size"))
    if size < 1 { size = 20 }
    list, total, err := s.store.ListInstancesPaginated(tenantID, keyword, page, size)
    if err != nil {
        jsonErr(w, "list instances: "+err.Error())
        return
    }
    if list == nil { list = []db.Instance{} }
    jsonOK(w, map[string]interface{}{"data": list, "total": total, "page": page, "size": size})
```

- [ ] **Step 2: Update `handleTenants` GET similarly**

```go
case http.MethodGet:
    keyword := r.URL.Query().Get("keyword")
    page, _ := strconv.Atoi(r.URL.Query().Get("page"))
    if page < 1 { page = 1 }
    size, _ := strconv.Atoi(r.URL.Query().Get("size"))
    if size < 1 { size = 20 }
    list, total, err := s.store.ListTenantsPaginated(keyword, page, size)
    if err != nil {
        jsonErr(w, "list tenants: "+err.Error())
        return
    }
    if list == nil { list = []db.Tenant{} }
    jsonOK(w, map[string]interface{}{"data": list, "total": total, "page": page, "size": size})
```

- [ ] **Step 3: Build and commit**

```bash
CGO_ENABLED=0 go build -o /dev/null ./cmd/server
git add internal/handler/handler.go
git commit -m "feat: add pagination + keyword search to instance and tenant APIs"
```

### Task 2.3: Tenants page

**Files:**
- Create: `frontend/src/api/tenants.js`
- Create: `frontend/src/views/Tenants.vue`
- Modify: `frontend/src/stores/tenants.js`

- [ ] **Step 1: Create `frontend/src/api/tenants.js`**

```js
import { get, post, del } from './index.js'

export function listTenants(params = {}) {
  return get('/tenants', params)
}

export function createTenant(data) {
  return post('/tenants', data)
}

export function getTenant(id) {
  return get('/tenants/' + id)
}

export function deleteTenant(id) {
  return del('/tenants/' + id)
}

export function syncTenant(id) {
  return post('/sync/' + id)
}

export function listKeys() {
  return get('/keys')
}

export function uploadKeys(formData) {
  return post('/keys', formData, {
    headers: { 'Content-Type': 'multipart/form-data' }
  })
}
```

- [ ] **Step 2: Create `frontend/src/views/Tenants.vue`**

Full Vue component with `el-table`, `el-pagination`, search bar, add/edit modal, key upload, OCI config paste textarea. Port existing `addTenant()` and helper functions from the old `index.html`.

Key elements:
- Search bar: `el-input` with `@input` debounce
- Table: `el-table` with columns for ID, Name, Region, Status, Actions (Sync, Delete)
- Add modal: `el-dialog` with form fields (Name, OCI config paste textarea, Tenancy OCID, User OCID, Region, Fingerprint, Key file dropdown + Upload/Batch buttons + drag-drop zone)
- Pagination: `el-pagination` with `layout="total, prev, pager, next"`
- `onMounted` calls `loadTenants()` and `loadKeys()`

(Full component ~200 lines — port from existing `index.html` with Element Plus components)

- [ ] **Step 3: Commit**

```bash
git add frontend/src/api/tenants.js frontend/src/views/Tenants.vue
git commit -m "feat: add Tenants page with pagination, search, and key upload"
```

### Task 2.4: Instances page

**Files:**
- Create: `frontend/src/api/instances.js`
- Create: `frontend/src/views/Instances.vue`

- [ ] **Step 1: Create `frontend/src/api/instances.js`**

```js
import { get, post } from './index.js'

export function listInstances(params = {}) {
  return get('/instances', params)
}

export function instanceAction(instanceId, action) {
  return post('/instances/' + instanceId, { action })
}

export function batchStart(payload) {
  return post('/instances/batch-start', payload)
}

export function createInstance(data) {
  return post('/instances', data)
}

export function changeShape(data) {
  return post('/instances/change-shape', data)
}

export function changeBootVolume(data) {
  return post('/instances/change-boot-volume', data)
}

export function attachIPv6(data) {
  return post('/instances/attach-ipv6', data)
}

export function updateInstanceName(data) {
  return post('/instances/update-name', data)
}

// placeholder stubs for later phases
export function changeIP(data) { return post('/instances/change-ip', data) }
export function checkAlive(data) { return post('/instances/check-alive', data) }
```

- [ ] **Step 2: Create `frontend/src/views/Instances.vue`**

Full Vue component with:
- Tenant filter dropdown + keyword search bar
- `el-table` with selection column, ID, Name, Shape, OCPU, Memory, Public IP, State (badge), Actions (Start/Stop/Reboot/Terminate via `el-dropdown`)
- Batch actions bar (appears when rows selected): Batch Start, Check Alive
- Pagination
- Instance detail drawer: shape, boot volume, IPv6, rename
- Change shape dialog: `el-dialog` with OCPU/memory/shape inputs
- Change boot volume dialog: size input

(Full component ~300 lines)

- [ ] **Step 3: Commit**

```bash
git add frontend/src/api/instances.js frontend/src/views/Instances.vue
git commit -m "feat: add Instances page with actions and pagination"
```

### Task 2.5: Instance Create page + instance mutation handlers

**Files:**
- Create: `frontend/src/views/InstanceCreate.vue`
- Modify: `internal/handler/handler.go` (implement phase 2 stubs)

- [ ] **Step 1: Create `frontend/src/views/InstanceCreate.vue`**

Port existing create instance form from `index.html` tab-create to Vue. Tenant selector, AD, image, shape, VCN, subnet, boot size, name fields. Uses `/api/availability-domains`, `/api/images`, `/api/shapes`, `/api/vcns`, `/api/subnets` for dropdowns.

- [ ] **Step 2: Implement instance mutation handlers**

Replace stubs in `handler.go`:

- `handleChangeShape`: parse `{tenant_id, instance_id, shape?, ocpus?, memory_gb?}`, create OCI client, call `UpdateInstance` on compute API
- `handleChangeBootVolume`: parse `{tenant_id, instance_id, size_gb}`, find boot volume via attachment, call `UpdateBootVolume`
- `handleAttachIPv6`: parse `{tenant_id, instance_id}`, find VNIC, assign IPv6
- `handleUpdateInstanceName`: parse `{tenant_id, instance_id, name}`, update display name
- `handleUpdateShape`: parse `{tenant_id, instance_id, shape}`, update shape only

- [ ] **Step 3: Add OCI client methods**

In `internal/oci/client.go`:
- `UpdateInstance(ctx, instanceID string, updateOpts core.UpdateInstanceDetails) error`
- `UpdateBootVolume(ctx, volumeID string, sizeGB int64) error`
- `AssignIPv6(ctx, vnicID string) error`
- `GetInstanceVNICs(ctx, compartmentID, instanceID string) ([]core.Vnic, error)`

- [ ] **Step 4: Build and commit**

```bash
CGO_ENABLED=0 go build -o /dev/null ./cmd/server
git add internal/handler/handler.go internal/oci/client.go \
        frontend/src/views/InstanceCreate.vue
git commit -m "feat: implement instance mutations and create page"
```

---

## Phase 3: Security & Traffic

### Task 3.1: Security Rules backend

**Files:**
- Create: `internal/handler/handler_security.go`
- Modify: `internal/oci/client.go`

- [ ] **Step 1: Create `internal/handler/handler_security.go`**

Implement `handleSecurityRules` routing to sub-handlers based on JSON body field `action`:
- `page` → `handleSecurityRulesPage`
- `addIngress` → `handleSecurityRuleAddIngress`
- `addEgress` → `handleSecurityRuleAddEgress`
- `remove` → `handleSecurityRuleRemove`
- `release` → `handleSecurityRuleRelease`

Each calls OCI security list / NSG APIs.

- [ ] **Step 2: Add OCI methods**

In `internal/oci/client.go`:
- `ListSecurityLists(ctx, compartmentID, vcnID) ([]core.SecurityList, error)`
- `UpdateSecurityList(ctx, securityListID string, rules ...) error`
- `ListNetworkSecurityGroups(ctx, compartmentID, vcnID) ([]core.NetworkSecurityGroup, error)`

- [ ] **Step 3: Build and commit**

### Task 3.2: Security Rules page

**Files:**
- Create: `frontend/src/views/SecurityRules.vue`
- Create: `frontend/src/api/securityRules.js`

- [ ] **Step 1: Create API client**
- [ ] **Step 2: Create Vue component with rule table, add/edit/delete dialogs, open-all-ports button**
- [ ] **Step 3: Commit**

### Task 3.3: Traffic statistics backend + OCI client

**Files:**
- Create: `internal/handler/handler_traffic.go`
- Modify: `internal/oci/client.go`

- [ ] **Step 1: Implement traffic data query using OCI monitoring API**

Query `VnicFromNetworkLoadBalancer` metrics: bytes in/out, packets in/out. Use summary statistics over time window.

- [ ] **Step 2: Build and commit**

### Task 3.4: Traffic page

**Files:**
- Create: `frontend/src/views/Traffic.vue`
- Create: `frontend/src/api/traffic.js`

- [ ] **Step 1: Create component with ECharts line chart, time range picker, instance/VNIC selectors**
- [ ] **Step 2: Commit**

### Task 3.5: Limits query

**Files:**
- Modify: `internal/oci/client.go` (add GetLimits)
- Modify: `internal/handler/handler.go` (implement handleLimits)
- Create: `frontend/src/views/Limits.vue`
- Create: `frontend/src/api/traffic.js` (add limits query)

- [ ] **Step 1: Backend + frontend as above**
- [ ] **Step 2: Commit**

### Task 3.6: One-click 500M

**Files:**
- Modify: `internal/handler/handler.go`
- Modify: `internal/oci/client.go`

- [ ] **Step 1: Implement `handleOneClick500M` and `handleOneClickClose500M`**

Create/destroy NLB attached to instance for 500Mbps AMD free tier acceleration.

- [ ] **Step 2: Commit**

---

## Phase 4: Batch Create (抢机)

### Task 4.1: Batch create backend

**Files:**
- Create: `internal/handler/handler_tasks.go` (handleBatchCreate, handleCreateTasks CRUD)
- Modify: `internal/handler/worker.go` (add batch_create task type)

- [ ] **Step 1: Implement batch create handler**

`handleBatchCreate`: parse config (tenant IDs, instances_per_tenant, region, shape, image_id, subnet_id, AD, boot_size). Create one `tasks` row per tenant with type=`batch_create`, status=`pending`, payload=config JSON.

- [ ] **Step 2: Extend worker for batch_create**

In `processNext()`, add case `"batch_create"`:
- Decode payload (region, shape, image_id, subnet_id, AD, boot_size, count)
- For each instance to create: call OCI `LaunchInstance`, track progress
- Pause/resume: check if task status is "paused" before each attempt
- On completion: status "completed" or "failed"

- [ ] **Step 3: Implement create task CRUD handlers**

`handleCreateTasks`:
- GET: `ListTasksPaginated` with type=`batch_create`
- POST stop: set status `cancelled`
- POST pause: set status `paused`
- POST resume: set status `pending` (worker picks up)
- POST delete: delete task row
- POST update: `UpdateTaskPayload`

- [ ] **Step 4: Build and commit**

### Task 4.2: Batch Create + Create Tasks pages

**Files:**
- Create: `frontend/src/views/InstanceBatchCreate.vue`
- Create: `frontend/src/views/CreateTasks.vue`
- Modify: `frontend/src/api/tasks.js`

- [ ] **Step 1: Create `InstanceBatchCreate.vue`**

Multi-tenant selector (checkboxes), instance config form (region, shape, image, AD, subnet, boot size, count per tenant). Submit creates batch tasks.

- [ ] **Step 2: Create `CreateTasks.vue`**

Task list with status badges, progress bars, action buttons (stop/pause/resume/delete per task or batch). Edit button opens dialog to modify task config.

- [ ] **Step 3: Commit**

---

## Phase 5: IP & Recurring Tasks

### Task 5.1: Change IP retry loop

**Files:**
- Modify: `internal/handler/handler.go` (implement handleChangeIP)
- Modify: `internal/oci/client.go` (add UpdatePublicIP, ListPublicIPs)
- Create: `frontend/src/views/InMemoryTasks.vue`

---

## Phase 6: Monitoring & Settings

### Task 6.1: Logs viewer, Settings page, Check Alive

---

## Phase 7: Polish

### Task 7.1: Home dashboard with server map, VNC console, IP info, dark mode polish

---
