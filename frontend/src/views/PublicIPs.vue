<template>
  <div class="public-ips-page">
    <!-- Page Header -->
    <div class="page-header">
      <h3>Public IPs</h3>
      <el-button type="primary" :disabled="!tenantId" @click="openReserveDialog">
        <el-icon><Plus /></el-icon> Reserve New
      </el-button>
    </div>

    <!-- Tenant Filter -->
    <div class="filter-bar">
      <el-select
        v-model="tenantId"
        placeholder="Select tenant"
        clearable
        @change="handleTenantChange"
        style="width: 240px"
      >
        <el-option
          v-for="t in tenants"
          :key="t.id"
          :label="t.name"
          :value="t.id"
        />
      </el-select>
    </div>

    <!-- Replace IP Section -->
    <el-card v-if="tenantId" shadow="never" class="replace-ip-card">
      <template #header>
        <span style="font-weight: 600">Replace Instance IP</span>
      </template>
      <div class="replace-ip-row">
        <el-select
          v-model="replaceForm.instanceId"
          placeholder="Select instance"
          filterable
          style="width: 320px"
        >
          <el-option
            v-for="inst in instances"
            :key="inst.id"
            :label="`${inst.name} (${inst.publicIp || 'no IP'})`"
            :value="inst.id"
          />
        </el-select>
        <el-input
          v-model="replaceForm.cidrList"
          placeholder="CIDR filter (optional, comma-separated)"
          clearable
          style="width: 300px"
        />
        <el-button
          type="primary"
          :loading="replacing"
          :disabled="!replaceForm.instanceId"
          @click="handleReplaceIP"
        >
          Replace IP
        </el-button>
      </div>
    </el-card>

    <!-- Empty State: no tenant selected -->
    <el-empty v-if="!tenantId" description="Select a tenant to view public IPs" />

    <!-- Loading State -->
    <el-skeleton v-else-if="loading" :rows="5" animated />

    <!-- Empty State: no IPs -->
    <el-empty v-else-if="publicIPs.length === 0" description="No public IPs found" />

    <!-- Table -->
    <template v-else>
      <el-table
        :data="paginatedIPs"
        stripe
        style="width: 100%"
      >
        <el-table-column prop="displayName" label="Name" min-width="180" />
        <el-table-column label="IP Address" width="180">
          <template #default="{ row }">
            <code>{{ row.ipAddress || 'N/A' }}</code>
          </template>
        </el-table-column>
        <el-table-column label="Scope" width="110">
          <template #default="{ row }">
            <el-tag :type="row.scope === 'REGION' ? 'primary' : 'warning'" size="small">
              {{ row.scope || 'N/A' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="State" width="110">
          <template #default="{ row }">
            <el-tag
              :type="row.lifecycleState === 'ASSIGNED' ? 'success' : 'info'"
              effect="dark"
              size="small"
            >
              {{ row.lifecycleState || 'unknown' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Actions" width="120" fixed="right" align="center">
          <template #default="{ row }">
            <el-button
              type="danger"
              size="small"
              :loading="deletingId === row.id"
              @click="handleDelete(row)"
            >
              Delete
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- Pagination -->
      <div v-if="publicIPs.length > 0" class="pagination-wrapper">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="size"
          :total="publicIPs.length"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next"
          @size-change="onSizeChange"
        />
      </div>
    </template>

    <!-- Reserve New Dialog -->
    <el-dialog
      v-model="reserveDialogVisible"
      title="Reserve New Public IP"
      width="420px"
      :close-on-click-modal="false"
      @closed="reserveForm.displayName = ''"
    >
      <el-form label-position="top">
        <el-form-item label="Display Name" required>
          <el-input v-model="reserveForm.displayName" placeholder="e.g. my-reserved-ip" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="reserveDialogVisible = false">Cancel</el-button>
        <el-button type="primary" :loading="reserving" @click="handleReserve">
          Reserve
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { get, post, del } from '../api/index.js'

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------
const tenants = ref([])
const tenantId = ref(undefined)
const publicIPs = ref([])
const instances = ref([])
const loading = ref(false)
const reserving = ref(false)
const replacing = ref(false)
const deletingId = ref(null)
const page = ref(1)
const size = ref(20)

const reserveDialogVisible = ref(false)
const reserveForm = ref({ displayName: '' })

const replaceForm = ref({ instanceId: '', cidrList: '' })

// ---------------------------------------------------------------------------
// Computed: client-side pagination
// ---------------------------------------------------------------------------
const paginatedIPs = computed(() => {
  const start = (page.value - 1) * size.value
  const end = start + size.value
  return publicIPs.value.slice(start, end)
})

// ---------------------------------------------------------------------------
// Data loading
// ---------------------------------------------------------------------------
async function loadTenants() {
  try {
    const res = await get('/tenants')
    tenants.value = res.data || []
  } catch (e) {
    ElMessage.error('Failed to load tenants')
  }
}

async function loadPublicIPs() {
  if (!tenantId.value) {
    publicIPs.value = []
    return
  }
  loading.value = true
  try {
    const res = await get('/public-ips', { tenant_id: tenantId.value })
    publicIPs.value = Array.isArray(res) ? res : []
    page.value = 1
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to load public IPs: ' + msg)
    publicIPs.value = []
  } finally {
    loading.value = false
  }
}

async function loadInstances() {
  if (!tenantId.value) {
    instances.value = []
    return
  }
  try {
    const res = await get('/instances', { tenant_id: tenantId.value, size: 200 })
    instances.value = res.data || []
  } catch (e) {
    console.error('Failed to load instances:', e)
    instances.value = []
  }
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------
function handleTenantChange() {
  page.value = 1
  if (tenantId.value) {
    loadPublicIPs()
    loadInstances()
  } else {
    publicIPs.value = []
    instances.value = []
  }
}

function onSizeChange() {
  page.value = 1
}

// ---------------------------------------------------------------------------
// Reserve New
// ---------------------------------------------------------------------------
function openReserveDialog() {
  reserveForm.value = { displayName: '' }
  reserveDialogVisible.value = true
}

async function handleReserve() {
  if (!reserveForm.value.displayName.trim()) {
    ElMessage.warning('Display name is required')
    return
  }
  reserving.value = true
  try {
    await post('/public-ips', {
      tenantId: tenantId.value,
      displayName: reserveForm.value.displayName.trim()
    })
    ElMessage.success('Public IP reserved')
    reserveDialogVisible.value = false
    await loadPublicIPs()
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to reserve IP: ' + msg)
  } finally {
    reserving.value = false
  }
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------
async function handleDelete(ip) {
  try {
    await ElMessageBox.confirm(
      `Delete public IP "${ip.displayName || ip.ipAddress}"? This action cannot be undone.`,
      'Confirm Delete',
      {
        confirmButtonText: 'Delete',
        cancelButtonText: 'Cancel',
        type: 'warning'
      }
    )
    deletingId.value = ip.id
    await del('/public-ips/' + ip.id + '?tenant_id=' + tenantId.value)
    ElMessage.success('Public IP deleted')
    await loadPublicIPs()
  } catch (err) {
    if (err && err !== 'cancel') {
      const msg = err.response?.data?.error || err.message
      ElMessage.error('Delete failed: ' + msg)
    }
  } finally {
    deletingId.value = null
  }
}

// ---------------------------------------------------------------------------
// Replace IP
// ---------------------------------------------------------------------------
async function handleReplaceIP() {
  if (!replaceForm.value.instanceId) {
    ElMessage.warning('Please select an instance')
    return
  }
  replacing.value = true
  try {
    const payload = {
      tenant_id: tenantId.value,
      instance_id: replaceForm.value.instanceId
    }
    const cidrStr = replaceForm.value.cidrList.trim()
    if (cidrStr) {
      payload.cidr_list = cidrStr.split(',').map(s => s.trim()).filter(Boolean)
    }
    const res = await post('/instances/change-ip', payload)
    ElMessage.success('IP replaced' + (res.new_ip ? ' -> ' + res.new_ip : ''))
    replaceForm.value.instanceId = ''
    replaceForm.value.cidrList = ''
    await loadInstances()
    await loadPublicIPs()
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to replace IP: ' + msg)
  } finally {
    replacing.value = false
  }
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------
onMounted(() => {
  loadTenants()
})
</script>

<style scoped>
.public-ips-page {
  padding: 0;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.page-header h3 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
}

.filter-bar {
  margin-bottom: 16px;
}

.replace-ip-card {
  margin-bottom: 16px;
}

.replace-ip-card :deep(.el-card__body) {
  padding: 12px 20px;
}

.replace-ip-row {
  display: flex;
  gap: 12px;
  align-items: center;
}

.pagination-wrapper {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}

code {
  background: var(--el-fill-color-light);
  padding: 2px 6px;
  border-radius: 3px;
  font-size: 13px;
  color: var(--el-color-primary);
}
</style>
