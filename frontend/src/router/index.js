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
    meta: { titleKey: 'route.home', guest: true }
  },
  {
    path: '/',
    component: () => import('../components/AppLayout.vue'),
    children: [
      { path: '', redirect: '/home' },
      { path: 'home', name: 'Home', component: () => import('../views/Home.vue'), meta: { titleKey: 'route.home', icon: 'HomeFilled' } },
      { path: 'tenants', name: 'Tenants', component: () => import('../views/Tenants.vue'), meta: { titleKey: 'route.tenants', icon: 'User' } },
      { path: 'instances', name: 'Instances', component: () => import('../views/Instances.vue'), meta: { titleKey: 'route.instances', icon: 'Monitor' } },
      { path: 'instances/create', name: 'InstanceCreate', component: () => import('../views/InstanceCreate.vue'), meta: { titleKey: 'route.instanceCreate', icon: 'Plus' } },
      { path: 'instances/batch-create', name: 'InstanceBatchCreate', component: () => import('../views/InstanceBatchCreate.vue'), meta: { titleKey: 'route.instanceBatchCreate', icon: 'Grid' } },
      { path: 'create-tasks', name: 'CreateTasks', component: () => import('../views/CreateTasks.vue'), meta: { titleKey: 'route.createTasks', icon: 'List' } },
      { path: 'security-rules', name: 'SecurityRules', component: () => import('../views/SecurityRules.vue'), meta: { titleKey: 'route.securityRules', icon: 'Lock' } },
      { path: 'traffic', name: 'Traffic', component: () => import('../views/Traffic.vue'), meta: { titleKey: 'route.traffic', icon: 'TrendCharts' } },
      { path: 'limits', name: 'Limits', component: () => import('../views/Limits.vue'), meta: { titleKey: 'route.limits', icon: 'DataAnalysis' } },
      { path: 'ips', name: 'PublicIPs', component: () => import('../views/PublicIPs.vue'), meta: { titleKey: 'route.publicIPs', icon: 'Connection' } },
      { path: 'volumes', name: 'BootVolumes', component: () => import('../views/BootVolumes.vue'), meta: { titleKey: 'route.bootVolumes', icon: 'FolderOpened' } },
      { path: 'cloudflare', name: 'Cloudflare', component: () => import('../views/Cloudflare.vue'), meta: { titleKey: 'route.cloudflare', icon: 'Cloudy' } },
      { path: 'ai-chat', name: 'AiChat', component: () => import('../views/AiChat.vue'), meta: { titleKey: 'route.aiChat', icon: 'ChatDotRound' } },
      { path: 'backup', name: 'Backup', component: () => import('../views/Backup.vue'), meta: { titleKey: 'route.backup', icon: 'Upload' } },
      { path: 'logs', name: 'Logs', component: () => import('../views/Logs.vue'), meta: { titleKey: 'route.logs', icon: 'Document' } },
      { path: 'mem-tasks', name: 'InMemoryTasks', component: () => import('../views/InMemoryTasks.vue'), meta: { titleKey: 'route.memTasks', icon: 'Timer' } },
      { path: 'settings', name: 'Settings', component: () => import('../views/Settings.vue'), meta: { titleKey: 'route.settings', icon: 'Setting' } },
      { path: 'audit', name: 'Audit', component: () => import('../views/Audit.vue'), meta: { titleKey: 'route.audit', icon: 'List' } },
      { path: 'vnc', name: 'VncConsole', component: () => import('../views/VncConsole.vue'), meta: { titleKey: 'route.vnc', icon: 'Monitor' } },
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
