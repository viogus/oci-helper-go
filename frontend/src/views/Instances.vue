<template>
  <div class="instances-page">
    <!-- Filter Bar -->
    <div class="filter-bar">
      <el-select
        v-model="tenantId"
        placeholder="All tenants"
        clearable
        @change="handleSearch"
        style="width: 200px"
      >
        <el-option
          v-for="t in tenants"
          :key="t.id"
          :label="t.name"
          :value="t.id"
        />
      </el-select>
      <el-input
        v-model="keyword"
        placeholder="Search by name, IP, or OCID..."
        clearable
        @input="handleSearch"
        style="width: 320px"
      />
    </div>

    <!-- Batch Action Bar -->
    <div v-if="selectedRows.length > 0" class="batch-bar">
      <span class="batch-info">{{ selectedRows.length }} instance(s) selected</span>
      <el-button type="primary" size="small" @click="handleBatchStart">
        Batch Start
      </el-button>
      <el-button type="danger" size="small" @click="handleBatchTerminate">
        Batch Terminate
      </el-button>
      <el-button size="small" @click="handleCheckAlive">
        Check Alive
      </el-button>
    </div>

    <!-- Instances Table -->
    <el-table
      :data="instances"
      v-loading="loading"
      @selection-change="onSelectionChange"
      border
      stripe
      style="width: 100%"
      row-key="id"
      element-loading-text="Loading instances..."
    >
      <el-table-column type="selection" width="50" />
      <el-table-column label="Name" min-width="200">
        <template #default="{ row }">
          <el-button type="primary" link @click="openMetrics(row)">
            {{ row.name }}
          </el-button>
        </template>
      </el-table-column>
      <el-table-column prop="shape" label="Shape" width="150" />
      <el-table-column prop="ocpu" label="OCPU" width="80" align="center" />
      <el-table-column prop="memoryGB" label="Memory (GB)" width="110" align="center" />
      <el-table-column label="State" width="130" align="center">
        <template #default="{ row }">
          <el-tag :type="stateTagType(row.state)" effect="dark" size="small">
            {{ row.state }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="publicIp" label="Public IP" width="150" />
      <el-table-column prop="tenantId" label="Tenant ID" width="90" align="center" />
      <el-table-column label="Actions" width="140" fixed="right" align="center">
        <template #default="{ row }">
          <el-dropdown
            trigger="click"
            @command="(cmd) => handleDropdownAction(row, cmd)"
          >
            <el-button type="primary" size="small">
              Actions
              <el-icon><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <!-- STOPPED → Start -->
                <el-dropdown-item
                  v-if="row.state === 'STOPPED'"
                  command="start"
                >
                  Start
                </el-dropdown-item>

                <!-- RUNNING → Stop / Reboot / Soft Stop / Soft Reset -->
                <template v-if="row.state === 'RUNNING'">
                  <el-dropdown-item command="stop">
                    Stop
                  </el-dropdown-item>
                  <el-dropdown-item command="reboot">
                    Reboot
                  </el-dropdown-item>
                  <el-dropdown-item command="softstop">
                    Soft Stop
                  </el-dropdown-item>
                  <el-dropdown-item command="softreset">
                    Soft Reset
                  </el-dropdown-item>
                </template>

                <el-dropdown-item command="terminate" divided>
                  Terminate
                </el-dropdown-item>

                <el-dropdown-item command="changeShape" divided>
                  Change Shape
                </el-dropdown-item>
                <el-dropdown-item command="changeBootVolume">
                  Change Boot Volume
                </el-dropdown-item>
                <el-dropdown-item command="attachIPv6">
                  Attach IPv6
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </template>
      </el-table-column>
    </el-table>

    <!-- Empty State Override -->
    <el-empty v-if="!loading && instances.length === 0" description="No instances found" />

    <!-- Pagination -->
    <div class="pagination-wrapper">
      <el-pagination
        v-model:current-page="page"
        v-model:page-size="size"
        :total="total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next"
        @size-change="onSizeChange"
        @current-change="loadInstances"
      />
    </div>

    <!-- Change Shape Dialog -->
    <el-dialog v-model="shapeDialogVisible" title="Change Shape" width="420px" :close-on-click-modal="false">
      <el-form :model="shapeForm" label-width="120px">
        <el-form-item label="Shape" required>
          <el-input v-model="shapeForm.shape" placeholder="e.g. VM.Standard.E3.Flex" />
        </el-form-item>
        <el-form-item label="OCPUs" required>
          <el-input-number
            v-model="shapeForm.ocpus"
            :min="1"
            :max="128"
            controls-position="right"
            style="width: 180px"
          />
        </el-form-item>
        <el-form-item label="Memory (GB)" required>
          <el-input-number
            v-model="shapeForm.memoryGB"
            :min="1"
            :max="2048"
            controls-position="right"
            style="width: 180px"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="shapeDialogVisible = false">Cancel</el-button>
        <el-button type="primary" :loading="saving" @click="handleChangeShape">
          Save
        </el-button>
      </template>
    </el-dialog>

    <!-- Change Boot Volume Dialog -->
    <el-dialog v-model="volumeDialogVisible" title="Change Boot Volume" width="420px" :close-on-click-modal="false">
      <p style="margin-bottom: 16px;">
        Resize boot volume for <strong>{{ currentInstance?.name }}</strong>
      </p>
      <el-form :model="volumeForm" label-width="120px">
        <el-form-item label="Size (GB)" required>
          <el-input-number
            v-model="volumeForm.sizeGB"
            :min="50"
            :max="2048"
            controls-position="right"
            style="width: 180px"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="volumeDialogVisible = false">Cancel</el-button>
        <el-button type="primary" :loading="saving" @click="handleChangeBootVolume">
          Save
        </el-button>
      </template>
    </el-dialog>

    <!-- Attach IPv6 Dialog -->
    <el-dialog v-model="attachIPv6Visible" title="Attach IPv6" width="420px" :close-on-click-modal="false">
      <p>
        Attach an IPv6 address to <strong>{{ currentInstance?.name }}</strong>?
      </p>
      <template #footer>
        <el-button @click="attachIPv6Visible = false">Cancel</el-button>
        <el-button type="primary" :loading="saving" @click="handleAttachIPv6">
          Confirm
        </el-button>
      </template>
    </el-dialog>

    <!-- Metrics Dialog (placeholder — Phase 3) -->
    <el-dialog v-model="metricsVisible" title="Instance Metrics" width="640px">
      <p v-if="currentInstance">
        Metrics for <strong>{{ currentInstance.name }}</strong>
        <span style="color: #909399; margin-left: 8px;">({{ currentInstance.shape }})</span>
      </p>
      <el-alert
        title="Metrics visualization will be available in Phase 3"
        type="info"
        :closable="false"
        show-icon
        style="margin-top: 16px"
      />
      <template #footer>
        <el-button @click="metricsVisible = false">Close</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown } from '@element-plus/icons-vue'
