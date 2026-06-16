<template>
  <div class="app-shell" :class="{ 'sidebar-collapsed': collapsed }">
    <!-- Sidebar -->
    <aside class="app-sidebar">
      <div class="sidebar-header" @click="$router.push('/home')">
        <div class="logo-icon">O</div>
        <span class="logo-text">oci-helper</span>
      </div>

      <el-menu
        :default-active="route.path"
        :collapse="collapsed"
        :router="true"
        :unique-opened="true"
        class="sidebar-menu"
      >
        <el-menu-item index="/home">
          <el-icon><HomeFilled /></el-icon>
          <template #title>Home</template>
        </el-menu-item>

        <el-sub-menu index="resources">
          <template #title>
            <el-icon><Monitor /></el-icon>
            <span>Resources</span>
          </template>
          <el-menu-item index="/instances">Instances</el-menu-item>
          <el-menu-item index="/instances/create">Create</el-menu-item>
          <el-menu-item index="/instances/batch-create">Batch Create</el-menu-item>
          <el-menu-item index="/ips">Public IPs</el-menu-item>
          <el-menu-item index="/volumes">Boot Volumes</el-menu-item>
          <el-menu-item index="/limits">Limits</el-menu-item>
        </el-sub-menu>

        <el-sub-menu index="network">
          <template #title>
            <el-icon><Connection /></el-icon>
            <span>Network</span>
          </template>
          <el-menu-item index="/security-rules">Security Rules</el-menu-item>
          <el-menu-item index="/traffic">Traffic</el-menu-item>
          <el-menu-item index="/cloudflare">Cloudflare</el-menu-item>
          <el-menu-item index="/vnc">VNC Console</el-menu-item>
        </el-sub-menu>

        <el-sub-menu index="tasks-sub">
          <template #title>
            <el-icon><Timer /></el-icon>
            <span>Tasks</span>
          </template>
          <el-menu-item index="/create-tasks">Create Tasks</el-menu-item>
          <el-menu-item index="/mem-tasks">Memory Tasks</el-menu-item>
        </el-sub-menu>

        <el-sub-menu index="system">
          <template #title>
            <el-icon><Tools /></el-icon>
            <span>System</span>
          </template>
          <el-menu-item index="/tenants">Tenants</el-menu-item>
          <el-menu-item index="/ai-chat">AI Chat</el-menu-item>
          <el-menu-item index="/logs">Logs</el-menu-item>
          <el-menu-item index="/audit">Audit Log</el-menu-item>
        </el-sub-menu>

        <el-menu-item index="/backup">
          <el-icon><Upload /></el-icon>
          <template #title>Backup</template>
        </el-menu-item>
        <el-menu-item index="/settings">
          <el-icon><Setting /></el-icon>
          <template #title>Settings</template>
        </el-menu-item>
      </el-menu>

      <div class="sidebar-footer">
        <el-tooltip :content="collapsed ? 'Expand' : 'Collapse'" placement="right">
          <el-button text :icon="Fold" @click="collapsed = !collapsed" class="collapse-btn" />
        </el-tooltip>
      </div>
    </aside>

    <!-- Main Area -->
    <div class="app-main">
      <header class="app-header">
        <div class="header-left">
          <el-button text :icon="Fold" @click="collapsed = !collapsed" class="header-collapse" />
        </div>
        <div class="header-right">
          <el-switch
            v-model="isDark"
            inline-prompt
            :active-icon="Moon"
            :inactive-icon="Sunny"
            @change="toggleDark"
            class="theme-switch"
          />
          <span class="user-name">{{ auth.user?.name }}</span>
          <el-button text class="logout-btn" @click="handleLogout">Logout</el-button>
        </div>
      </header>

      <main class="app-content">
        <router-view v-slot="{ Component }">
          <transition name="fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </main>
    </div>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth.js'
import { Fold, Moon, Sunny } from '@element-plus/icons-vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const collapsed = ref(localStorage.getItem('sidebar_collapsed') === 'true')
const isDark = ref(localStorage.getItem('theme') === 'dark')

// Apply dark mode on mount
if (isDark.value) document.documentElement.classList.add('dark')

