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
          <template #title>{{ $t('menu.home') }}</template>
        </el-menu-item>

        <el-sub-menu index="resources">
          <template #title>
            <el-icon><Monitor /></el-icon>
            <span>{{ $t('menu.resources') }}</span>
          </template>
          <el-menu-item index="/instances">{{ $t('menu.instances') }}</el-menu-item>
          <el-menu-item index="/instances/create">{{ $t('menu.create') }}</el-menu-item>
          <el-menu-item index="/instances/batch-create">{{ $t('menu.batchCreate') }}</el-menu-item>
          <el-menu-item index="/ips">{{ $t('menu.publicIPs') }}</el-menu-item>
          <el-menu-item index="/volumes">{{ $t('menu.bootVolumes') }}</el-menu-item>
          <el-menu-item index="/limits">{{ $t('menu.limits') }}</el-menu-item>
        </el-sub-menu>

        <el-sub-menu index="network">
          <template #title>
            <el-icon><Connection /></el-icon>
            <span>{{ $t('menu.network') }}</span>
          </template>
          <el-menu-item index="/security-rules">{{ $t('menu.securityRules') }}</el-menu-item>
          <el-menu-item index="/traffic">{{ $t('menu.traffic') }}</el-menu-item>
          <el-menu-item index="/cloudflare">{{ $t('menu.cloudflare') }}</el-menu-item>
          <el-menu-item index="/vnc">{{ $t('menu.vnc') }}</el-menu-item>
        </el-sub-menu>

        <el-sub-menu index="tasks-sub">
          <template #title>
            <el-icon><Timer /></el-icon>
            <span>{{ $t('menu.tasks') }}</span>
          </template>
          <el-menu-item index="/create-tasks">{{ $t('menu.createTasks') }}</el-menu-item>
          <el-menu-item index="/mem-tasks">{{ $t('menu.memTasks') }}</el-menu-item>
        </el-sub-menu>

        <el-sub-menu index="system">
          <template #title>
            <el-icon><Tools /></el-icon>
            <span>{{ $t('menu.system') }}</span>
          </template>
          <el-menu-item index="/tenants">{{ $t('menu.tenants') }}</el-menu-item>
          <el-menu-item index="/ai-chat">{{ $t('menu.aiChat') }}</el-menu-item>
          <el-menu-item index="/logs">{{ $t('menu.logs') }}</el-menu-item>
          <el-menu-item index="/audit">{{ $t('menu.audit') }}</el-menu-item>
        </el-sub-menu>

        <el-menu-item index="/backup">
          <el-icon><Upload /></el-icon>
          <template #title>{{ $t('menu.backup') }}</template>
        </el-menu-item>
        <el-menu-item index="/settings">
          <el-icon><Setting /></el-icon>
          <template #title>{{ $t('menu.settings') }}</template>
        </el-menu-item>
      </el-menu>

      <div class="sidebar-footer">
        <el-tooltip :content="collapsed ? $t('app.expand') : $t('app.collapse')" placement="right">
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
          <!-- Language Switcher -->
          <div class="locale-switch">
            <span :class="{ active: $i18n.locale === 'zh-CN' }" @click="setLocale('zh-CN')">中</span>
            <span class="divider">|</span>
            <span :class="{ active: $i18n.locale === 'en' }" @click="setLocale('en')">EN</span>
          </div>
          <el-switch
            v-model="isDark"
            inline-prompt
            :active-icon="Moon"
            :inactive-icon="Sunny"
            @change="toggleDark"
            class="theme-switch"
          />
          <span class="user-name">{{ auth.user?.name }}</span>
          <el-button text class="logout-btn" @click="handleLogout">{{ $t('app.logout') }}</el-button>
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

// Locale toggle: update i18n locale and persist
import { useI18n } from "vue-i18n"
const { locale } = useI18n()
function setLocale(lang) {
  locale.value = lang
  localStorage.setItem('locale', lang)
}

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

.locale-switch {
  display: flex;
  align-items: center;
  gap: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-muted);
  user-select: none;
  cursor: default;
}

.locale-switch span {
  padding: 2px 6px;
  cursor: pointer;
  transition: color var(--transition);
}

.locale-switch span:hover {
  color: var(--text-primary);
}

.locale-switch span.active {
  color: var(--primary);
  font-weight: 700;
}

.locale-switch span.divider {
  color: var(--border-color);
  cursor: default;
  padding: 0 2px;
}

.locale-switch span.divider:hover {
  color: var(--border-color);
}

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
