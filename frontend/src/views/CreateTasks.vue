<template>
  <div class="create-tasks-page">
    <h3>Create Tasks</h3>

    <!-- Search & Refresh Bar -->
    <div class="toolbar">
      <el-input
        v-model="keyword"
        placeholder="Search tasks..."
        clearable
        @input="handleSearch"
        style="width: 320px"
      />
      <el-button @click="loadTasks" :loading="loading" :icon="Refresh">
        Refresh
      </el-button>
    </div>

    <!-- Batch Action Bar -->
    <div v-if="selectedRows.length > 0" class="batch-bar">
      <span class="batch-info">{{ selectedRows.length }} task(s) selected</span>
      <el-button type="warning" size="small" @click="handleBatchAction('stop')">
        Batch Stop
      </el-button>
      <el-button type="warning" size="small" @click="handleBatchAction('pause')">
        Batch Pause
      </el-button>
      <el-button type="primary" size="small" @click="handleBatchAction('resume')">
        Batch Resume
      </el-button>
      <el-button type="danger" size="small" @click="handleBatchAction('delete')">
        Batch Delete
      </el-button>
    </div>

    <!-- Tasks Table -->
    <el-table
      :data="tasks"
      v-loading="loading"
      @selection-change="onSelectionChange"
      border
      stripe
      style="width: 100%"
      row-key="id"
      element-loading-text="Loading tasks..."
    >
      <el-table-column type="selection" width="50" />
      <el-table-column label="ID" prop="id" width="70" align="center" />
      <el-table-column label="Tenant ID" prop="tenantId" width="100" align="center" />
      <el-table-column label="Status" width="110" align="center">
        <template #default="{ row }">
          <el-tag :type="statusTagType(row.status)" effect="dark" size="small">
            {{ row.status }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="Progress" width="160" align="center">
        <template #default="{ row }">
          <el-progress
            :percentage="row.progress"
            :status="progressStatus(row)"
            :stroke-width="14"
            style="width: 120px; display: inline-block"
          />
        </template>
      </el-table-column>
      <el-table-column label="Message" min-width="200">
        <template #default="{ row }">
          <span class="task-message">{{ row.message || '-' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="Created At" width="170" align="center">
        <template #default="{ row }">
          {{ formatDate(row.createdAt) }}
        </template>
      </el-table-column>
      <el-table-column label="Actions" width="260" fixed="right" align="center">
        <template #default="{ row }">
          <el-button
            v-if="row.status === 'running'"
            type="warning"
            size="small"
            @click="handleAction('stop', [row.id])"
          >
            Stop
          </el-button>
          <el-button
            v-if="row.status === 'running'"
            type="warning"
            size="small"
            @click="handleAction('pause', [row.id])"
          >
            Pause
          </el-button>
          <el-button
            v-if="row.status === 'paused'"
            type="primary"
            size="small"
            @click="handleAction('resume', [row.id])"
          >
            Resume
          </el-button>
          <el-button
            size="small"
            @click="handleEdit(row)"
          >
            Edit
          </el-button>
          <el-button
            v-if="row.status !== 'cancelled'"
            type="danger"
            size="small"
            @click="handleAction('delete', [row.id])"
          >
            Delete
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- Empty State -->
    <el-empty v-if="!loading && tasks.length === 0" description="No create tasks found" />

    <!-- Pagination -->
    <div class="pagination-wrapper">
      <el-pagination
        v-model:current-page="page"
        v-model:page-size="size"
        :total="total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next"
        @size-change="onSizeChange"
        @current-change="onPageChange"
      />
    </div>

    <!-- Edit Dialog -->
    <el-dialog
      v-model="editDialogVisible"
      title="Edit Task"
      width="500px"
      :close-on-click-modal="false"
    >
      <el-form :model="editForm" label-width="180px">
        <el-form-item label="Instances Per Tenant">
          <el-input-number
            v-model="editForm.instancesPerTenant"
            :min="1"
            :max="10"
            controls-position="right"
            style="width: 180px"
          />
        </el-form-item>
        <el-form-item label="Region">
          <el-input v-model="editForm.region" style="width: 280px" />
        </el-form-item>
        <el-form-item label="Boot Volume (GB)">
          <el-input-number
            v-model="editForm.bootVolumeSizeGB"
            :min="50"
            :max="2048"
            controls-position="right"
            style="width: 180px"
          />
        </el-form-item>
        <el-form-item label="Display Name Prefix">
          <el-input v-model="editForm.displayNamePrefix" style="width: 280px" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editDialogVisible = false">Cancel</el-button>
        <el-button type="primary" :loading="savingEdit" @click="handleSaveEdit">
          Save
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { listCreateTasks, stopTasks, pauseTasks, resumeTasks, deleteTasks, updateTask } from '../api/tasks.js'

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------
const tasks = ref([])
const total = ref(0)
const page = ref(1)
const size = ref(20)
const keyword = ref('')
const selectedRows = ref([])
const loading = ref(false)
const savingEdit = ref(false)

// Edit dialog
const editDialogVisible = ref(false)
const editTaskId = ref(0)
const editForm = reactive({
  instancesPerTenant: 1,
  region: '',
  bootVolumeSizeGB: 50,
  displayNamePrefix: 'oci-helper'
})

// Auto-refresh
let refreshTimer = null

// ---------------------------------------------------------------------------
// Debounced search
// ---------------------------------------------------------------------------
let searchTimer = null

function handleSearch() {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    loadTasks()
  }, 300)
}

// ---------------------------------------------------------------------------
// Data loading
// ---------------------------------------------------------------------------
async function loadTasks() {
  loading.value = true
  try {
    const params = {
      page: page.value,
      size: size.value
    }
    if (keyword.value) {
      params.keyword = keyword.value
    }
    const res = await listCreateTasks(params)
    tasks.value = res.data || []
    total.value = res.total || 0
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to load tasks: ' + msg)
  } finally {
    loading.value = false
  }
}

// ---------------------------------------------------------------------------
// Pagination
// ---------------------------------------------------------------------------
function onSizeChange() {
  page.value = 1
  loadTasks()
}

function onPageChange() {
  loadTasks()
}

// ---------------------------------------------------------------------------
// Selection
// ---------------------------------------------------------------------------
function onSelectionChange(rows) {
  selectedRows.value = rows
}

// ---------------------------------------------------------------------------
// Status helpers
// ---------------------------------------------------------------------------
function statusTagType(status) {
  switch (status) {
    case 'running':
      return 'warning'
    case 'pending':
      return 'info'
    case 'completed':
      return 'success'
    case 'failed':
      return 'danger'
    case 'paused':
      return 'warning'
    case 'cancelled':
      return 'info'
    default:
      return 'info'
  }
}

function progressStatus(row) {
  if (row.status === 'completed') return 'success'
  if (row.status === 'failed') return 'exception'
  return undefined
}

// ---------------------------------------------------------------------------
// Date formatting
// ---------------------------------------------------------------------------
function formatDate(dateStr) {
  if (!dateStr) return '-'
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return dateStr
  return d.toLocaleString()
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------
async function handleAction(action, taskIds) {
  if (action === 'delete') {
    try {
      await ElMessageBox.confirm(
        `Are you sure you want to delete ${taskIds.length} task(s)?`,
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

  try {
    if (action === 'stop') {
      await stopTasks(taskIds)
    } else if (action === 'pause') {
      await pauseTasks(taskIds)
    } else if (action === 'resume') {
      await resumeTasks(taskIds)
    } else if (action === 'delete') {
      await deleteTasks(taskIds)
    }
    ElMessage.success(`Action "${action}" completed`)
    selectedRows.value = []
    await loadTasks()
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error(`Action "${action}" failed: ${msg}`)
  }
}

async function handleBatchAction(action) {
  if (selectedRows.value.length === 0) return
  const ids = selectedRows.value.map((r) => r.id)

  if (action === 'delete') {
    try {
      await ElMessageBox.confirm(
        `Are you sure you want to delete ${ids.length} task(s)?`,
        'Confirm Batch Delete',
        {
          confirmButtonText: 'Delete All',
          cancelButtonText: 'Cancel',
          type: 'warning'
        }
      )
    } catch {
      return
    }
  }

  try {
    if (action === 'stop') {
      await stopTasks(ids)
    } else if (action === 'pause') {
      await pauseTasks(ids)
    } else if (action === 'resume') {
      await resumeTasks(ids)
    } else if (action === 'delete') {
      await deleteTasks(ids)
    }
    ElMessage.success(`Batch "${action}" completed for ${ids.length} task(s)`)
    selectedRows.value = []
    await loadTasks()
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error(`Batch "${action}" failed: ${msg}`)
  }
}

// ---------------------------------------------------------------------------
// Edit
// ---------------------------------------------------------------------------
function handleEdit(task) {
  editTaskId.value = task.id
  try {
    const payload = JSON.parse(task.payload || '{}')
    editForm.instancesPerTenant = payload.instances_per_tenant || 1
    editForm.region = payload.region || ''
    editForm.bootVolumeSizeGB = payload.boot_volume_size_gb || 50
    editForm.displayNamePrefix = payload.display_name_prefix || 'oci-helper'
  } catch {
    editForm.instancesPerTenant = 1
    editForm.region = ''
    editForm.bootVolumeSizeGB = 50
    editForm.displayNamePrefix = 'oci-helper'
  }
  editDialogVisible.value = true
}

async function handleSaveEdit() {
  savingEdit.value = true
  try {
    const payload = {
      instances_per_tenant: editForm.instancesPerTenant,
      region: editForm.region || undefined,
      boot_volume_size_gb: editForm.bootVolumeSizeGB,
      display_name_prefix: editForm.displayNamePrefix || 'oci-helper'
    }
    await updateTask(editTaskId.value, payload)
    ElMessage.success('Task updated')
    editDialogVisible.value = false
    await loadTasks()
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Update failed: ' + msg)
  } finally {
    savingEdit.value = false
  }
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------
onMounted(() => {
  loadTasks()
  refreshTimer = setInterval(() => {
    loadTasks()
  }, 10000)
})

onBeforeUnmount(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer)
    refreshTimer = null
  }
})
</script>

<style scoped>
.create-tasks-page {
  padding: 20px;
}

.create-tasks-page h3 {
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

/* ── Task message ─────────────────────────────────────────────────────── */
.task-message {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

/* ── Pagination ───────────────────────────────────────────────────────── */
.pagination-wrapper {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
  padding: 8px 0;
}
</style>