import {
  listInstances,
  instanceAction,
  batchStart,
  changeShape,
  changeBootVolume,
  attachIPv6
} from '../api/instances.js'
import { get } from '../api/index.js'

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------
const instances = ref([])
const total = ref(0)
const page = ref(1)
const size = ref(20)
const keyword = ref('')
const tenantId = ref(undefined)
const selectedRows = ref([])
const tenants = ref([])
const loading = ref(false)
const saving = ref(false)

// Dialog visibility & forms
const shapeDialogVisible = ref(false)
const volumeDialogVisible = ref(false)
const attachIPv6Visible = ref(false)
const metricsVisible = ref(false)
const currentInstance = ref(null)

const shapeForm = reactive({
  shape: '',
  ocpus: 1,
  memoryGB: 1
})

const volumeForm = reactive({
  sizeGB: 50
})

// ---------------------------------------------------------------------------
// Debounced search
// ---------------------------------------------------------------------------
let searchTimer = null

function handleSearch() {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    loadInstances()
  }, 300)
}

// ---------------------------------------------------------------------------
// Data loading
// ---------------------------------------------------------------------------
async function loadTenants() {
  try {
    const res = await get('/tenants')
    tenants.value = res.data || []
  } catch (e) {
    console.error('Failed to load tenants:', e)
  }
}

async function loadInstances() {
  loading.value = true
  try {
    const params = {
      page: page.value,
      size: size.value
    }
    if (keyword.value) {
      params.keyword = keyword.value
    }
    if (tenantId.value) {
      params.tenant_id = tenantId.value
    }
    const res = await listInstances(params)
    instances.value = res.data || []
    total.value = res.total || 0
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to load instances: ' + msg)
  } finally {
    loading.value = false
  }
}

// ---------------------------------------------------------------------------
// Pagination
// ---------------------------------------------------------------------------
function onSizeChange() {
  page.value = 1
  loadInstances()
}

// ---------------------------------------------------------------------------
// Selection
// ---------------------------------------------------------------------------
function onSelectionChange(rows) {
  selectedRows.value = rows
}

// ---------------------------------------------------------------------------
// State colouring
// ---------------------------------------------------------------------------
function stateTagType(state) {
  switch (state) {
    case 'RUNNING':
      return 'success'
    case 'STOPPED':
      return 'danger'
    case 'TERMINATED':
      return 'info'
    case 'STARTING':
    case 'STOPPING':
    case 'TERMINATING':
      return 'warning'
    default:
      return 'info'
  }
}

// ---------------------------------------------------------------------------
// Row actions (dropdown)
// ---------------------------------------------------------------------------
function handleDropdownAction(row, command) {
  currentInstance.value = row
  switch (command) {
    case 'start':
    case 'stop':
    case 'reboot':
    case 'softstop':
    case 'softreset':
      handleAction(row, command)
      break
    case 'terminate':
      handleTerminate(row)
      break
    case 'changeShape':
      shapeForm.shape = row.shape || ''
      shapeForm.ocpus = row.ocpu || 1
      shapeForm.memoryGB = row.memoryGB || 1
      shapeDialogVisible.value = true
      break
    case 'changeBootVolume':
      volumeForm.sizeGB = row.bootVolumeGB || 50
      volumeDialogVisible.value = true
      break
    case 'attachIPv6':
      attachIPv6Visible.value = true
      break
  }
}

