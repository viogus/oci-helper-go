<template>
  <div>
    <div class="page-header">
      <el-button @click="$router.push('/tenants')" :icon="'ArrowLeft'" text>{{ $t('common.back') }}</el-button>
      <h3>{{ tenant.name || $t('tenantDetail.title') }}</h3>
      <el-tag :type="tenant.status==='active'?'success':'warning'" size="small">{{ tenant.status }}</el-tag>
    </div>

    <el-card v-loading="loading">
      <template v-if="!loading && tenant.id">
        <!-- Summary cards -->
        <div class="stat-row">
          <div class="stat-card">
            <div class="stat-value">{{ instanceStats?.total || 0 }}</div>
            <div class="stat-label">{{ $t('tenantDetail.totalInstances') }}</div>
          </div>
          <div class="stat-card running">
            <div class="stat-value">{{ instanceStats?.RUNNING || 0 }}</div>
            <div class="stat-label">{{ $t('tenantDetail.running') }}</div>
          </div>
          <div class="stat-card stopped">
            <div class="stat-value">{{ instanceStats?.STOPPED || 0 }}</div>
            <div class="stat-label">{{ $t('tenantDetail.stopped') }}</div>
          </div>
          <div class="stat-card">
            <div class="stat-value">{{ totalOCPU?.toFixed(0) || 0 }}</div>
            <div class="stat-label">{{ $t('tenantDetail.totalOCPU') }}</div>
          </div>
          <div class="stat-card">
            <div class="stat-value">{{ totalMemoryGB?.toFixed(0) || 0 }} GB</div>
            <div class="stat-label">{{ $t('tenantDetail.totalMemory') }}</div>
          </div>
        </div>

        <!-- Regions -->
        <div v-if="regions && regions.length > 0" style="margin-bottom:16px">
          <el-tag v-for="r in regions" :key="r" size="small" style="margin-right:4px;margin-bottom:4px">{{ r }}</el-tag>
        </div>

        <el-descriptions :column="2" border>
          <el-descriptions-item :label="$t('tenantDetail.id')">{{ tenant.id }}</el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.name')">{{ tenant.name }}</el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.accountStatus')">
            <el-tag v-if="tenant.accountStatus" :type="tenant.accountStatus === 'ACTIVE' ? 'success' : 'danger'" size="small">
              {{ tenant.accountStatus || '—' }}
            </el-tag>
            <span v-else>—</span>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.region')">
            <el-tag size="small">{{ tenant.region }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.homeRegion')">
            <el-tag size="small" type="info">{{ tenant.homeRegion || '—' }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.userOcid')" :span="2">
            <code>{{ tenant.userOcid }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.tenancyOcid')" :span="2">
            <code>{{ tenant.tenancyOcid }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.fingerprint')">
            <code>{{ tenant.fingerprint }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.keyFile')">
            <code>{{ tenant.keyFile || '—' }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.createdAt')">{{ formatTime(tenant.createdAt) }}</el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.updatedAt')">{{ formatTime(tenant.updatedAt) }}</el-descriptions-item>
        </el-descriptions>

        <!-- Subscription info -->
        <template v-if="subscription">
          <h4 style="margin-top:24px">{{ $t('tenantDetail.subscription') }}</h4>
          <el-descriptions :column="3" border size="small" style="margin-top:8px">
            <el-descriptions-item label="Plan Type">
              <el-tag size="small" :type="subscription.planType === 'PAYG' ? 'warning' : 'info'">
                {{ subscription.planType || '—' }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="Account Type">{{ subscription.accountType || '—' }}</el-descriptions-item>
            <el-descriptions-item label="Currency">{{ subscription.currencyCode || '—' }}</el-descriptions-item>
            <el-descriptions-item label="Upgrade State">{{ subscription.upgradeState || '—' }}</el-descriptions-item>
            <el-descriptions-item label="Start Time">{{ formatTime(subscription.timeStart) }}</el-descriptions-item>
            <el-descriptions-item label="Intent To Pay">
              <el-tag size="small" :type="subscription.isIntentToPay ? 'success' : 'info'">
                {{ subscription.isIntentToPay ? 'Yes' : 'No' }}
              </el-tag>
            </el-descriptions-item>
          </el-descriptions>
        </template>

        <!-- Identity Users -->
        <h4 style="margin-top:24px">{{ $t('tenantDetail.users') }}</h4>
        <el-table :data="users" stripe size="small" style="margin-top:8px">
          <el-table-column prop="name" :label="$t('tenantDetail.instanceName')" min-width="140" />
          <el-table-column prop="email" label="Email" min-width="180">
            <template #default="{ row }">{{ row.email || '—' }}</template>
          </el-table-column>
          <el-table-column prop="lifecycleState" label="State" width="100" />
          <el-table-column label="MFA" width="80" align="center">
            <template #default="{ row }">
              <el-tag size="small" :type="row.isMfa ? 'success' : 'info'">{{ row.isMfa ? 'On' : 'Off' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="Email Verified" width="110" align="center">
            <template #default="{ row }">
              <el-tag size="small" :type="row.emailVerified ? 'success' : 'info'">{{ row.emailVerified ? 'Yes' : 'No' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="Last Login" width="150">
            <template #default="{ row }">{{ formatTime(row.lastLogin) }}</template>
          </el-table-column>
          <el-table-column :label="$t('stockAlerts.actions')" width="250" fixed="right" align="center">
            <template #default="{ row }">
              <el-button size="small" :loading="acting === 'reset'" @click="openResetPassword(row)">Reset Pwd</el-button>
              <el-button size="small" :loading="acting === 'mfa'" @click="clearMFA(row)">Clear MFA</el-button>
              <el-button size="small" :loading="acting === 'apiKeys'" @click="clearAPIKeys(row)">API Keys</el-button>
              <el-button size="small" type="danger" :loading="acting === 'delete'" @click="deleteUser(row)">Delete</el-button>
            </template>
          </el-table-column>
        </el-table>

        <!-- Password policy + notification recipients -->
        <div style="display:flex;gap:24px;margin-top:24px;flex-wrap:wrap">
          <div>
            <h4 style="margin:0 0 8px">{{ $t('tenantDetail.passwordPolicy') }}</h4>
            <div style="display:flex;gap:8px;align-items:center">
              <el-input-number v-model="passwordExpiresAfter" :min="0" :step="30" />
              <span style="color:#909399;font-size:13px">days</span>
              <el-button size="small" type="primary" :loading="acting === 'policy'" @click="savePasswordPolicy">
                Save
              </el-button>
            </div>
          </div>
          <div>
            <h4 style="margin:0 0 8px">{{ $t('tenantDetail.notificationRecipients') }}</h4>
            <div v-if="notificationRecipients.length" style="display:flex;gap:6px;flex-wrap:wrap">
              <el-tag v-for="r in notificationRecipients" :key="r" size="small">{{ r }}</el-tag>
            </div>
            <span v-else style="color:#909399;font-size:13px">—</span>
          </div>
        </div>

        <h4 style="margin-top:24px">{{ $t('tenantDetail.instances') }}</h4>
        <el-table :data="instances" stripe size="small" style="margin-top:12px">
          <el-table-column prop="name" :label="$t('tenantDetail.instanceName')" min-width="140" />
          <el-table-column prop="shape" :label="$t('tenantDetail.shape')" width="200" />
          <el-table-column :label="$t('tenantDetail.config')" width="160">
            <template #default="{ row }">{{ row.ocpu }}c / {{ row.memoryGB }}G / {{ row.bootVolumeGB }}G</template>
          </el-table-column>
          <el-table-column prop="publicIp" :label="$t('tenantDetail.publicIP')" width="150" />
          <el-table-column prop="state" :label="$t('tenantDetail.state')" width="110">
            <template #default="{ row }">
              <el-tag :type="row.state==='RUNNING'?'success':row.state==='STOPPED'?'danger':'info'" size="small">{{ row.state }}</el-tag>
            </template>
          </el-table-column>
        </el-table>
      </template>
      <el-empty v-if="!loading && !tenant.id" :description="$t('tenantDetail.notFound')" />
    </el-card>

    <!-- Reset password dialog -->
    <el-dialog v-model="resetPwdDialog" title="Reset Password" width="400px">
      <p style="margin-top:0">
        Reset password for <strong>{{ resetPwdUser?.name }}</strong>:
      </p>
      <el-input
        v-model="resetPwdNew"
        type="password"
        show-password
        placeholder="New password"
        @keyup.enter="confirmResetPassword"
      />
      <template #footer>
        <el-button @click="resetPwdDialog = false">Cancel</el-button>
        <el-button type="primary" :loading="acting === 'reset'" @click="confirmResetPassword">
          Reset
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { get, post } from '../api/index.js'

const route = useRoute()
const loading = ref(true)
const tenant = ref({})
const instances = ref([])
const regions = ref([])
const instanceStats = ref(null)
const totalOCPU = ref(0)
const totalMemoryGB = ref(0)
const users = ref([])
const subscription = ref(null)
const passwordExpiresAfter = ref(0)
const notificationRecipients = ref([])

// Dialogs
const resetPwdDialog = ref(false)
const resetPwdUser = ref(null)
const resetPwdNew = ref('')
const acting = ref('')

onMounted(async () => {
  const id = route.params.id
  try {
    const [infoRes, iRes] = await Promise.all([
      get(`/tenants/${id}/info`),
      get('/instances', { tenant_id: id, size: 500 }),
    ])
    tenant.value = infoRes?.tenant || {}
    regions.value = infoRes?.regions || []
    instanceStats.value = infoRes?.instanceStats || {}
    totalOCPU.value = infoRes?.totalOCPU || 0
    totalMemoryGB.value = infoRes?.totalMemoryGB || 0
    instances.value = iRes?.data || []
    users.value = infoRes?.users || []
    subscription.value = infoRes?.subscription || null
    passwordExpiresAfter.value = infoRes?.passwordExpiresAfter || 0
    notificationRecipients.value = infoRes?.notificationRecipients || []
  } catch {
    // Fallback: basic tenant info
    try {
      const tRes = await get(`/tenants/${id}`)
      tenant.value = tRes || {}
    } catch {}
  }
  loading.value = false
})

function formatTime(t) {
  if (!t) return '—'
  return new Date(t).toLocaleString()
}

function refreshInfo() {
  const id = route.params.id
  get(`/tenants/${id}/info`)
    .then((res) => {
      users.value = res?.users || users.value
      subscription.value = res?.subscription || subscription.value
      passwordExpiresAfter.value = res?.passwordExpiresAfter ?? passwordExpiresAfter.value
      notificationRecipients.value = res?.notificationRecipients || notificationRecipients.value
    })
    .catch(() => {})
}

async function openResetPassword(user) {
  resetPwdUser.value = user
  resetPwdNew.value = ''
  resetPwdDialog.value = true
}

async function confirmResetPassword() {
  if (!resetPwdNew.value) {
    ElMessage.warning('New password required')
    return
  }
  acting.value = 'reset'
  try {
    await post(`/tenants/${tenant.value.id}/users/reset-password`, {
      user_id: resetPwdUser.value.id,
      new_password: resetPwdNew.value
    })
    ElMessage.success('Password reset')
    resetPwdDialog.value = false
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Reset failed')
  } finally {
    acting.value = ''
  }
}

async function clearMFA(user) {
  try {
    await ElMessageBox.confirm(`Clear MFA devices for "${user.name}"?`, 'Clear MFA', {
      confirmButtonText: 'Clear',
      cancelButtonText: 'Cancel',
      type: 'warning'
    })
  } catch {
    return
  }
  acting.value = 'mfa'
  try {
    const res = await post(`/tenants/${tenant.value.id}/mfa/clear`, { user_id: user.id })
    ElMessage.success(res?.message || 'MFA devices cleared')
    refreshInfo()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to clear MFA')
  } finally {
    acting.value = ''
  }
}

async function clearAPIKeys(user) {
  try {
    await ElMessageBox.confirm(`Clear API keys for "${user.name}"?`, 'Clear API Keys', {
      confirmButtonText: 'Clear',
      cancelButtonText: 'Cancel',
      type: 'warning'
    })
  } catch {
    return
  }
  acting.value = 'apiKeys'
  try {
    const res = await post(`/tenants/${tenant.value.id}/api-keys/clear`, { user_id: user.id })
    ElMessage.success(res?.message || 'API keys cleared')
    refreshInfo()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to clear API keys')
  } finally {
    acting.value = ''
  }
}

async function deleteUser(user) {
  try {
    await ElMessageBox.confirm(`Delete user "${user.name}"? This cannot be undone.`, 'Delete User', {
      confirmButtonText: 'Delete',
      cancelButtonText: 'Cancel',
      type: 'error'
    })
  } catch {
    return
  }
  acting.value = 'delete'
  try {
    await post(`/tenants/${tenant.value.id}/users/delete`, { user_id: user.id })
    ElMessage.success('User deleted')
    refreshInfo()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Delete failed')
  } finally {
    acting.value = ''
  }
}

async function savePasswordPolicy() {
  acting.value = 'policy'
  try {
    await post(`/tenants/${tenant.value.id}/password-policy`, {
      password_expires_after: Number(passwordExpiresAfter.value) || 0
    })
    ElMessage.success('Password policy saved')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Save failed')
  } finally {
    acting.value = ''
  }
}
</script>

<style scoped>
.page-header { display:flex; align-items:center; gap:12px; margin-bottom:12px }
.page-header h3 { margin:0 }
code { font-size:12px; word-break:break-all }
.stat-row { display:flex; gap:12px; margin-bottom:16px; flex-wrap:wrap }
.stat-card {
  flex:1; background:var(--card-bg); border-radius:8px; padding:14px 18px;
  box-shadow:var(--shadow-sm); max-width:140px; min-width:100px; text-align:center
}
.stat-card.running { border-left:3px solid #67C23A }
.stat-card.stopped { border-left:3px solid #F56C6C }
.stat-value { font-size:24px; font-weight:700; color:var(--text-primary) }
.stat-label { font-size:11px; color:var(--text-muted); margin-top:2px }
</style>
