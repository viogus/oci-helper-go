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
          <el-button size="small" :loading="testingAll" @click="handleTestAll">
            Test All
          </el-button>
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
              v-model="config.dingtalk_webhook"
              placeholder="https://oapi.dingtalk.com/robot/send?access_token=..."
              :disabled="saving"
            />
            <el-button
              type="primary"
              :loading="saving"
              @click="saveSetting('dingtalk_webhook', config.dingtalk_webhook)"
            >
              Save
            </el-button>
            <el-button
              :loading="testingDingtalk"
              @click="handleTestDingtalk"
            >
              Test
            </el-button>
          </div>
        </el-form-item>
        <el-form-item label="Telegram Chat ID">
          <div class="setting-row">
            <el-input
              v-model="config.telegram_chat_id"
              placeholder="Personal Telegram chat id for notifications"
              :disabled="saving"
            />
            <el-button
              type="primary"
              :loading="saving"
              @click="saveSetting('telegram_chat_id', config.telegram_chat_id)"
            >
              Save
            </el-button>
          </div>
        </el-form-item>
        <el-form-item label="Telegram Webhook Secret">
          <div class="setting-row">
            <el-input
              v-model="config.telegram_webhook_secret"
              type="password"
              show-password
              placeholder="Secret token for the bot webhook (optional)"
              :disabled="saving"
            />
            <el-button
              type="primary"
              :loading="saving"
              @click="saveSetting('telegram_webhook_secret', config.telegram_webhook_secret)"
            >
              Save
            </el-button>
          </div>
          <div class="setting-hint">
            Guards {{ baseUrl }}/api/telegram/webhook. Set it with Telegram's
            <code>setWebhook</code> call: the bot rejects updates without a matching
            <code>X-Telegram-Bot-Api-Secret-Token</code> header.
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
        <el-form-item label="Allowed Emails">
          <div class="setting-row">
            <el-input
              v-model="config.google_allowed_emails"
              placeholder="Comma-separated emails; empty = allow any Google account"
              :disabled="saving"
            />
            <el-button
              type="primary"
              :loading="saving"
              @click="saveSetting('google_allowed_emails', config.google_allowed_emails)"
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
        <el-form-item label="SiliconFlow Model">
          <div class="setting-row">
            <el-input
              v-model="config.siliconflow_model"
              placeholder="deepseek-ai/DeepSeek-R1-0528-Qwen3-8B"
              :disabled="saving"
            />
            <el-button
              type="primary"
              :loading="saving"
              @click="saveSetting('siliconflow_model', config.siliconflow_model)"
            >
              Save
            </el-button>
          </div>
        </el-form-item>
        <el-form-item label="AI 联网搜索">
          <el-switch
            :model-value="config.ai_search_enabled === 'true'"
            @change="v => saveSetting('ai_search_enabled', v ? 'true' : 'false')"
          />
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" class="settings-card">
      <template #header>
        <div class="card-header">
          <span>Scheduled Notifications</span>
        </div>
      </template>
      <el-form label-position="top" @submit.prevent>
        <el-form-item label="每日播报">
          <el-switch
            :model-value="config.daily_broadcast_enabled === 'true'"
            @change="v => saveSetting('daily_broadcast_enabled', v ? 'true' : 'false')"
          />
        </el-form-item>
        <el-form-item label="每日播报时间 (HH:MM)">
          <div class="setting-row">
            <el-input v-model="config.daily_broadcast_cron" placeholder="08:00" :disabled="saving" />
            <el-button type="primary" :loading="saving" @click="saveSetting('daily_broadcast_cron', config.daily_broadcast_cron)">Save</el-button>
          </div>
        </el-form-item>
        <el-form-item label="版本更新通知">
          <el-switch
            :model-value="config.version_update_notifications_enabled === 'true'"
            @change="v => saveSetting('version_update_notifications_enabled', v ? 'true' : 'false')"
          />
        </el-form-item>
        <el-form-item label="更新仓库 (owner/repo)">
          <div class="setting-row">
            <el-input v-model="config.update_repo" placeholder="viogus/oci-helper-go" :disabled="saving" />
            <el-button type="primary" :loading="saving" @click="saveSetting('update_repo', config.update_repo)">Save</el-button>
          </div>
        </el-form-item>
        <el-form-item label="面板地址 (panel_url)">
          <div class="setting-row">
            <el-input v-model="config.panel_url" placeholder="https://panel.example.com" :disabled="saving" />
            <el-button type="primary" :loading="saving" @click="saveSetting('panel_url', config.panel_url)">Save</el-button>
          </div>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- About & Update -->
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
      <div style="margin-top: 16px; display: flex; gap: 12px; align-items: center;">
        <el-button :loading="checkingUpdate" @click="handleCheckUpdate">
          Check for Updates
        </el-button>
        <el-button v-if="updateInfo" type="primary" :loading="updating" @click="handleUpdateNow">
          Update Now
        </el-button>
      </div>
      <el-alert
        v-if="updateInfo"
        :title="'Latest: ' + updateInfo.tag_name"
        :description="updateInfo.body || 'Release available'"
        type="success"
        :closable="false"
        show-icon
        style="margin-top: 12px;"
      />
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
const baseUrl = window.location.origin

const config = reactive({})
const saving = ref(false)
const loadError = ref('')
const loadingConfig = ref(false)

// Test buttons
const testingAll = ref(false)
const testingDingtalk = ref(false)

// Update check
const checkingUpdate = ref(false)
const updating = ref(false)
const updateInfo = ref(null)

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

async function handleTestAll() {
  testingAll.value = true
  try {
    const res = await post('/notify/test')
    ElMessage.success(res.message || 'Test notification sent')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Test failed')
  } finally {
    testingAll.value = false
  }
}

async function handleTestDingtalk() {
  testingDingtalk.value = true
  try {
    await post('/dingtalk/test')
    ElMessage.success('DingTalk test message sent')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'DingTalk test failed')
  } finally {
    testingDingtalk.value = false
  }
}

async function handleCheckUpdate() {
  checkingUpdate.value = true
  updateInfo.value = null
  try {
    const res = await get('/update/check')
    updateInfo.value = res
    ElMessage.success('Update check complete')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Update check failed')
  } finally {
    checkingUpdate.value = false
  }
}

async function handleUpdateNow() {
  updating.value = true
  try {
    const res = await post('/update/now')
    ElMessage.success(res.message || res.instructions || 'Update instructions sent')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Update failed')
  } finally {
    updating.value = false
  }
}

onMounted(() => {
  loadConfig()
})
</script>

<style scoped>
.settings-page {
  padding: 0;
}

.setting-hint {
  margin-top: 6px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.5;
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