async function handleAction(instance, action) {
  try {
    await instanceAction(instance.id, action)
    ElMessage.success(`Action "${action}" sent to ${instance.name}`)
    await loadInstances()
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error(`Action "${action}" failed: ${msg}`)
  }
}

async function handleTerminate(instance) {
  try {
    await ElMessageBox.confirm(
      `Are you sure you want to terminate "${instance.name}"?\n\nThis action cannot be undone.`,
      'Confirm Terminate',
      {
        confirmButtonText: 'Terminate',
        cancelButtonText: 'Cancel',
        type: 'warning'
      }
    )
    await handleAction(instance, 'terminate')
  } catch {
    // User cancelled
  }
}

// ---------------------------------------------------------------------------
// Dialogs
// ---------------------------------------------------------------------------
async function handleChangeShape() {
  saving.value = true
  try {
    await changeShape({
      tenant_id: currentInstance.value.tenantId,
      instance_id: currentInstance.value.id,
      shape: shapeForm.shape,
      ocpus: shapeForm.ocpus,
      memory_gb: shapeForm.memoryGB
    })
    ElMessage.success('Shape change request submitted')
    shapeDialogVisible.value = false
    await loadInstances()
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Change shape failed: ' + msg)
  } finally {
    saving.value = false
  }
}

async function handleChangeBootVolume() {
  saving.value = true
  try {
    await changeBootVolume({
      tenant_id: currentInstance.value.tenantId,
      instance_id: currentInstance.value.id,
      size_gb: volumeForm.sizeGB
    })
    ElMessage.success('Boot volume change request submitted')
    volumeDialogVisible.value = false
    await loadInstances()
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Change boot volume failed: ' + msg)
  } finally {
    saving.value = false
  }
}

async function handleAttachIPv6() {
  saving.value = true
  try {
    await attachIPv6({
      tenant_id: currentInstance.value.tenantId,
      instance_id: currentInstance.value.id
    })
    ElMessage.success('IPv6 attachment request submitted')
    attachIPv6Visible.value = false
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Attach IPv6 failed: ' + msg)
  } finally {
    saving.value = false
  }
}

// ---------------------------------------------------------------------------
// Batch actions
// ---------------------------------------------------------------------------
async function handleBatchStart() {
  if (selectedRows.value.length === 0) return
  const ids = selectedRows.value.map((r) => r.id)
  const tid = selectedRows.value[0].tenantId
  try {
    await batchStart({
      tenantId: tid,
      instanceIds: ids
    })
    ElMessage.success(`Batch start requested for ${ids.length} instance(s)`)
    selectedRows.value = []
    await loadInstances()
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Batch start failed: ' + msg)
  }
}

async function handleBatchTerminate() {
  if (selectedRows.value.length === 0) return
  try {
    await ElMessageBox.confirm(
      `Are you sure you want to terminate ${selectedRows.value.length} instance(s)?`,
      'Confirm Batch Terminate',
      {
        confirmButtonText: 'Terminate All',
        cancelButtonText: 'Cancel',
        type: 'warning'
      }
    )
    for (const inst of selectedRows.value) {
      try {
        await instanceAction(inst.id, 'terminate')
      } catch (e) {
        console.error(`Failed to terminate ${inst.name}:`, e)
      }
    }
    ElMessage.success(`Termination requested for ${selectedRows.value.length} instance(s)`)
    selectedRows.value = []
    await loadInstances()
  } catch {
    // User cancelled
  }
}

async function handleCheckAlive() {
  ElMessage.info('Check Alive — feature coming soon')
}

// ---------------------------------------------------------------------------
// Metrics (placeholder)
// ---------------------------------------------------------------------------
function openMetrics(instance) {
  currentInstance.value = instance
  metricsVisible.value = true
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------
onMounted(() => {
  loadTenants()
  loadInstances()
})
</script>

<style scoped>
.instances-page {
  padding: 20px;
}

/* ── Filter bar ───────────────────────────────────────────────────────── */
.filter-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  align-items: center;
}

/* ── Batch action bar ─────────────────────────────────────────────────── */
.batch-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 16px;
  margin-bottom: 12px;
  background: #e6f7ff;
  border: 1px solid #91d5ff;
  border-radius: 6px;
}

.batch-info {
  font-weight: 600;
  color: #1890ff;
  margin-right: 12px;
}

.dark .batch-bar {
  background: #1a2744;
  border-color: #15395b;
}

.dark .batch-info {
  color: #69b1ff;
}

/* ── Pagination ───────────────────────────────────────────────────────── */
.pagination-wrapper {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
  padding: 8px 0;
}
</style>
