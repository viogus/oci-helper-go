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
      { path: 'audit', name: 'Audit', component: () => import('../views/Audit.vue'), meta: { title: 'Audit Log', icon: 'List' } },
      { path: 'vnc', name: 'VncConsole', component: () => import('../views/VncConsole.vue'), meta: { title: 'VNC Console', icon: 'Monitor' } },
    ]
  },
  {
    path: '/oauth/callback',
    redirect: '/home'
  }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes
})

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
