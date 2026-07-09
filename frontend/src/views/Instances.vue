<template>
  <div class="instances-page">
    <!-- Filter Bar -->
    <div class="filter-bar">
      <el-select
        v-model="tenantId"
        :placeholder="$t('instance.allTenants')"
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
        :placeholder="$t('instance.searchPlaceholder')"
        clearable
        @input="handleSearch"
        style="width: 320px"
      />
    </div>

    <!-- Batch Action Bar -->
    <div v-if="selectedRows.length > 0" class="batch-bar">
      <span class="batch-info">{{ $t('instance.selected', { count: selectedRows.length }) }}</span>
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
      @row-click="(row) => $router.push('/instances/' + encodeURIComponent(row.id))"
      @selection-change="onSelectionChange"
      border
      stripe
      style="width: 100%; cursor: pointer"
      row-key="id"
      :element-loading-text="$t('instance.loading')"
    >
      <el-table-column type="selection" width="50" />
      <el-table-column label="Name" min-width="200">
        <template #default="{ row }">
          <el-button type="primary" link @click.stop="openMetrics(row)">
            {{ row.name }}
          </el-button>
        </template>
      </el-table-column>
      <el-table-column :label="$t('instance.shape')" width="150">
        <template #default="{ row }">{{ row.shape }}</template>
      </el-table-column>
      <el-table-column :label="$t('instance.region')" width="130" align="center">
        <template #default="{ row }">{{ row.region || '-' }}</template>
      </el-table-column>
      <el-table-column prop="ocpu" label="OCPU" width="80" align="center" />
      <el-table-column :label="$t('instance.memoryGB')" width="110" align="center">
        <template #default="{ row }">{{ row.memoryGB }}</template>
      </el-table-column>
      <el-table-column :label="$t('instance.state')" width="130" align="center">
        <template #default="{ row }">
          <el-tag :type="stateTagType(row.state)" effect="dark" size="small">
            {{ row.state }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="publicIp" :label="$t('instance.publicIP')" width="150" />
      <el-table-column :label="$t('instance.tenantID')" width="90" align="center">
        <template #default="{ row }">{{ row.tenantId }}</template>
      </el-table-column>
      <el-table-column label="500M" width="90" align="center">
        <template #default="{ row }">
          <el-switch
            :model-value="netStatus[row.id]?.nlb_enabled || false"
            :loading="netBusy[row.id] === '500m'"
            :disabled="!!netBusy[row.id]"
            :before-change="() => confirm500M(row)"
            @click.stop
          />
        </template>
      </el-table-column>
      <el-table-column label="IPv6" width="90" align="center">
        <template #default="{ row }">
          <el-switch
            :model-value="netStatus[row.id]?.ipv6_enabled || false"
            :loading="netBusy[row.id] === 'ipv6'"
            :disabled="!!netBusy[row.id]"
            :before-change="() => confirmIPv6(row)"
            @click.stop
          />
        </template>
      </el-table-column>
      <el-table-column :label="$t('instance.actions')" width="140" fixed="right" align="center">
        <template #default="{ row }">
          <el-dropdown
            trigger="click"
            @command="(cmd) => handleDropdownAction(row, cmd)"
          >
            <el-button type="primary" size="small" @click.stop>
              {{ $t('instance.actions') }}
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

                <el-dropdown-item command="updateName" divided>
                  Update Name
                </el-dropdown-item>
                <el-dropdown-item command="updateShapeOnly">
                  Update Shape Only
                </el-dropdown-item>
                <el-dropdown-item command="configInfo">
                  Config Info
                </el-dropdown-item>
                <el-dropdown-item command="updatePassword">
                  Update Password
                </el-dropdown-item>
                <el-dropdown-item command="autoRescue">
                  Auto Rescue
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </template>
      </el-table-column>
    </el-table>

    <!-- Empty State Override -->
    <el-empty v-if="!loading && instances.length === 0" :description="$t('instance.notFound')" />

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
    <el-dialog v-model="shapeDialogVisible" :title="$t('instance.changeShape')" width="420px" :close-on-click-modal="false">
      <el-form :model="shapeForm" label-width="120px">
        <el-form-item :label="$t('instance.shape')" required>
          <el-input v-model="shapeForm.shape" placeholder="e.g. VM.Standard.E3.Flex" />
        </el-form-item>
        <el-form-item label="OCPU" required>
          <el-input-number
            v-model="shapeForm.ocpus"
            :min="1"
            :max="128"
            controls-position="right"
            style="width: 180px"
          />
        </el-form-item>
        <el-form-item :label="$t('instance.memoryGB')" required>
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
        <el-button @click="shapeDialogVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="handleChangeShape">
          {{ $t('common.save') }}
        </el-button>
      </template>
    </el-dialog>

    <!-- Change Boot Volume Dialog -->
    <el-dialog v-model="volumeDialogVisible" :title="$t('instance.changeBootVolume')" width="420px" :close-on-click-modal="false">
      <p style="margin-bottom: 16px;">
        Resize boot volume for <strong>{{ currentInstance?.name }}</strong>
      </p>
      <el-form :model="volumeForm" label-width="120px">
        <el-form-item :label="$t('instance.bootVolumeSize')" required>
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
        <el-button @click="volumeDialogVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="handleChangeBootVolume">
          {{ $t('common.save') }}
        </el-button>
      </template>
    </el-dialog>

    <!-- Metrics Dialog -->
    <MetricsDialog v-model:visible="metricsVisible" :instance="currentInstance" />

    <!-- Check Alive Results Dialog -->
    <el-dialog v-model="checkAliveVisible" :title="$t('instance.aliveCheckResult')" width="540px">
      <el-table :data="checkAliveResults" stripe border size="small">
        <el-table-column prop="instance_id" :label="$t('instance.instanceID')" min-width="200" show-overflow-tooltip />
        <el-table-column :label="$t('instance.aliveState')" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="row.alive ? 'success' : 'danger'" effect="dark" size="small">
              {{ row.alive ? $t('instance.alive') : $t('instance.dead') }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="$t('instance.errorInfo')" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.error" style="color: #F56C6C; font-size: 12px;">{{ row.error }}</span>
            <span v-else>-</span>
          </template>
        </el-table-column>
      </el-table>
      <template #footer>
        <el-button @click="checkAliveVisible = false">{{ $t('instance.close') }}</el-button>
      </template>
    </el-dialog>

    <!-- Update Name Dialog -->
    <el-dialog v-model="nameDialogVisible" title="Update Name" width="420px" :close-on-click-modal="false">
      <el-form :model="nameForm" label-width="80px">
        <el-form-item label="Name" required>
          <el-input v-model="nameForm.name" placeholder="Enter new instance name" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="nameDialogVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="handleUpdateName">
          {{ $t('common.save') }}
        </el-button>
      </template>
    </el-dialog>

    <!-- Update Shape Only Dialog -->
    <el-dialog v-model="updateShapeDialogVisible" title="Update Shape Only" width="420px" :close-on-click-modal="false">
      <el-form :model="updateShapeForm" label-width="80px">
        <el-form-item label="Shape" required>
          <el-input v-model="updateShapeForm.shape" placeholder="e.g. VM.Standard.E3.Flex" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="updateShapeDialogVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="handleUpdateShapeOnly">
          {{ $t('common.save') }}
        </el-button>
      </template>
    </el-dialog>

    <!-- Config Info Dialog -->
    <el-dialog v-model="configInfoVisible" title="Instance Config Info" width="560px">
      <el-descriptions v-if="configInfoData" :column="1" border size="small">
        <el-descriptions-item label="OCID">{{ configInfoData.id || '-' }}</el-descriptions-item>
        <el-descriptions-item label="Display Name">{{ configInfoData.display_name || '-' }}</el-descriptions-item>
        <el-descriptions-item label="Shape">{{ configInfoData.shape || '-' }}</el-descriptions-item>
        <el-descriptions-item label="State">{{ configInfoData.state || '-' }}</el-descriptions-item>
        <el-descriptions-item label="Region">{{ configInfoData.region || '-' }}</el-descriptions-item>
        <el-descriptions-item label="AD">{{ configInfoData.availability_domain || '-' }}</el-descriptions-item>
        <el-descriptions-item label="Fault Domain">{{ configInfoData.fault_domain || '-' }}</el-descriptions-item>
        <el-descriptions-item label="OCPU">{{ configInfoData.shape_config?.ocpus || '-' }}</el-descriptions-item>
        <el-descriptions-item label="Memory (GB)">{{ configInfoData.shape_config?.memory_gb || '-' }}</el-descriptions-item>
        <el-descriptions-item label="Created">{{ configInfoData.time_created || '-' }}</el-descriptions-item>
        <el-descriptions-item v-if="configInfoData.vnic" label="VNIC ID">{{ configInfoData.vnic.id || '-' }}</el-descriptions-item>
        <el-descriptions-item v-if="configInfoData.vnic" label="Public IP">{{ configInfoData.vnic.public_ip || '-' }}</el-descriptions-item>
        <el-descriptions-item v-if="configInfoData.vnic" label="Private IP">{{ configInfoData.vnic.private_ip || '-' }}</el-descriptions-item>
        <el-descriptions-item v-if="configInfoData.vnic" label="MAC">{{ configInfoData.vnic.mac || '-' }}</el-descriptions-item>
        <el-descriptions-item v-if="configInfoData.boot_volume" label="Boot Volume ID">{{ configInfoData.boot_volume.id || '-' }}</el-descriptions-item>
        <el-descriptions-item v-if="configInfoData.boot_volume" label="Boot Vol Size (GB)">{{ configInfoData.boot_volume.size_gb || '-' }}</el-descriptions-item>
      </el-descriptions>
      <el-empty v-else description="No config info loaded" />
      <template #footer>
        <el-button @click="configInfoVisible = false">{{ $t('instance.close') }}</el-button>
      </template>
    </el-dialog>

    <!-- Update Password Dialog -->
    <el-dialog v-model="passwordDialogVisible" title="Update Instance Password" width="420px" :close-on-click-modal="false">
      <el-form :model="passwordForm" label-width="120px">
        <el-form-item label="New Password" required>
          <el-input v-model="passwordForm.new_password" type="password" show-password placeholder="Min 8 characters" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="passwordDialogVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="handleUpdatePassword">
          {{ $t('common.save') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown } from '@element-plus/icons-vue'
import MetricsDialog from '../components/MetricsDialog.vue'
import {
  listInstances,
  instanceAction,
  batchStart,
  changeShape,
  changeBootVolume,
  updateInstanceName,
  checkAlive
} from '../api/instances.js'
import { get, post } from '../api/index.js'

const { t } = useI18n()

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
const metricsVisible = ref(false)
const checkAliveVisible = ref(false)
const checkAliveResults = ref([])
const checkAliveLoading = ref(false)
const currentInstance = ref(null)

// Per-instance network status (500M/IPv6) and in-flight toggle marker.
const netStatus = reactive({})
const netBusy = reactive({})

const shapeForm = reactive({
  shape: '',
  ocpus: 1,
  memoryGB: 1
})

const volumeForm = reactive({
  sizeGB: 50
})

// ── New action dialogs ────────────────────────────────────────────────
const nameDialogVisible = ref(false)
const updateShapeDialogVisible = ref(false)
const configInfoVisible = ref(false)
const configInfoData = ref(null)
const passwordDialogVisible = ref(false)

const nameForm = reactive({ name: '' })
const updateShapeForm = reactive({ shape: '' })
const passwordForm = reactive({ new_password: '' })

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
    loadNetworkStatus()
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to load instances: ' + msg)
  } finally {
    loading.value = false
  }
}

// ---------------------------------------------------------------------------
// Network status (500M / IPv6) — auto-fetched after each instance load,
// grouped per tenant since network-status is tenant-scoped.
// ---------------------------------------------------------------------------
async function loadNetworkStatus() {
  const rows = instances.value
  if (!rows.length) return
  const byTenant = {}
  for (const row of rows) {
    if (!netStatus[row.id]) {
      netStatus[row.id] = { nlb_enabled: false, nlb_ip: '', ipv6_enabled: false, ipv6_addr: '' }
    }
    ;(byTenant[row.tenantId] ||= []).push(row.id)
  }
  for (const [tid, ids] of Object.entries(byTenant)) {
    try {
      const res = await post('/instances/network-status',
        { tenant_id: Number(tid), instance_ids: ids },
        { timeout: 120000 })
      for (const [id, st] of Object.entries(res || {})) {
        netStatus[id] = st
      }
    } catch (e) {
      // Non-fatal: leave defaults (disabled) for this tenant's rows.
      console.error('network-status failed for tenant', tid, e)
    }
  }
}

// before-change handler for the 500M switch: confirm, call API, return whether
// the toggle should commit. Returns false on cancel or failure (switch reverts).
async function confirm500M(row) {
  const enabling = !netStatus[row.id]?.nlb_enabled
  try {
    await ElMessageBox.confirm(
      enabling ? t('instance.confirm500MEnable') : t('instance.confirm500MDisable'),
      t('instance.networkBoost'),
      { type: 'warning' }
    )
  } catch {
    return false // cancelled
  }
  netBusy[row.id] = '500m'
  try {
    if (enabling) {
      const res = await post('/instances/one-click-500m',
        { tenant_id: row.tenantId, instance_id: row.id }, { timeout: 360000 })
      netStatus[row.id] = { ...netStatus[row.id], nlb_enabled: true, nlb_ip: res?.nlb_ip || '' }
      ElMessage.success(t('instance.enabled500M') + (res?.nlb_ip ? ` (${res.nlb_ip})` : ''))
    } else {
      await post('/instances/one-click-close-500m',
        { tenant_id: row.tenantId, instance_id: row.id }, { timeout: 180000 })
      netStatus[row.id] = { ...netStatus[row.id], nlb_enabled: false, nlb_ip: '' }
      ElMessage.success(t('instance.disabled500M'))
    }
    return true
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '500M operation failed')
    return false
  } finally {
    netBusy[row.id] = ''
  }
}

