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
