<template>
  <div class="cloudflare-page">
    <!-- OCI DNS Auto-Sync (backend monitor runs every 60s when enabled) -->
    <el-card shadow="never" class="autosync-card">
      <template #header>
        <div class="autosync-header">
          <span>OCI DNS Auto-Sync</span>
          <el-switch v-model="autoSync.enabled" @change="toggleAutoSync" />
        </div>
      </template>
      <div class="autosync-row">
        <el-input v-model="autoSync.zoneId" placeholder="Zone ID" style="width: 200px" />
        <el-input
          v-model="autoSync.domain"
          placeholder="Domain (e.g. vps.example.com)"
          style="width: 260px"
        />
        <el-button @click="saveAutoSync" :loading="savingAutoSync">Save</el-button>
        <el-button
          type="primary"
          :loading="syncing"
          :disabled="!autoSync.enabled"
          @click="triggerAutoSync"
        >
          Sync Now
        </el-button>
      </div>
      <div v-if="autoSync.lastSync" class="autosync-last">
        Last sync: {{ autoSync.lastSync }} · {{ autoSync.lastCount }} record(s)
      </div>
    </el-card>

    <!-- Zone selector and actions -->
    <div class="filter-bar">
      <el-select
        v-model="selectedZoneId"
        placeholder="Select a zone..."
        clearable
        @change="onZoneChange"
        style="width: 300px"
        :loading="zonesLoading"
      >
        <el-option
          v-for="z in zones"
          :key="z.id"
          :label="z.name"
          :value="z.id"
        />
      </el-select>
      <el-button @click="loadZones" :loading="zonesLoading">
        Refresh Zones
      </el-button>
      <el-button
        type="primary"
        :disabled="!selectedZoneId"
        @click="openAddDialog"
      >
        Add Record
      </el-button>
      <el-button
        @click="loadRecords"
        :loading="recordsLoading"
        :disabled="!selectedZoneId"
      >
        Refresh Records
      </el-button>
    </div>

    <template v-if="selectedZoneId">
      <!-- DNS Records Table -->
      <el-table
        :data="records"
        v-loading="recordsLoading"
        border
        stripe
        style="width: 100%"
        element-loading-text="Loading DNS records..."
      >
        <el-table-column label="Name" min-width="200">
          <template #default="{ row }">
            <span>{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="type" label="Type" width="90" align="center" />
        <el-table-column label="Content" min-width="280">
          <template #default="{ row }">
            <span class="record-content">{{ row.content }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="ttl" label="TTL" width="80" align="center">
          <template #default="{ row }">
            {{ row.ttl === 1 ? 'Auto' : row.ttl }}
          </template>
        </el-table-column>
        <el-table-column label="Proxied" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="row.proxied ? 'success' : 'info'" size="small">
              {{ row.proxied ? 'Yes' : 'No' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Actions" width="150" fixed="right" align="center">
          <template #default="{ row }">
            <el-button type="primary" link size="small" @click="openEditDialog(row)">
              Edit
            </el-button>
            <el-button type="danger" link size="small" @click="handleDelete(row)">
              Delete
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-empty
        v-if="!recordsLoading && records.length === 0"
        description="No DNS records found for this zone"
      />
    </template>

    <el-empty
      v-if="!selectedZoneId && !zonesLoading"
      description="Select a zone above to view DNS records"
    />

    <!-- Add / Edit Record Dialog -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEditing ? 'Edit DNS Record' : 'Add DNS Record'"
      width="520px"
      :close-on-click-modal="false"
    >
      <el-form :model="recordForm" label-width="100px">
        <el-form-item label="Name" required>
          <el-input
            v-model="recordForm.name"
            placeholder="e.g. www, @, api, mail"
          />
        </el-form-item>
        <el-form-item label="Type" required>
          <el-select v-model="recordForm.type" style="width: 100%">
            <el-option label="A" value="A" />
            <el-option label="AAAA" value="AAAA" />
            <el-option label="CNAME" value="CNAME" />
            <el-option label="TXT" value="TXT" />
            <el-option label="MX" value="MX" />
          </el-select>
        </el-form-item>
        <el-form-item label="Content" required>
          <el-input
            v-model="recordForm.content"
            placeholder="IP address or target hostname"
          />
        </el-form-item>
        <el-form-item label="TTL">
          <el-input-number
            v-model="recordForm.ttl"
            :min="60"
            :max="86400"
            :step="60"
            controls-position="right"
            style="width: 200px"
          />
          <span style="margin-left: 8px; color: #909399; font-size: 12px;">
            (1 = Auto)
          </span>
        </el-form-item>
        <el-form-item label="Proxied">
          <el-switch v-model="recordForm.proxied" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">Cancel</el-button>
        <el-button type="primary" :loading="saving" @click="handleSave">
          {{ isEditing ? 'Update' : 'Create' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { listZones, listRecords, createRecord, updateRecord, deleteRecord } from '../api/cloudflare.js'
import { get, post } from '../api/index.js'

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------
const zones = ref([])
const records = ref([])
const selectedZoneId = ref('')
const zonesLoading = ref(false)
const recordsLoading = ref(false)
const saving = ref(false)

// Dialog state
const dialogVisible = ref(false)
const isEditing = ref(false)
const editingRecordId = ref('')
const recordForm = reactive({
  name: '',
  type: 'A',
  content: '',
  ttl: 120,
  proxied: false
})

// ---------------------------------------------------------------------------
// Data loading
// ---------------------------------------------------------------------------
async function loadZones() {
  zonesLoading.value = true
  try {
    zones.value = await listZones() || []
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to load zones: ' + msg)
  } finally {
    zonesLoading.value = false
  }
}

async function loadRecords() {
  if (!selectedZoneId.value) return
  recordsLoading.value = true
  try {
    records.value = await listRecords(selectedZoneId.value) || []
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to load records: ' + msg)
  } finally {
    recordsLoading.value = false
  }
}

function onZoneChange() {
  records.value = []
  if (selectedZoneId.value) {
    loadRecords()
  }
}

// ---------------------------------------------------------------------------
// Add / Edit
// ---------------------------------------------------------------------------
function resetForm() {
  recordForm.name = ''
  recordForm.type = 'A'
  recordForm.content = ''
  recordForm.ttl = 120
  recordForm.proxied = false
}

function openAddDialog() {
  isEditing.value = false
  editingRecordId.value = ''
  resetForm()
  dialogVisible.value = true
}

function openEditDialog(row) {
  isEditing.value = true
  editingRecordId.value = row.id
  recordForm.name = row.name || ''
  recordForm.type = row.type || 'A'
  recordForm.content = row.content || ''
  recordForm.ttl = typeof row.ttl === 'number' ? row.ttl : 120
  recordForm.proxied = !!row.proxied
  dialogVisible.value = true
}

async function handleSave() {
  if (!recordForm.name || !recordForm.content) {
    ElMessage.warning('Name and Content are required')
    return
  }
  saving.value = true
  try {
    const payload = {
      type: recordForm.type,
      name: recordForm.name,
      content: recordForm.content,
      ttl: recordForm.ttl,
      proxied: recordForm.proxied
    }
    if (isEditing.value) {
      await updateRecord(selectedZoneId.value, editingRecordId.value, payload)
      ElMessage.success('Record updated')
    } else {
      await createRecord(selectedZoneId.value, payload)
      ElMessage.success('Record created')
    }
    dialogVisible.value = false
    await loadRecords()
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error((isEditing.value ? 'Update' : 'Create') + ' failed: ' + msg)
  } finally {
    saving.value = false
  }
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------
async function handleDelete(row) {
  try {
    await ElMessageBox.confirm(
      `Delete DNS record "${row.name}" (${row.type})?`,
      'Confirm Delete',
      {
        confirmButtonText: 'Delete',
        cancelButtonText: 'Cancel',
        type: 'warning'
      }
    )
    await deleteRecord(selectedZoneId.value, row.id)
    ElMessage.success('Record deleted')
    await loadRecords()
  } catch {
    // User cancelled or error
  }
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------
onMounted(() => {
  loadZones()
  loadAutoSync()
})

// ---------------------------------------------------------------------------
// OCI DNS Auto-Sync
// ---------------------------------------------------------------------------
const autoSync = ref({ enabled: false, zoneId: '', domain: '', lastSync: '', lastCount: 0 })
const savingAutoSync = ref(false)
const syncing = ref(false)

async function loadAutoSync() {
  try {
    autoSync.value = await get('/cloudflare/auto-sync/status')
  } catch {
    // monitor endpoint unavailable — keep defaults
  }
}

async function toggleAutoSync(val) {
  try {
    await post('/config', { key: 'dns_auto_sync_enabled', value: String(val) })
    ElMessage.success(val ? 'Auto-sync enabled' : 'Auto-sync disabled')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to toggle auto-sync')
    loadAutoSync()
  }
}

async function saveAutoSync() {
  savingAutoSync.value = true
  try {
    await post('/config', { key: 'dns_auto_sync_zone_id', value: autoSync.value.zoneId })
    await post('/config', { key: 'dns_auto_sync_domain', value: autoSync.value.domain })
    ElMessage.success('Auto-sync config saved')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to save auto-sync config')
  } finally {
    savingAutoSync.value = false
  }
}

async function triggerAutoSync() {
  syncing.value = true
  try {
    await post('/cloudflare/auto-sync/trigger')
    ElMessage.success('Sync triggered')
    setTimeout(loadAutoSync, 3000)
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to trigger sync')
  } finally {
    syncing.value = false
  }
}
</script>

<style scoped>
.cloudflare-page {
  padding: 20px;
}

.autosync-card {
  margin-bottom: 16px;
}

.autosync-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.autosync-row {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  align-items: center;
}

.autosync-last {
  margin-top: 8px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.filter-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  align-items: center;
  flex-wrap: wrap;
}

.record-content {
  word-break: break-all;
}
</style>