// before-change handler for the IPv6 switch.
async function confirmIPv6(row) {
  const enabling = !netStatus[row.id]?.ipv6_enabled
  try {
    await ElMessageBox.confirm(
      enabling ? t('instance.confirmIPv6Enable') : t('instance.confirmIPv6Disable'),
      t('instance.networkBoost'),
      { type: 'warning', dangerouslyUseHTMLString: false }
    )
  } catch {
    return false
  }
  netBusy[row.id] = 'ipv6'
  try {
    if (enabling) {
      const res = await post('/instances/attach-ipv6',
        { tenant_id: row.tenantId, instance_id: row.id }, { timeout: 360000 })
      netStatus[row.id] = { ...netStatus[row.id], ipv6_enabled: true, ipv6_addr: res?.ipv6 || '' }
      ElMessage.success(t('instance.enabledIPv6') + (res?.ipv6 ? ` (${res.ipv6})` : ''))
    } else {
      await post('/instances/disable-ipv6',
        { tenant_id: row.tenantId, instance_id: row.id }, { timeout: 180000 })
      netStatus[row.id] = { ...netStatus[row.id], ipv6_enabled: false, ipv6_addr: '' }
      ElMessage.success(t('instance.disabledIPv6'))
    }
    return true
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'IPv6 operation failed')
    return false
  } finally {
    netBusy[row.id] = ''
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
    case 'updateName':
      nameForm.name = row.name || ''
      nameDialogVisible.value = true
      break
    case 'updateShapeOnly':
      updateShapeForm.shape = row.shape || ''
      updateShapeDialogVisible.value = true
      break
    case 'configInfo':
      handleConfigInfo(row)
      break
    case 'updatePassword':
      passwordForm.new_password = ''
      passwordDialogVisible.value = true
      break
    case 'autoRescue':
      handleAutoRescue(row)
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
  if (selectedRows.value.length === 0) {
    ElMessage.warning('Select at least one instance')
    return
  }
  checkAliveLoading.value = true
  try {
    const res = await checkAlive({
      tenant_id: selectedRows.value[0].tenantId,
      instance_ids: selectedRows.value.map(r => r.id)
    })
    checkAliveResults.value = res.results || []
    checkAliveVisible.value = true
  } catch (e) {
    ElMessage.error(e.response?.data?.error || e.message)
  } finally {
    checkAliveLoading.value = false
  }
}

