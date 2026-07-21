import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import * as authApi from '../api/auth.js'
import { setCSRFToken, clearCSRFToken } from '../api/index.js'

export const useAuthStore = defineStore('auth', () => {
  const user = ref(null)
  const isAuthenticated = computed(() => !!user.value)

  async function checkSession() {
    try {
      const cfg = await authApi.getConfig()
      user.value = { name: cfg.username || 'admin' }
      // Fetch CSRF token after session is validated.
      try {
        const csrfResp = await authApi.getCSRFToken()
        if (csrfResp.csrf_token) {
          setCSRFToken(csrfResp.csrf_token)
        }
      } catch {
        // Old session without CSRF token — backend skips check.
      }
      return true
    } catch {
      user.value = null
      clearCSRFToken()
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
