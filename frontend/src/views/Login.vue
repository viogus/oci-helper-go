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