// ── New action handlers ───────────────────────────────────────────────
async function handleUpdateName() {
  saving.value = true
  try {
    await updateInstanceName({
      tenant_id: currentInstance.value.tenantId,
      instance_id: currentInstance.value.id,
      name: nameForm.name
    })
    ElMessage.success('Instance name updated')
    nameDialogVisible.value = false
    await loadInstances()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Update name failed')
  } finally {
    saving.value = false
  }
}

async function handleUpdateShapeOnly() {
  saving.value = true
  try {
    await post('/instances/update-shape', {
      tenant_id: currentInstance.value.tenantId,
      instance_id: currentInstance.value.id,
      shape: updateShapeForm.shape
    })
    ElMessage.success('Shape update submitted')
    updateShapeDialogVisible.value = false
    await loadInstances()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Update shape failed')
  } finally {
    saving.value = false
  }
}

async function handleConfigInfo(row) {
  try {
    const res = await post('/instances/config-info', {
      tenant_id: row.tenantId,
      instance_id: row.id
    })
    configInfoData.value = res
    configInfoVisible.value = true
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Config info failed')
  }
}

async function handleUpdatePassword() {
  saving.value = true
  try {
    const res = await post('/instances/update-password', {
      tenant_id: currentInstance.value.tenantId,
      instance_id: currentInstance.value.id,
      new_password: passwordForm.new_password
    })
    passwordDialogVisible.value = false
    ElMessage.success(res.message || 'Password update initiated')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Update password failed')
  } finally {
    saving.value = false
  }
}

async function handleAutoRescue(row) {
  try {
    await ElMessageBox.confirm(
      `Auto Rescue will try TCP check → softreset → reset → stop+start for "${row.name}". Continue?`,
      'Auto Rescue',
      { confirmButtonText: 'Rescue', cancelButtonText: 'Cancel', type: 'warning' }
    )
    const res = await post('/instances/auto-rescue', {
      tenant_id: row.tenantId,
      instance_id: row.id
    })
    ElMessage.success(res.message || 'Auto rescue completed')
    await loadInstances()
  } catch (e) {
    if (e !== 'cancel') {
      ElMessage.error(e.response?.data?.error || 'Auto rescue failed')
    }
  }
}

// ---------------------------------------------------------------------------
// Metrics
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
