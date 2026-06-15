<template>
  <div class="in-memory-tasks-page">
    <h3>In-Memory Tasks</h3>

    <el-tabs v-model="activeTab" @tab-change="onTabChange">
      <!-- ── Change IP Tasks Tab ─────────────────────────────────── -->
      <el-tab-pane label="Change IP Tasks" name="change-ip">
        <div class="toolbar">
          <el-button type="primary" :icon="Plus" @click="openAddDialog">
            Add Task
          </el-button>
          <el-button :icon="Refresh" :loading="loading" @click="loadTasks">
            Refresh
          </el-button>
        </div>

        <el-table
          :data="changeIPTasks"
          v-loading="loading && activeTab === 'change-ip'"
          border
          stripe
          style="width: 100%"
          row-key="id"
          element-loading-text="Loading tasks..."
        >
          <el-table-column label="ID" width="100" align="center">
            <template #default="{ row }">
              {{ row.id ? row.id.slice(0, 8) : '-' }}
            </template>
          </el-table-column>
          <el-table-column label="Username" min-width="140">
            <template #default="{ row }">
              {{ row.username || '-' }}
            </template>
          </el-table-column>
          <el-table-column label="Region" width="150">
            <template #default="{ row }">
              {{ row.region || '-' }}
            </template>
          </el-table-column>
          <el-table-column label="Instance ID" min-width="200">
            <template #default="{ row }">
              <el-tooltip :content="row.instance_id || ''" placement="top" :show-after="300">
                <span class="mono-text">{{ row.instance_id ? row.instance_id.slice(0, 24) + '...' : '-' }}</span>
              </el-tooltip>
            </template>
          </el-table-column>
          <el-table-column label="CIDR List" min-width="160">
            <template #default="{ row }">
              <span class="mono-text">{{ row.cidr_list || '-' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="Attempts" width="90" align="center">
            <template #default="{ row }">
              <el-tag size="small" effect="plain">{{ row.attempts ?? 0 }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="Status" width="110" align="center">
            <template #default="{ row }">
              <el-tag
                :type="row.paused ? 'warning' : 'success'"
                effect="dark"
                size="small"
              >
                {{ row.paused ? 'Paused' : 'Running' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="Created At" width="170" align="center">
            <template #default="{ row }">
              {{ formatDate(row.created_at) }}
            </template>
          </el-table-column>
          <el-table-column label="Actions" width="200" fixed="right" align="center">
            <template #default="{ row }">
              <el-button
                v-if="!row.paused"
                type="warning"
                size="small"
                @click="handleAction([row.id], 'pause', 'change-ip')"
              >
                Pause
              </el-button>
              <el-button
                v-if="row.paused"
                type="primary"
                size="small"
                @click="handleAction([row.id], 'resume', 'change-ip')"
              >
                Resume
              </el-button>
              <el-button
                type="danger"
                size="small"
                @click="handleAction([row.id], 'delete', 'change-ip')"
              >
                Delete
              </el-button>
            </template>
          </el-table-column>
        </el-table>

        <el-empty v-if="!loading && changeIPTasks.length === 0" description="No active tasks" />
      </el-tab-pane>

      <!-- ── Update Config Tasks Tab ─────────────────────────────── -->
      <el-tab-pane label="Update Config Tasks" name="update-cfg">
        <div class="toolbar">
          <el-button type="primary" :icon="Plus" @click="openAddDialog">
            Add Task
          </el-button>
          <el-button :icon="Refresh" :loading="loading" @click="loadTasks">
            Refresh
          </el-button>
        </div>

        <el-table
          :data="updateCfgTasks"
          v-loading="loading && activeTab === 'update-cfg'"
          border
          stripe
          style="width: 100%"
          row-key="id"
          element-loading-text="Loading tasks..."
        >
          <el-table-column label="ID" width="100" align="center">
            <template #default="{ row }">
              {{ row.id ? row.id.slice(0, 8) : '-' }}
            </template>
          </el-table-column>
          <el-table-column label="Username" min-width="140">
            <template #default="{ row }">
              {{ row.username || '-' }}
            </template>
          </el-table-column>
          <el-table-column label="Region" width="150">
            <template #default="{ row }">
              {{ row.region || '-' }}
            </template>
          </el-table-column>
          <el-table-column label="Instance ID" min-width="200">
            <template #default="{ row }">
              <el-tooltip :content="row.instance_id || ''" placement="top" :show-after="300">
                <span class="mono-text">{{ row.instance_id ? row.instance_id.slice(0, 24) + '...' : '-' }}</span>
              </el-tooltip>
            </template>
          </el-table-column>
          <el-table-column label="Config" min-width="170">
            <template #default="{ row }">
              <span v-if="row.ocpus || row.memory || row.shape" class="mono-text">
                {{ row.ocpus ?? '-' }} OCPU / {{ row.memory ?? '-' }} GB{{ row.shape ? ' / ' + row.shape : '' }}
              </span>
              <span v-else>-</span>
            </template>
          </el-table-column>
          <el-table-column label="Attempts" width="90" align="center">
            <template #default="{ row }">
              <el-tag size="small" effect="plain">{{ row.attempts ?? 0 }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="Status" width="110" align="center">
            <template #default="{ row }">
              <el-tag
                :type="row.paused ? 'warning' : 'success'"
                effect="dark"
                size="small"
              >
                {{ row.paused ? 'Paused' : 'Running' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="Created At" width="170" align="center">
            <template #default="{ row }">
              {{ formatDate(row.created_at) }}
            </template>
          </el-table-column>
          <el-table-column label="Actions" width="200" fixed="right" align="center">
            <template #default="{ row }">
              <el-button
                v-if="!row.paused"
                type="warning"
                size="small"
                @click="handleAction([row.id], 'pause', 'update-cfg')"
              >
                Pause
              </el-button>
              <el-button
                v-if="row.paused"
                type="primary"
                size="small"
                @click="handleAction([row.id], 'resume', 'update-cfg')"
              >
                Resume
              </el-button>
              <el-button
                type="danger"
                size="small"
                @click="handleAction([row.id], 'delete', 'update-cfg')"
              >
                Delete
              </el-button>
            </template>
          </el-table-column>
        </el-table>

        <el-empty v-if="!loading && updateCfgTasks.length === 0" description="No active tasks" />
      </el-tab-pane>
    </el-tabs>

    <!-- ── Add Task Dialog ────────────────────────────────────────── -->
    <el-dialog
      v-model="dialogVisible"
      :title="activeTab === 'change-ip' ? 'Add Change IP Task' : 'Add Update Config Task'"
      width="550px"
      :close-on-click-modal="false"
      @closed="onDialogClosed"
    >
      <el-form label-position="top">
        <!-- Tenant selector -->
        <el-form-item label="Tenant">
          <el-select
            v-model="form.tenant_id"
            placeholder="Select tenant"
            style="width: 100%"
            @change="onTenantChange"
          >
            <el-option
              v-for="t in tenants"
              :key="t.id"
              :label="t.name"
              :value="t.id"
            />
          </el-select>
        </el-form-item>

        <!-- Instance selector -->
        <el-form-item label="Instance">
          <el-select
            v-model="form.instance_id"
            placeholder="Select instance"
            filterable
            :disabled="!form.tenant_id"
            :loading="loadingInstances"
            style="width: 100%"
          >
            <el-option
              v-for="inst in instances"
              :key="inst.id"
              :label="inst.name || inst.id"
              :value="inst.id"
            />
          </el-select>
        </el-form-item>

        <!-- Change IP fields -->
        <template v-if="activeTab === 'change-ip'">
          <el-form-item label="CIDR List (optional)">
            <el-input
              v-model="form.cidr_list"
              placeholder="e.g. 10.0.0.0/8, 192.168.0.0/16"
              style="width: 100%"
            />
          </el-form-item>
        </template>

        <!-- Update Config fields -->
        <template v-if="activeTab === 'update-cfg'">
          <el-form-item label="OCPU">
            <el-input-number
              v-model="form.ocpus"
              :min="0"
              :precision="0"
              controls-position="right"
              style="width: 180px"
            />
          </el-form-item>
          <el-form-item label="Memory (GB)">
            <el-input-number
              v-model="form.memory"
              :min="0"
              :precision="1"
              :step="0.5"
              controls-position="right"
              style="width: 180px"
            />
          </el-form-item>
          <el-form-item label="Shape (optional)">
            <el-input
              v-model="form.shape"
              placeholder="e.g. VM.Standard.E5.Flex"
              style="width: 100%"
            />
          </el-form-item>
        </template>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">Cancel</el-button>
        <el-button type="primary" :loading="addLoading" @click="handleAdd">
          Add
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Refresh } from '@element-plus/icons-vue'
import { get, post } from '../api/index.js'
import { listTenants } from '../api/tenants.js'

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------
const activeTab = ref('change-ip')
const changeIPTasks = ref([])
const updateCfgTasks = ref([])
const loading = ref(false)
const addLoading = ref(false)
const dialogVisible = ref(false)

const tenants = ref([])
const instances = ref([])
const loadingInstances = ref(false)

const form = reactive({
  tenant_id: 0,
  instance_id: '',
  cidr_list: '',
  ocpus: null,
  memory: null,
  shape: ''
})

let refreshTimer = null

// ---------------------------------------------------------------------------
// Data loading
// ---------------------------------------------------------------------------
async function loadTenants() {
  try {
    const res = await listTenants()
    tenants.value = res.data || []
  } catch (e) {
    // Tenants will be missing from the selector, that's fine
  }
}

async function loadChangeIPTasks() {
  loading.value = true
  try {
    const res = await get('/mem-tasks/change-ip')
    changeIPTasks.value = res.data || []
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to load Change IP tasks: ' + msg)
  } finally {
    loading.value = false
  }
}

async function loadUpdateCfgTasks() {
  loading.value = true
  try {
    const res = await get('/mem-tasks/update-cfg')
    updateCfgTasks.value = res.data || []
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to load Update Config tasks: ' + msg)
  } finally {
    loading.value = false
  }
}

async function loadTasks() {
  if (activeTab.value === 'change-ip') {
    await loadChangeIPTasks()
  } else {
    await loadUpdateCfgTasks()
  }
}

function onTabChange() {
  loadTasks()
}

// ---------------------------------------------------------------------------
// Instances loading
// ---------------------------------------------------------------------------
async function loadInstances(tenantId) {
  if (!tenantId) {
    instances.value = []
    return
  }
  loadingInstances.value = true
  try {
    const res = await get('/instances', { tenant_id: tenantId })
    instances.value = res.data || []
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to load instances: ' + msg)
  } finally {
    loadingInstances.value = false
  }
}

function onTenantChange() {
  form.instance_id = ''
  loadInstances(form.tenant_id)
}

// ---------------------------------------------------------------------------
// Dialog
// ---------------------------------------------------------------------------
function openAddDialog() {
  form.tenant_id = 0
  form.instance_id = ''
  form.cidr_list = ''
  form.ocpus = null
  form.memory = null
  form.shape = ''
  instances.value = []
  dialogVisible.value = true
}

function onDialogClosed() {
  form.tenant_id = 0
  form.instance_id = ''
  form.cidr_list = ''
  form.ocpus = null
  form.memory = null
  form.shape = ''
  instances.value = []
}

async function handleAdd() {
  if (!form.tenant_id) {
    ElMessage.warning('Please select a tenant')
    return
  }
  if (!form.instance_id) {
    ElMessage.warning('Please select an instance')
    return
  }

  addLoading.value = true
  try {
    const endpoint =
      activeTab.value === 'change-ip'
        ? '/mem-tasks/change-ip'
        : '/mem-tasks/update-cfg'

    const payload = {
      action: 'add',
      tenant_id: form.tenant_id,
      instance_id: form.instance_id
    }

    if (activeTab.value === 'change-ip') {
      if (form.cidr_list) {
        payload.cidr_list = form.cidr_list
      }
    } else {
      if (form.ocpus !== null && form.ocpus !== undefined) {
        payload.ocpus = Number(form.ocpus)
      }
      if (form.memory !== null && form.memory !== undefined) {
        payload.memory = Number(form.memory)
      }
      if (form.shape) {
        payload.shape = form.shape
      }
    }

    await post(endpoint, payload)
    ElMessage.success('Task added successfully')
    dialogVisible.value = false
    await loadTasks()
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to add task: ' + msg)
  } finally {
    addLoading.value = false
  }
}

// ---------------------------------------------------------------------------
// Row actions
// ---------------------------------------------------------------------------
async function handleAction(taskIds, action, taskType) {
  if (action === 'delete') {
    try {
      await ElMessageBox.confirm(
        'Are you sure you want to delete this task?',
        'Confirm Delete',
        {
          confirmButtonText: 'Delete',
          cancelButtonText: 'Cancel',
          type: 'warning'
        }
      )
    } catch {
      return
    }
  }

  const endpoint =
    taskType === 'change-ip'
      ? '/mem-tasks/change-ip'
      : '/mem-tasks/update-cfg'

  try {
    await post(endpoint, {
      action,
      task_ids: Array.isArray(taskIds) ? taskIds : [taskIds]
    })
    ElMessage.success(`Task ${action}d successfully`)
    await loadTasks()
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error(`Action "${action}" failed: ${msg}`)
  }
}

// ---------------------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------------------
function formatDate(dateStr) {
  if (!dateStr) return '-'
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return dateStr
  return d.toLocaleString()
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------
onMounted(() => {
  loadTenants()
  loadTasks()
  refreshTimer = setInterval(() => {
    loadTasks()
  }, 5000)
})

onBeforeUnmount(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer)
    refreshTimer = null
  }
})
</script>

<style scoped>
.in-memory-tasks-page {
  padding: 20px;
}

.in-memory-tasks-page h3 {
  margin-bottom: 24px;
  font-size: 20px;
  font-weight: 600;
}

/* ── Toolbar ──────────────────────────────────────────────────────────── */
.toolbar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  align-items: center;
}

/* ── Monospace text (OCIDs, CIDRs) ───────────────────────────────────── */
.mono-text {
  font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', 'Consolas', monospace;
  font-size: 12px;
}
</style>
