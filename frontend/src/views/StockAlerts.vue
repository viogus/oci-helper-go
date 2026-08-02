<template>
  <div class="stock-alerts-page">
    <div class="filter-bar">
      <h3 class="page-title">{{ $t('stockAlerts.title') }}</h3>
      <div style="display:flex;gap:8px">
        <el-button @click="load" :loading="loading">
          {{ $t('stockAlerts.refresh') }}
        </el-button>
        <el-button type="primary" @click="openAdd">
          {{ $t('stockAlerts.add') }}
        </el-button>
      </div>
    </div>

    <el-alert
      v-if="hint"
      :title="hint"
      type="info"
      :closable="false"
      show-icon
      style="margin-bottom:12px"
    />

    <el-table :data="alerts" v-loading="loading" border stripe style="width:100%">
      <el-table-column prop="region" :label="$t('stockAlerts.region')" width="140" />
      <el-table-column prop="shape" :label="$t('stockAlerts.shape')" min-width="200" />
      <el-table-column prop="availabilityDomain" :label="$t('stockAlerts.ad')" width="140">
        <template #default="{ row }">{{ row.availabilityDomain || '—' }}</template>
      </el-table-column>
      <el-table-column prop="chatId" :label="$t('stockAlerts.chatId')" width="120">
        <template #default="{ row }">{{ row.chatId || '—' }}</template>
      </el-table-column>
      <el-table-column :label="$t('stockAlerts.enabled')" width="100" align="center">
        <template #default="{ row }">
          <el-switch :model-value="row.enabled" @change="(v) => toggleEnabled(row, v)" />
        </template>
      </el-table-column>
      <el-table-column :label="$t('stockAlerts.lastStatus')" min-width="160">
        <template #default="{ row }">
          <el-tag v-if="row.lastStockStatus" :type="row.lastStockStatus === 'IN_STOCK' ? 'success' : 'info'" size="small">
            {{ row.lastStockStatus }}
          </el-tag>
          <span v-else>—</span>
          <span v-if="row.lastCheckedAt" class="stock-last-checked">
            {{ formatTime(row.lastCheckedAt) }}
          </span>
        </template>
      </el-table-column>
      <el-table-column :label="$t('stockAlerts.actions')" width="200" fixed="right" align="center">
        <template #default="{ row }">
          <el-button size="small" @click="checkNow(row)">Check</el-button>
          <el-button size="small" type="danger" @click="remove(row)">Delete</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-empty v-if="!loading && alerts.length === 0" :description="$t('stockAlerts.empty')" />

    <!-- Add dialog -->
    <el-dialog v-model="dialogVisible" :title="$t('stockAlerts.addTitle')" width="480px">
      <el-form label-width="110px" @submit.prevent>
        <el-form-item :label="$t('stockAlerts.tenant')">
          <el-select v-model="form.tenantId" filterable style="width:100%" placeholder="Select tenant">
            <el-option v-for="t in tenants" :key="t.id" :label="t.name" :value="t.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('stockAlerts.region')">
          <el-input v-model="form.region" placeholder="e.g. ap-singapore-1" />
        </el-form-item>
        <el-form-item :label="$t('stockAlerts.shape')">
          <el-input v-model="form.shape" placeholder="e.g. VM.Standard.A1.Flex" />
        </el-form-item>
        <el-form-item :label="$t('stockAlerts.ad')">
          <el-input v-model="form.availabilityDomain" placeholder="optional" />
        </el-form-item>
        <el-form-item :label="$t('stockAlerts.chatId')">
          <el-input v-model.number="form.chatId" placeholder="Telegram chat id (defaults to telegram_chat_id)" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="save">{{ $t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { get, post, put, del } from '../api/index.js'

const loading = ref(false)
const saving = ref(false)
const alerts = ref([])
const tenants = ref([])
const dialogVisible = ref(false)
const hint = ref('')
const form = reactive({
  tenantId: null,
  region: '',
  shape: '',
  availabilityDomain: '',
  chatId: null
})

async function load() {
  loading.value = true
  try {
    const res = await get('/stock-alerts')
    alerts.value = Array.isArray(res) ? res : []
    hint.value = alerts.value.length
      ? ''
      : 'Alerts are checked every 60s by the background monitor and notify the configured Telegram chat.'
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to load stock alerts')
  } finally {
    loading.value = false
  }
}

async function loadTenants() {
  try {
    const res = await get('/tenants')
    tenants.value = Array.isArray(res) ? res : (res.data || [])
  } catch {
    tenants.value = []
  }
}

function openAdd() {
  Object.assign(form, { tenantId: null, region: '', shape: '', availabilityDomain: '', chatId: null })
  dialogVisible.value = true
}

async function save() {
  if (!form.tenantId || !form.region || !form.shape) {
    ElMessage.warning('Tenant, region and shape are required')
    return
  }
  saving.value = true
  try {
    await post('/stock-alerts', { ...form })
    ElMessage.success('Stock alert created')
    dialogVisible.value = false
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to create alert')
  } finally {
    saving.value = false
  }
}

async function toggleEnabled(row, val) {
  try {
    await put('/stock-alerts/' + row.id, { ...row, enabled: val })
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to update alert')
    load()
  }
}

async function checkNow(row) {
  try {
    await post(`/stock-alerts/${row.id}/check`)
    ElMessage.success('Check triggered')
    setTimeout(load, 2000)
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Check failed')
  }
}

async function remove(row) {
  try {
    await ElMessageBox.confirm(`Delete stock alert for ${row.shape} (${row.region})?`, 'Delete Alert', {
      confirmButtonText: 'Delete',
      cancelButtonText: 'Cancel',
      type: 'warning'
    })
  } catch {
    return
  }
  try {
    await del('/stock-alerts/' + row.id)
    ElMessage.success('Deleted')
    load()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Delete failed')
  }
}

function formatTime(t) {
  if (!t) return ''
  const d = new Date(t)
  return ` · ${d.toLocaleString()}`
}

onMounted(() => {
  load()
  loadTenants()
})
</script>

<style scoped>
.stock-alerts-page {
  padding: 4px;
}

.filter-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.page-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}

.stock-last-checked {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
