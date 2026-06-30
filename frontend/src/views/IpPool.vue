<template>
  <div class="ip-pool-page">
    <div class="page-header">
      <h3>{{ $t('ipPool.title') }}</h3>
      <div class="header-actions">
        <el-button type="primary" @click="handleAdd">
          <el-icon><Plus /></el-icon> {{ $t('ipPool.add') }}
        </el-button>
        <el-button
          :disabled="!selectedTenantId"
          :loading="importing"
          @click="handleImportOci"
        >
          {{ $t('ipPool.importOci') }}
        </el-button>
      </div>
    </div>

    <!-- Tenant Selector -->
    <div class="filter-bar">
      <el-select
        v-model="selectedTenantId"
        :placeholder="$t('tenant.title')"
        clearable
        @change="onTenantChange"
        :loading="tenantsLoading"
        style="width: 300px"
      >
        <el-option
          v-for="t in tenants"
          :key="t.id"
          :label="t.name"
          :value="t.id"
        />
      </el-select>
    </div>

    <!-- Tabs -->
    <el-tabs v-model="activeTab" @tab-change="onTabChange">
      <el-tab-pane label="Pool" :name="tabTypes.pool">
        <template #label>
          {{ $t('ipPool.pool') }}
        </template>
      </el-tab-pane>
      <el-tab-pane label="Whitelist" :name="tabTypes.whitelist">
        <template #label>
          {{ $t('ipPool.whitelist') }}
        </template>
      </el-tab-pane>
      <el-tab-pane label="Blacklist" :name="tabTypes.blacklist">
        <template #label>
          {{ $t('ipPool.blacklist') }}
        </template>
      </el-tab-pane>
    </el-tabs>

    <!-- Table -->
    <el-table
      :data="tableData"
      stripe
      v-loading="loading"
      :empty-text="$t('ipPool.notFound')"
    >
      <el-table-column :label="$t('ipPool.cidr')" min-width="200">
        <template #default="{ row }">
          <code style="font-family: monospace;">{{ row.cidr }}</code>
        </template>
      </el-table-column>
      <el-table-column prop="label" :label="$t('ipPool.label')" min-width="160">
        <template #default="{ row }">
          {{ row.label || '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="type" :label="$t('ipPool.type')" width="110">
        <template #default="{ row }">
          <el-tag size="small">
            {{ row.type }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="$t('ipPool.enabled')" width="100" align="center">
        <template #default="{ row }">
          <el-switch
            :model-value="row.enabled"
            @change="(val) => handleToggleEnabled(row, val)"
            size="small"
          />
        </template>
      </el-table-column>
      <el-table-column :label="$t('tenant.actions')" width="140" fixed="right">
        <template #default="{ row }">
          <el-button type="primary" link size="small" @click="handleEdit(row)">
            Edit
          </el-button>
          <el-button type="danger" link size="small" @click="handleDelete(row)">
            {{ $t('common.delete') }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-empty
      v-if="!loading && !selectedTenantId"
      description="Select a tenant to view IP data"
    />

    <!-- Add / Edit Dialog -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEditing ? $t('ipPool.editTitle') : $t('ipPool.addTitle')"
      width="480px"
      :close-on-click-modal="false"
      @closed="resetForm"
    >
      <el-form label-position="top">
        <el-form-item :label="$t('ipPool.cidr')" required>
          <el-input v-model="form.cidr" placeholder="e.g. 10.0.0.0/8" />
        </el-form-item>
        <el-form-item :label="$t('ipPool.label')">
          <el-input v-model="form.label" :placeholder="$t('ipPool.label')" />
        </el-form-item>
        <el-form-item :label="$t('ipPool.type')">
          <el-select v-model="form.type" style="width: 100%">
            <el-option label="pool" value="pool" />
            <el-option label="whitelist" value="whitelist" />
            <el-option label="deny" value="deny" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('ipPool.enabled')">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="handleSave">
          {{ $t('common.save') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
import { get, post, put, del } from '../api/index.js'

const tabTypes = { pool: 'pool', whitelist: 'whitelist', blacklist: 'deny' }

const tenants = ref([])
const selectedTenantId = ref('')
const activeTab = ref(tabTypes.pool)
const tableData = ref([])
const tenantsLoading = ref(false)
const loading = ref(false)
const saving = ref(false)
const importing = ref(false)

const dialogVisible = ref(false)
const isEditing = ref(false)
const editingId = ref(null)
const form = reactive({
  cidr: '',
  label: '',
  type: 'pool',
  enabled: true
})

function resetForm() {
  form.cidr = ''
  form.label = ''
  form.type = 'pool'
  form.enabled = true
}

onMounted(() => {
  loadTenants()
})

async function loadTenants() {
  tenantsLoading.value = true
  try {
    const res = await get('/tenants')
    tenants.value = res.data || []
  } catch (e) {
    ElMessage.error('Failed to load tenants')
  }
  tenantsLoading.value = false
}

function onTenantChange() {
  tableData.value = []
  if (selectedTenantId.value) {
    loadData()
  }
}

function onTabChange() {
  if (selectedTenantId.value) {
    loadData()
  }
}

async function loadData() {
  if (!selectedTenantId.value) return
  loading.value = true
  try {
    const res = await get('/ip-data', {
      tenant_id: selectedTenantId.value,
      type: activeTab.value
    })
    tableData.value = res.data || []
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to load IP data')
    tableData.value = []
  }
  loading.value = false
}

function handleAdd() {
  isEditing.value = false
  editingId.value = null
  form.type = activeTab.value
  resetForm()
  dialogVisible.value = true
}

function handleEdit(row) {
  isEditing.value = true
  editingId.value = row.id
  form.cidr = row.cidr || ''
  form.label = row.label || ''
  form.type = row.type || 'pool'
  form.enabled = !!row.enabled
  dialogVisible.value = true
}

async function handleSave() {
  if (!form.cidr.trim()) {
    ElMessage.warning('CIDR is required')
    return
  }
  saving.value = true
  try {
    if (isEditing.value) {
      await put(`/ip-data/${editingId.value}`, {
        cidr: form.cidr,
        label: form.label,
        type: form.type,
        enabled: form.enabled
      })
      ElMessage.success('Updated')
    } else {
      await post('/ip-data', {
        tenant_id: selectedTenantId.value,
        cidr: form.cidr,
        label: form.label,
        type: form.type,
        enabled: form.enabled
      })
      ElMessage.success('Created')
    }
    dialogVisible.value = false
    loadData()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to save')
  }
  saving.value = false
}

async function handleToggleEnabled(row, val) {
  try {
    await put(`/ip-data/${row.id}`, {
      cidr: row.cidr,
      label: row.label,
      type: row.type,
      enabled: val
    })
    row.enabled = val
  } catch (e) {
    ElMessage.error('Failed to update')
  }
}

async function handleDelete(row) {
  try {
    await ElMessageBox.confirm(
      t('ipPool.confirmDelete'),
      t('common.delete'),
      {
        confirmButtonText: t('common.delete'),
        cancelButtonText: t('common.cancel'),
        type: 'warning'
      }
    )
    await del(`/ip-data/${row.id}`)
    ElMessage.success('Deleted')
    loadData()
  } catch (err) {
    if (err !== 'cancel') {
      ElMessage.error(err.response?.data?.error || 'Delete failed')
    }
  }
}

async function handleImportOci() {
  importing.value = true
  try {
    const res = await post('/ip-data', {
      action: 'load_oci',
      tenant_id: selectedTenantId.value
    })
    const count = res.added || 0
    ElMessage.success(t('ipPool.importSuccess', { count }))
    loadData()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Import failed')
  }
  importing.value = false
}
</script>

<style scoped>
.ip-pool-page {
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

.header-actions {
  display: flex;
  gap: 8px;
}

.filter-bar {
  margin-bottom: 16px;
}
</style>
