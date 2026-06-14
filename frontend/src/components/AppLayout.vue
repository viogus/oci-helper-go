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
