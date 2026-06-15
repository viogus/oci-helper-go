<template>
  <div class="settings-page">
    <!-- Authentication -->
    <el-card shadow="never" class="settings-card">
      <template #header>
        <div class="card-header">
          <span>Authentication</span>
        </div>
      </template>
      <el-descriptions :column="1" border>
        <el-descriptions-item label="Username">
          {{ authStore.user?.name || 'N/A' }}
        </el-descriptions-item>
        <el-descriptions-item label="MFA Status">
          <el-tag
            :type="config.mfa_enabled === 'true' ? 'success' : 'info'"
            size="small"
          >
            {{ config.mfa_enabled === 'true' ? 'Enabled' : 'Disabled' }}
          </el-tag>
          <el-button
            size="small"
            style="margin-left: 12px;"
            @click="toggleMFA"
            :disabled="saving"
          >
            {{ config.mfa_enabled === 'true' ? 'Disable' : 'Enable' }}
          </el-button>
        </el-descriptions-item>
      </el-descriptions>
    </el-card>

    <!-- Notifications -->
    <el-card shadow="never" class="settings-card">
      <template #header>
        <div class="card-header">
          <span>Notifications</span>
        </div>
      </template>
      <el-form label-position="top" @submit.prevent>
        <el-form-item label="Telegram Bot Token">
          <div class="setting-row">
            <el-input
              v-model="config.telegram_token"
              type="password"
              show-password
              placeholder="Enter Telegram bot token"
              :disabled="saving"
            />
            <el-button
              type="primary"
              :loading="saving"
              @click="saveSetting('telegram_token', config.telegram_token)"
            >
              Save
            </el-button>
          </div>
        </el-form-item>
        <el-form-item label="DingTalk Webhook URL">
          <div class="setting-row">
            <el-input
              v-model="config.dingtalk_url"
              placeholder="https://oapi.dingtalk.com/robot/send?access_token=..."
              :disabled="saving"
            />
            <el-button
              type="primary"
              :loading="saving"
              @click="saveSetting('dingtalk_url', config.dingtalk_url)"
            >
              Save
            </el-button>
          </div>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- Google OAuth -->
    <el-card shadow="never" class="settings-card">
      <template #header>
        <div class="card-header">
          <span>Google OAuth</span>
        </div>
      </template>
      <el-form label-position="top" @submit.prevent>
        <el-form-item label="Client ID">
          <div class="setting-row">
            <el-input
              v-model="config.google_client_id"
              placeholder="xxxxxxxxxxxx-xxxxxxxxxxxxxxxxxxxx.apps.googleusercontent.com"
              :disabled="saving"
            />
            <el-button
              type="primary"
              :loading="saving"
              @click="saveSetting('google_client_id', config.google_client_id)"
            >
              Save
            </el-button>
          </div>
        </el-form-item>
        <el-form-item label="Client Secret">
          <div class="setting-row">
            <el-input
              v-model="config.google_client_secret"
              type="password"
              show-password
              placeholder="Enter client secret"
              :disabled="saving"
            />
            <el-button
              type="primary"
              :loading="saving"
              @click="saveSetting('google_client_secret', config.google_client_secret)"
            >
              Save
            </el-button>
          </div>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- Cloudflare -->
    <el-card shadow="never" class="settings-card">
      <template #header>
        <div class="card-header">
          <span>Cloudflare</span>
        </div>
      </template>
      <el-form label-position="top" @submit.prevent>
        <el-form-item label="API Token">
          <div class="setting-row">
            <el-input
              v-model="config.cloudflare_token"
              type="password"
              show-password
              placeholder="Enter Cloudflare API token"
              :disabled="saving"
            />
            <el-button
              type="primary"
              :loading="saving"
              @click="saveSetting('cloudflare_token', config.cloudflare_token)"
            >
              Save
            </el-button>
          </div>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- AI -->
    <el-card shadow="never" class="settings-card">
      <template #header>
        <div class="card-header">
          <span>AI</span>
        </div>
      </template>
      <el-form label-position="top" @submit.prevent>
        <el-form-item label="SiliconFlow API Key">
          <div class="setting-row">
            <el-input
              v-model="config.siliconflow_key"
              type="password"
              show-password
              placeholder="Enter SiliconFlow API key"
              :disabled="saving"
            />
            <el-button
              type="primary"
              :loading="saving"
              @click="saveSetting('siliconflow_key', config.siliconflow_key)"
            >
              Save
            </el-button>
          </div>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- About -->
    <el-card shadow="never" class="settings-card">
      <template #header>
        <div class="card-header">
          <span>About</span>
        </div>
      </template>
      <el-descriptions :column="1" border>
        <el-descriptions-item label="Application">
          oci-helper-go
        </el-descriptions-item>
        <el-descriptions-item label="Version">
          {{ config.version || '1.0.0' }}
        </el-descriptions-item>
        <el-descriptions-item label="Language">
          Go 1.26
        </el-descriptions-item>
        <el-descriptions-item label="Database">
          SQLite (WAL mode)
        </el-descriptions-item>
      </el-descriptions>
    </el-card>

    <el-alert
      v-if="loadError"
      :title="loadError"
      type="error"
      :closable="false"
      show-icon
      style="margin-top: 12px;"
    />
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { get, post } from '../api/index.js'
import { useAuthStore } from '../stores/auth.js'

const authStore = useAuthStore()

const config = reactive({})
const saving = ref(false)
const loadError = ref('')
const loadingConfig = ref(false)

async function loadConfig() {
  loadingConfig.value = true
  loadError.value = ''
  try {
    const r = await get('/config')
    // Merge response into reactive config
    Object.assign(config, r || {})
  } catch (e) {
    loadError.value = e.response?.data?.error || 'Failed to load configuration'
    ElMessage.error('Failed to load configuration')
  } finally {
    loadingConfig.value = false
  }
}

async function saveSetting(key, value) {
  saving.value = true
  try {
    await post('/config', { key, value })
    ElMessage.success('Saved')
  } catch (e) {
    const detail = e.response?.data?.error || 'Failed to save setting'
    ElMessage.error(detail)
  } finally {
    saving.value = false
  }
}

async function toggleMFA() {
  const newVal = config.mfa_enabled === 'true' ? 'false' : 'true'
  await saveSetting('mfa_enabled', newVal)
  config.mfa_enabled = newVal
}

onMounted(() => {
  loadConfig()
})
</script>

<style scoped>
.settings-page {
  padding: 0;
}

.settings-card {
  margin-bottom: 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-header > span {
  font-size: 18px;
  font-weight: 600;
}

.setting-row {
  display: flex;
  gap: 12px;
  align-items: center;
  width: 100%;
}

.setting-row .el-input {
  flex: 1;
}
</style>
