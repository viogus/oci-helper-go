<template>
  <div class="login-page">
    <div class="login-bg"></div>
    <div class="login-container">
      <div class="login-card">
        <div class="login-header">
          <div class="login-logo">O</div>
          <h1>oci-helper</h1>
          <p>Oracle Cloud Infrastructure Manager</p>
        </div>
        <el-form @submit.prevent="handleLogin" class="login-form">
          <el-form-item>
            <el-input
              v-model="username"
              placeholder="Username"
              size="large"
              :prefix-icon="User"
            />
          </el-form-item>
          <el-form-item>
            <el-input
              v-model="password"
              type="password"
              show-password
              placeholder="Password"
              size="large"
              :prefix-icon="Lock"
            />
          </el-form-item>
          <el-form-item v-if="needMfa">
            <el-input
              v-model="totp"
              placeholder="6-digit MFA code"
              maxlength="6"
              size="large"
            />
          </el-form-item>
          <div v-if="error" class="login-error">{{ error }}</div>
          <el-form-item>
            <el-button type="primary" native-type="submit" :loading="loading" class="login-btn" size="large">
              {{ needMfa ? 'Verify' : 'Sign In' }}
            </el-button>
          </el-form-item>
        </el-form>
        <div class="login-footer">
          <el-button link type="primary" @click="googleLogin" :disabled="loading">
            
            Sign in with Google
          </el-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '../stores/auth.js'
import { User, Lock, Chrome } from '@element-plus/icons-vue'

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
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  background: var(--main-bg, #f8fafc);
  overflow: hidden;
}

.login-bg {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(ellipse at 20% 50%, rgba(37,99,235,0.08) 0%, transparent 50%),
    radial-gradient(ellipse at 80% 20%, rgba(99,102,241,0.06) 0%, transparent 50%),
    radial-gradient(ellipse at 40% 80%, rgba(37,99,235,0.04) 0%, transparent 50%);
}

.dark .login-bg {
  background:
    radial-gradient(ellipse at 20% 50%, rgba(37,99,235,0.15) 0%, transparent 50%),
    radial-gradient(ellipse at 80% 20%, rgba(99,102,241,0.1) 0%, transparent 50%);
}

.login-container {
  position: relative;
  z-index: 1;
  width: 100%;
  max-width: 400px;
  padding: 20px;
}

.login-card {
  background: var(--card-bg, #ffffff);
  border-radius: var(--border-radius-lg, 14px);
  box-shadow: var(--shadow-lg, 0 20px 25px -5px rgba(0,0,0,0.1));
  padding: 40px 32px;
  backdrop-filter: blur(20px);
}

.login-header {
  text-align: center;
  margin-bottom: 32px;
}

.login-logo {
  width: 52px;
  height: 52px;
  border-radius: 14px;
  background: linear-gradient(135deg, var(--primary, #2563eb), #6366f1);
  color: #fff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  font-weight: 700;
  margin-bottom: 16px;
  box-shadow: 0 4px 12px rgba(37,99,235,0.3);
}

.login-header h1 {
  margin: 0;
  font-size: 22px;
  font-weight: 700;
  color: var(--text-primary, #0f172a);
  letter-spacing: -0.01em;
}

.login-header p {
  margin: 6px 0 0;
  font-size: 13px;
  color: var(--text-muted, #94a3b8);
}

.login-form {
  margin-top: 4px;
}

.login-form :deep(.el-form-item) {
  margin-bottom: 16px;
}

.login-form :deep(.el-input__wrapper) {
  padding: 4px 12px;
}

.login-btn {
  width: 100%;
  height: 44px;
  font-size: 15px;
  font-weight: 600;
}

.login-error {
  color: #ef4444;
  font-size: 13px;
  text-align: center;
  margin-bottom: 12px;
  padding: 8px 12px;
  background: #fef2f2;
  border-radius: var(--border-radius-sm, 6px);
}

.dark .login-error {
  background: rgba(239,68,68,0.1);
}

.login-footer {
  text-align: center;
  margin-top: 8px;
}
</style>