function toggleDark(v) {
  localStorage.setItem('theme', v ? 'dark' : 'light')
  document.documentElement.classList.toggle('dark', v)
}

watch(collapsed, v => localStorage.setItem('sidebar_collapsed', v))

async function handleLogout() {
  await auth.doLogout()
  router.push('/login')
}
</script>

<style scoped>
.app-shell {
  display: flex;
  height: 100vh;
  overflow: hidden;
}

/* ── Sidebar ──────────────────────────────────────────────── */
.app-sidebar {
  width: var(--sidebar-width);
  min-width: var(--sidebar-width);
  background: var(--sidebar-bg);
  display: flex;
  flex-direction: column;
  transition: width var(--transition), min-width var(--transition);
  overflow: hidden;
  z-index: 100;
}

.sidebar-collapsed .app-sidebar {
  width: var(--sidebar-collapsed-width);
  min-width: var(--sidebar-collapsed-width);
}

.sidebar-header {
  height: 60px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 18px;
  border-bottom: 1px solid rgba(255,255,255,0.06);
  cursor: pointer;
  flex-shrink: 0;
}

.logo-icon {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: linear-gradient(135deg, var(--primary), #6366f1);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 16px;
  flex-shrink: 0;
}

.logo-text {
  font-size: 16px;
  font-weight: 700;
  color: #f1f5f9;
  white-space: nowrap;
  transition: opacity var(--transition);
}

.sidebar-collapsed .logo-text {
  opacity: 0;
  width: 0;
  overflow: hidden;
}

/* Menu */
.sidebar-menu {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  border-right: none !important;
  background: transparent !important;
  padding: 8px 0;
}

.sidebar-menu :deep(.el-menu-item),
.sidebar-menu :deep(.el-sub-menu__title) {
  height: 42px;
  line-height: 42px;
  color: var(--sidebar-text) !important;
  border-radius: 0 20px 20px 0;
  margin: 1px 0;
  padding-left: 20px !important;
  transition: all var(--transition);
}

.sidebar-menu :deep(.el-menu-item:hover),
.sidebar-menu :deep(.el-sub-menu__title:hover) {
  background: var(--sidebar-hover) !important;
  color: var(--sidebar-active-text) !important;
}

.sidebar-menu :deep(.el-menu-item.is-active) {
  background: var(--sidebar-active) !important;
  color: var(--sidebar-active-text) !important;
  font-weight: 600;
  border-left: 3px solid var(--primary);
  padding-left: 17px !important;
}

.sidebar-menu :deep(.el-sub-menu.is-active > .el-sub-menu__title) {
  color: var(--sidebar-active-text) !important;
}

.sidebar-menu :deep(.el-menu--inline .el-menu-item) {
  padding-left: 52px !important;
  font-size: 13px;
}

.sidebar-menu :deep(.el-icon) {
  font-size: 18px;
  margin-right: 10px;
}

/* Footer */
.sidebar-footer {
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-top: 1px solid rgba(255,255,255,0.06);
  flex-shrink: 0;
}

.collapse-btn {
  color: var(--sidebar-text) !important;
  font-size: 18px;
}
.collapse-btn:hover { color: var(--sidebar-active-text) !important; }

.sidebar-collapsed .sidebar-footer { padding: 0; }

/* ── Main Area ────────────────────────────────────────────── */
.app-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  min-width: 0;
}

/* Header */
.app-header {
  height: var(--header-height);
  min-height: var(--header-height);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  background: var(--header-bg);
  border-bottom: 1px solid var(--header-border);
}

.header-left, .header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-collapse {
  font-size: 18px;
  color: var(--text-secondary) !important;
}
.header-collapse:hover { color: var(--text-primary) !important; }

.theme-switch { margin: 0 4px; }

.user-name {
  font-size: 13px;
  color: var(--text-secondary);
  font-weight: 500;
}

.logout-btn {
  color: var(--text-muted) !important;
  font-size: 13px;
}
.logout-btn:hover { color: #ef4444 !important; }

/* Content */
.app-content {
  flex: 1;
  overflow-y: auto;
  padding: 24px;
  background: var(--main-bg);
}
</style>
