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
        <el-select
          v-model="autoSync.cfgId"
          clearable
          filterable
          placeholder="CF config (default: global token)"
          style="width: 230px"
        >
          <el-option v-for="c in cfConfigs" :key="c.id" :label="c.name" :value="c.id" />
        </el-select>
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

    <!-- Per-instance DNS bindings: ties an instance's public IP to a named
         record in a Cloudflare zone. Auto-sync / change-IP update exactly the
         bound record(s) instead of guessing from the instance name. -->
    <el-card shadow="never" class="autosync-card">
      <template #header>
        <div class="autosync-header">
          <span>Instance ↔ DNS Bindings</span>
          <el-button type="primary" size="small" @click="openAddBindingDialog">
            Add Binding
          </el-button>
        </div>
      </template>
      <div style="margin-bottom:12px;display:flex;gap:8px;align-items:center">
        <el-select
          v-model="bindingInstanceFilter"
          filterable
          clearable
          placeholder="Filter by instance..."
          style="width: 280px"
          @change="onBindingInstanceFilter"
        >
          <el-option
            v-for="inst in instances"
            :key="inst.id"
            :label="inst.name + ' (' + (inst.publicIp || 'no-ip') + ')'"
            :value="inst.id"
          />
        </el-select>
        <el-button @click="loadBindings" :loading="bindingsLoading">Refresh</el-button>
      </div>
      <el-table
        :data="filteredBindings"
        v-loading="bindingsLoading"
        border
        stripe
        size="small"
        style="width: 100%"
      >
        <el-table-column prop="instance" label="Instance" min-width="160">
          <template #default="{ row }">
            {{ instanceName(row.instanceId) }}<span v-if="instanceName(row.instanceId) !== row.instanceId" class="muted"> ({{ row.instanceId }})</span>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="DNS Name" min-width="180" />
        <el-table-column label="Zone" min-width="120">
          <template #default="{ row }">
            {{ zoneName(row.zoneId) }}
          </template>
        </el-table-column>
        <el-table-column label="CF Config" width="120">
          <template #default="{ row }">
            {{ cfCfgName(row.cfCfgId) }}
          </template>
        </el-table-column>
        <el-table-column prop="ttl" label="TTL" width="70" align="center" />
        <el-table-column label="Proxied" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="row.proxied ? 'success' : 'info'" size="small">{{ row.proxied ? 'Yes' : 'No' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Enabled" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'" size="small">{{ row.enabled ? 'On' : 'Off' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Actions" width="130" fixed="right" align="center">
          <template #default="{ row }">
            <el-button type="primary" link size="small" @click="openEditBindingDialog(row)">Edit</el-button>
            <el-button type="danger" link size="small" @click="handleDeleteBinding(row)">Delete</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="No bindings yet. Add one to pin an instance's IP to a DNS name." />
        </template>
      </el-table>
    </el-card>

    <!-- Zone selector and actions -->
    <div class="filter-bar">
      <el-select
        v-model="selectedCfgId"
        clearable
        filterable
        placeholder="Account (default: global token)"
        style="width: 240px"
        @change="onCfgChange"
      >
        <el-option v-for="c in cfConfigs" :key="c.id" :label="c.name" :value="c.id" />
      </el-select>
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

    <!-- Add / Edit Binding Dialog -->
    <el-dialog
      v-model="bindingDialog"
      :title="bindingEditing ? 'Edit DNS Binding' : 'Add DNS Binding'"
      width="560px"
      :close-on-click-modal="false"
    >
      <el-form :model="bindingForm" label-width="110px">
        <el-form-item label="Instance" required>
          <el-select
            v-model="bindingForm.instanceId"
            filterable
            placeholder="Select instance"
            style="width: 100%"
            :disabled="bindingEditing"
          >
            <el-option
              v-for="inst in instances"
              :key="inst.id"
              :label="inst.name + ' · ' + (inst.publicIp || 'no-ip')"
              :value="inst.id"
            >
              <span>{{ inst.name }}</span>&nbsp;
              <span style="float:right;color:#909399;font-size:12px">{{ inst.publicIp || '—' }}</span>
            </el-option>
          </el-select>
        </el-form-item>
        <el-form-item label="DNS Name" required>
          <el-input v-model="bindingForm.name" placeholder="e.g. vps1.example.com (full record name)" />
        </el-form-item>
        <el-form-item label="Zone" required>
          <el-select v-model="bindingForm.zoneId" filterable placeholder="Zone ID (zones of the CF config above)" style="width: 100%" :loading="bindingZonesLoading">
            <el-option v-for="z in bindingZones" :key="z.id" :label="z.name + ' · ' + z.id" :value="z.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="CF Config">
          <el-select v-model="bindingForm.cfCfgId" clearable placeholder="(token fallback)" style="width: 100%">
            <el-option v-for="c in cfConfigs" :key="c.id" :label="c.name" :value="c.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="Proxied">
          <el-switch v-model="bindingForm.proxied" />
        </el-form-item>
        <el-form-item label="TTL">
          <el-input-number v-model="bindingForm.ttl" :min="1" :max="2147483647" :step="60" controls-position="right" />
        </el-form-item>
        <el-form-item label="Enabled">
          <el-switch v-model="bindingForm.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="bindingDialog = false">Cancel</el-button>
        <el-button type="primary" :loading="savingBinding" @click="handleSaveBinding">
          {{ bindingEditing ? 'Update' : 'Create' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, computed, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  listZones, listRecords, createRecord, updateRecord, deleteRecord,
  listBindings, createBinding, updateBinding, deleteBinding,
  listCfConfigs
} from '../api/cloudflare.js'
import { listInstances } from '../api/instances.js'
import { get, post } from '../api/index.js'

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------
const zones = ref([])
const records = ref([])
const selectedZoneId = ref('')
// Which named CF config the zone/record views operate on; null = global token.
const selectedCfgId = ref(null)
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
    zones.value = await listZones(selectedCfgId.value || undefined) || []
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
    records.value = await listRecords(selectedZoneId.value, selectedCfgId.value || undefined) || []
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to load records: ' + msg)
  } finally {
    recordsLoading.value = false
  }
}

function onCfgChange() {
  selectedZoneId.value = ''
  records.value = []
  zones.value = []
  loadZones()
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
      await updateRecord(selectedZoneId.value, editingRecordId.value, payload, selectedCfgId.value || undefined)
      ElMessage.success('Record updated')
    } else {
      await createRecord(selectedZoneId.value, payload, selectedCfgId.value || undefined)
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
    await deleteRecord(selectedZoneId.value, row.id, selectedCfgId.value || undefined)
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
  loadBindings()
  loadInstances()
  loadCfConfigs()
})

// ---------------------------------------------------------------------------
// OCI DNS Auto-Sync
// ---------------------------------------------------------------------------
const autoSync = ref({ enabled: false, zoneId: '', domain: '', cfgId: null, lastSync: '', lastCount: 0 })
const savingAutoSync = ref(false)
const syncing = ref(false)

async function loadAutoSync() {
  try {
    const st = await get('/cloudflare/auto-sync/status')
    autoSync.value = { ...st, cfgId: st && st.cfgId ? Number(st.cfgId) : null }
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
    await post('/config', { key: 'dns_auto_sync_cfg_id', value: autoSync.value.cfgId ? String(autoSync.value.cfgId) : '' })
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

// ---------------------------------------------------------------------------
// Instance ↔ DNS Bindings
// ---------------------------------------------------------------------------
const bindings = ref([])
const instances = ref([])
const cfConfigs = ref([])
const bindingsLoading = ref(false)
const bindingInstanceFilter = ref('')
const bindingDialog = ref(false)
const bindingEditing = ref(false)
const editingBindingId = ref('')
const savingBinding = ref(false)
// Zones offered in the binding dialog: they follow the binding's own CF
// config (null = global token), not the record-view account selector.
const bindingZones = ref([])
const bindingZonesLoading = ref(false)
const bindingForm = reactive({
  instanceId: '',
  name: '',
  zoneId: '',
  cfCfgId: null,
  proxied: false,
  ttl: 120,
  enabled: true
})

const filteredBindings = computed(() => {
  if (!bindingInstanceFilter.value) return bindings.value
  return bindings.value.filter(b => b.instanceId === bindingInstanceFilter.value)
})

function instanceName(id) {
  const inst = instances.value.find(i => i.id === id)
  return inst ? (inst.name || id) : id
}

function zoneName(id) {
  const all = zones.value.concat(bindingZones.value)
  const z = all.find(z => z.id === id)
  return z ? z.name : (id || '—')
}

function cfCfgName(id) {
  const c = cfConfigs.value.find(c => c.id === id)
  return c ? c.name : 'default token'
}

async function loadBindingZones() {
  bindingZonesLoading.value = true
  try {
    bindingZones.value = await listZones(bindingForm.cfCfgId || undefined) || []
  } catch {
    bindingZones.value = []
  } finally {
    bindingZonesLoading.value = false
  }
}

// Reload the zone picker whenever the binding's CF config changes.
watch(() => bindingForm.cfCfgId, () => {
  bindingForm.zoneId = ''
  loadBindingZones()
})

async function loadBindings() {
  bindingsLoading.value = true
  try {
    const res = await listBindings()
    bindings.value = res?.data || []
  } catch (e) {
    ElMessage.error('Failed to load bindings: ' + (e.response?.data?.error || e.message))
  } finally {
    bindingsLoading.value = false
  }
}

async function loadInstances() {
  try {
    const res = await listInstances({ size: 500 })
    instances.value = res?.data || []
  } catch {
    // list may fail if no OCI configured; keep empty
  }
}

async function loadCfConfigs() {
  try {
    const res = await listCfConfigs()
    cfConfigs.value = res?.data || []
  } catch {
    cfConfigs.value = []
  }
}

function onBindingInstanceFilter() {
  // no-op; template uses computed filter
}

function resetBindingForm() {
  bindingForm.instanceId = ''
  bindingForm.name = ''
  bindingForm.zoneId = ''
  bindingForm.cfCfgId = null
  bindingForm.proxied = false
  bindingForm.ttl = 120
  bindingForm.enabled = true
}

function openAddBindingDialog() {
  bindingEditing.value = false
  editingBindingId.value = ''
  resetBindingForm()
  bindingDialog.value = true
  loadBindingZones()
}

function openEditBindingDialog(row) {
  bindingEditing.value = true
  editingBindingId.value = row.id
  bindingForm.instanceId = row.instanceId
  bindingForm.name = row.name
  bindingForm.zoneId = row.zoneId
  bindingForm.cfCfgId = row.cfCfgId || null
  bindingForm.proxied = !!row.proxied
  bindingForm.ttl = row.ttl || 120
  bindingForm.enabled = row.enabled !== false
  bindingDialog.value = true
  loadBindingZones()
}

async function handleSaveBinding() {
  if (!bindingForm.instanceId || !bindingForm.name || !bindingForm.zoneId) {
    ElMessage.warning('Instance, DNS Name and Zone are required')
    return
  }
  savingBinding.value = true
  try {
    const payload = {
      instanceId: bindingForm.instanceId,
      tenantId: instances.value.find(i => i.id === bindingForm.instanceId)?.tenantId || 0,
      name: bindingForm.name,
      zoneId: bindingForm.zoneId,
      cfCfgId: bindingForm.cfCfgId || 0,
      proxied: bindingForm.proxied,
      ttl: bindingForm.ttl || 120,
      enabled: bindingForm.enabled
    }
    if (bindingEditing.value) {
      await updateBinding(editingBindingId.value, payload)
      ElMessage.success('Binding updated')
    } else {
      await createBinding(payload)
      ElMessage.success('Binding created')
    }
    bindingDialog.value = false
    await loadBindings()
  } catch (e) {
    ElMessage.error('Save binding failed: ' + (e.response?.data?.error || e.message))
  } finally {
    savingBinding.value = false
  }
}

async function handleDeleteBinding(row) {
  try {
    await ElMessageBox.confirm(
      `Delete binding "${row.name}" for ${instanceName(row.instanceId)}?`,
      'Confirm Delete',
      {
        confirmButtonText: 'Delete',
        cancelButtonText: 'Cancel',
        type: 'warning'
      }
    )
    await deleteBinding(row.id)
    ElMessage.success('Binding deleted')
    await loadBindings()
  } catch {
    // cancelled or error
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

.muted {
  color: #909399;
  font-size: 12px;
}
</style>
