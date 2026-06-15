<template>
  <div class="boot-volumes-page">
    <!-- Page Header -->
    <div class="page-header">
      <h3>Boot Volumes</h3>
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

    <!-- Empty State: no tenant selected -->
    <el-empty v-if="!tenantId" description="Select a tenant to view boot volumes" />

    <!-- Loading State -->
    <el-skeleton v-else-if="loading" :rows="5" animated />

    <!-- Empty State: no volumes -->
    <el-empty v-else-if="bootVolumes.length === 0" description="No boot volumes found" />

    <!-- Table -->
    <template v-else>
      <el-table
        :data="paginatedVolumes"
        stripe
        style="width: 100%"
      >
        <el-table-column prop="displayName" label="Name" min-width="200" />
        <el-table-column label="Size (GB)" width="110" align="center">
          <template #default="{ row }">
            {{ row.sizeInGBs ?? row.sizeInMBs ? Math.round(row.sizeInMBs / 1024) : 'N/A' }}
          </template>
        </el-table-column>
        <el-table-column label="State" width="130" align="center">
          <template #default="{ row }">
            <el-tag
              :type="stateTagType(row.lifecycleState)"
              effect="dark"
              size="small"
            >
              {{ row.lifecycleState || 'unknown' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Instance" min-width="200">
          <template #default="{ row }">
            <span v-if="row._instanceName" class="instance-name">{{ row._instanceName }}</span>
            <span v-else style="color: var(--el-text-color-placeholder); font-size: 13px;">&mdash;</span>
          </template>
        </el-table-column>
        <el-table-column label="Actions" width="220" fixed="right" align="center">
          <template #default="{ row }">
            <el-button type="primary" size="small" @click="openResizeDialog(row)">
              Resize
            </el-button>
            <el-button type="warning" size="small" @click="handleShrink(row)">
              Shrink to 47GB
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- Pagination -->
      <div v-if="bootVolumes.length > 0" class="pagination-wrapper">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="size"
          :total="bootVolumes.length"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next"
          @size-change="onSizeChange"
        />
      </div>
    </template>

    <!-- Resize Dialog -->
    <el-dialog
      v-model="resizeDialogVisible"
      title="Resize Boot Volume"
      width="420px"
      :close-on-click-modal="false"
    >
      <p v-if="selectedVolume" style="margin-bottom: 16px;">
        Resize <strong>{{ selectedVolume.displayName }}</strong>
      </p>
      <el-form label-position="top">
        <el-form-item label="New Size (GB)" required>
          <el-input-number
            v-model="resizeForm.sizeGB"
            :min="47"
            :max="2048"
            controls-position="right"
            style="width: 180px"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="resizeDialogVisible = false">Cancel</el-button>
        <el-button type="primary" :loading="resizing" @click="handleResize">
          Save
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { get, post } from '../api/index.js'

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------
const tenants = ref([])
const tenantId = ref(undefined)
const bootVolumes = ref([])
const allInstances = ref([])
const loading = ref(false)
const resizing = ref(false)
const page = ref(1)
const size = ref(20)

// Resize dialog
const resizeDialogVisible = ref(false)
const selectedVolume = ref(null)
const resizeForm = ref({ sizeGB: 47 })

// ---------------------------------------------------------------------------
// Computed
// ---------------------------------------------------------------------------
const paginatedVolumes = computed(() => {
  const start = (page.value - 1) * size.value
  const end = start + size.value
  return bootVolumes.value.slice(start, end)
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

async function loadBootVolumes() {
  if (!tenantId.value) {
    bootVolumes.value = []
    return
  }
  loading.value = true
  try {
    const res = await get('/boot-volumes', { tenant_id: tenantId.value })
    const volumes = Array.isArray(res) ? res : []
    // Attempt to cross-reference with instances
    bootVolumes.value = attachInstanceNames(volumes, allInstances.value)
    page.value = 1
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to load boot volumes: ' + msg)
    bootVolumes.value = []
  } finally {
    loading.value = false
  }
}

async function loadInstances() {
  if (!tenantId.value) {
    allInstances.value = []
    return
  }
  try {
    const res = await get('/instances', { tenant_id: tenantId.value, size: 200 })
    allInstances.value = res.data || []
  } catch (e) {
    console.error('Failed to load instances:', e)
    allInstances.value = []
  }
}

// ---------------------------------------------------------------------------
// Instance matching (best-effort heuristic)
// ---------------------------------------------------------------------------
function attachInstanceNames(volumes, instances) {
  if (!instances.length) return volumes.map(v => ({ ...v, _instanceName: null }))

  return volumes.map(vol => {
    let match = null

    // Match by availability domain + boot volume size
    if (vol.sizeInGBs && vol.availabilityDomain) {
      match = instances.find(inst =>
        inst.bootVolumeGB &&
        Number(inst.bootVolumeGB) === Number(vol.sizeInGBs) &&
        inst.availabilityDomain &&
        inst.availabilityDomain === vol.availabilityDomain
      )
    }

    // Fallback: match by availability domain only
    if (!match && vol.availabilityDomain) {
      match = instances.find(inst =>
        inst.availabilityDomain &&
        inst.availabilityDomain === vol.availabilityDomain
      )
    }

    return {
      ...vol,
      _instanceName: match ? match.name : null
    }
  })
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------
function handleTenantChange() {
  page.value = 1
  if (tenantId.value) {
    loadInstances().then(() => loadBootVolumes())
  } else {
    bootVolumes.value = []
    allInstances.value = []
  }
}

function onSizeChange() {
  page.value = 1
}

// ---------------------------------------------------------------------------
// State colouring
// ---------------------------------------------------------------------------
function stateTagType(state) {
  switch (state) {
    case 'AVAILABLE':
      return 'success'
    case 'PROVISIONING':
    case 'RESTORING':
      return 'warning'
    case 'TERMINATING':
    case 'TERMINATED':
      return 'info'
    case 'FAULTY':
      return 'danger'
    default:
      return 'info'
  }
}

// ---------------------------------------------------------------------------
// Resize
// ---------------------------------------------------------------------------
function openResizeDialog(volume) {
  selectedVolume.value = volume
  resizeForm.value = {
    sizeGB: volume.sizeInGBs ? Number(volume.sizeInGBs) : 47
  }
  resizeDialogVisible.value = true
}

async function handleResize() {
  if (!selectedVolume.value) return
  const volumeId = selectedVolume.value.id
  if (!volumeId) {
    ElMessage.error('Boot volume ID not found')
    return
  }
  resizing.value = true
  try {
    await post('/boot-volumes/' + volumeId + '/resize', {
      tenantId: tenantId.value,
      sizeInGBs: resizeForm.value.sizeGB
    })
    ElMessage.success('Boot volume resized to ' + resizeForm.value.sizeGB + ' GB')
    resizeDialogVisible.value = false
    await loadBootVolumes()
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Resize failed: ' + msg)
  } finally {
    resizing.value = false
  }
}

// ---------------------------------------------------------------------------
// Shrink (auto-rescue)
// ---------------------------------------------------------------------------
async function handleShrink(volume) {
  const instance = allInstances.value.find(i => i.name === volume._instanceName)
  let instanceId = instance ? instance.id : null

  // If we cannot auto-detect the instance, prompt the user
  if (!instanceId) {
    try {
      await ElMessageBox.alert(
        'No matching instance found for this boot volume. Ensure the instance is synced and try again.',
        'Instance Required',
        { confirmButtonText: 'OK', type: 'info' }
      )
    } catch {
      // dialog dismissed
    }
    return
  }

  try {
    await ElMessageBox.confirm(
      'Shrink boot volume for <strong>' + volume._instanceName + '</strong> to 47 GB? The instance will be stopped during this operation.',
      'Shrink to 47GB',
      {
        confirmButtonText: 'Shrink',
        cancelButtonText: 'Cancel',
        type: 'warning',
        dangerouslyUseHTMLString: true
      }
    )
  } catch {
    return // user cancelled
  }

  // Call auto-rescue
  try {
    const res = await post('/instances/auto-rescue', {
      tenant_id: tenantId.value,
      instance_id: instanceId
    })
    if (res.steps) {
      const lastStep = res.steps[res.steps.length - 1]
      if (res.final_alive) {
        ElMessage.success('Auto-rescue succeeded: alive after ' + lastStep.action)
      } else {
        ElMessage.warning('Auto-rescue completed but still dead. Action: ' + lastStep.action + ' error: ' + (lastStep.error || 'none'))
      }
    } else {
      ElMessage.success('Auto-rescue completed')
    }
    setTimeout(() => loadBootVolumes(), 2000)
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Shrink failed: ' + msg)
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
.boot-volumes-page {
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

.pagination-wrapper {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}

.instance-name {
  font-size: 14px;
}
</style>
