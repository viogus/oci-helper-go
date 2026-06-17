<template>
  <div class="security-rules-page">
    <div class="page-header">
      <h3>安全规则</h3>
    </div>

    <!-- Selectors -->
    <div class="filter-bar">
      <el-select
        v-model="tenantId"
        placeholder="选择租户"
        :loading="loadingTenants"
        :disabled="loadingTenants"
        style="width: 240px"
        @change="onTenantChange"
      >
        <el-option
          v-for="t in tenants"
          :key="t.id"
          :label="t.name"
          :value="t.id"
        />
      </el-select>

      <el-select
        v-model="vcnId"
        placeholder="选择 VCN"
        :loading="loadingVCNs"
        :disabled="!tenantId"
        style="width: 400px"
        @change="onVCNChange"
      >
        <el-option
          v-for="vcn in vcns"
          :key="vcn.id"
          :label="vcn.displayName || vcn.id"
          :value="vcn.id"
        />
      </el-select>

      <el-input
        v-model="keyword"
        placeholder="过滤规则..."
        clearable
        :disabled="!vcnId"
        style="width: 240px"
        @input="handleSearch"
        @clear="handleSearch"
      >
        <template #prefix>
          <el-icon><Search /></el-icon>
        </template>
      </el-input>
    </div>

    <!-- Action Buttons -->
    <div class="action-bar">
      <el-button
        type="primary"
        :disabled="!vcnId"
        @click="dialogVisible = true"
      >
        <el-icon><Plus /></el-icon> 添加规则
      </el-button>
      <el-button
        type="danger"
        :disabled="!vcnId"
        @click="handleRelease"
      >
        <el-icon><WarningFilled /></el-icon> 放开所有端口
      </el-button>
    </div>

    <!-- Rules Table -->
    <el-table
      :data="rules"
      v-loading="loading"
      stripe
      border
      empty-text="未找到安全规则"
      style="width: 100%"
    >
      <el-table-column prop="name" label="名称" min-width="200" show-overflow-tooltip />
      <el-table-column prop="protocol" label="协议" width="100" align="center">
        <template #default="{ row }">
          <el-tag :type="protocolTagType(row.protocol)" size="small">
            {{ row.protocol || 'all' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="来源 / 目标" min-width="180">
        <template #default="{ row }">
          <span>{{ row.type === 'ingress' ? row.source : row.dest }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="port" label="端口" width="120" align="center">
        <template #default="{ row }">
          <span>{{ row.port || 'all' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="类型" width="100" align="center">
        <template #default="{ row }">
          <el-tag
            :type="row.type === 'ingress' ? 'warning' : 'success'"
            effect="plain"
            size="small"
          >
            {{ row.type }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="80" fixed="right" align="center">
        <template #default="{ row }">
          <el-button
            type="danger"
            link
            size="small"
            :loading="deletingId === row.id"
            @click="handleDelete(row)"
          >
            Delete
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- Empty state when no VCN selected -->
    <el-empty
      v-if="!vcnId && !loading"
      description="选择租户和 VCN 查看安全规则"
    />

    <!-- Pagination -->
    <div v-if="total > 0" class="pagination-wrapper">
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

    <!-- 添加规则 Dialog -->
    <el-dialog
      v-model="dialogVisible"
      title="添加安全规则"
      width="520px"
      :close-on-click-modal="false"
      @closed="onDialogClosed"
    >
      <el-form :model="ruleForm" label-width="100px">
        <el-form-item label="类型" required>
          <el-radio-group v-model="ruleForm.type">
            <el-radio value="ingress">入站</el-radio>
            <el-radio value="egress">出站</el-radio>
          </el-radio-group>
        </el-form-item>

        <el-form-item label="协议" required>
          <el-select v-model="ruleForm.protocol" style="width: 100%">
            <el-option label="TCP" value="TCP" />
            <el-option label="UDP" value="UDP" />
            <el-option label="ICMP" value="ICMP" />
            <el-option label="All" value="all" />
          </el-select>
        </el-form-item>

        <el-form-item label="端口" required>
          <el-input
            v-model="ruleForm.port"
            placeholder="e.g. 80, 443, or 3000-4000"
          />
        </el-form-item>

        <el-form-item v-if="ruleForm.type === 'ingress'" label="来源" required>
          <el-input
            v-model="ruleForm.source"
            placeholder="0.0.0.0/0"
          />
        </el-form-item>

        <el-form-item v-if="ruleForm.type === 'egress'" label="目标" required>
          <el-input
            v-model="ruleForm.dest"
            placeholder="0.0.0.0/0"
          />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="handleAdd">
          Add
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search, WarningFilled } from '@element-plus/icons-vue'
import { get } from '../api/index.js'
import {
  listSecurityRules,
  addIngressRule,
  addEgressRule,
  removeSecurityRules,
  releaseAllPorts
} from '../api/securityRules.js'

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------
const tenants = ref([])
const vcns = ref([])
const rules = ref([])
const total = ref(0)
const page = ref(1)
const size = ref(20)
const keyword = ref('')
const tenantId = ref(0)
const vcnId = ref('')
const loading = ref(false)
const loadingTenants = ref(false)
const loadingVCNs = ref(false)
const saving = ref(false)
const deletingId = ref(null)

const dialogVisible = ref(false)
const ruleForm = reactive({
  type: 'ingress',
  protocol: 'TCP',
  port: '',
  source: '0.0.0.0/0',
  dest: '0.0.0.0/0'
})

// ---------------------------------------------------------------------------
// Debounced search
// ---------------------------------------------------------------------------
let searchTimer = null

function handleSearch() {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    loadRules()
  }, 300)
}

// ---------------------------------------------------------------------------
// Data Loading
// ---------------------------------------------------------------------------
async function loadTenants() {
  loadingTenants.value = true
  try {
    const res = await get('/tenants')
    tenants.value = res.data || []
  } catch (e) {
    ElMessage.error('Failed to load tenants')
    tenants.value = []
  }
  loadingTenants.value = false
}

async function loadVCNs() {
  loadingVCNs.value = true
  vcnId.value = ''
  rules.value = []
  total.value = 0
  try {
    const res = await get('/vcns', { tenant_id: tenantId.value })
    vcns.value = Array.isArray(res) ? res : []
  } catch (e) {
    ElMessage.error('Failed to load VCNs')
    vcns.value = []
  }
  loadingVCNs.value = false
}

async function loadRules() {
  if (!vcnId.value) {
    rules.value = []
    total.value = 0
    return
  }
  loading.value = true
  try {
    const res = await listSecurityRules({
      tenant_id: tenantId.value,
      vcn_id: vcnId.value,
      keyword: keyword.value || undefined,
      page: page.value,
      size: size.value
    })
    rules.value = res.data || []
    total.value = res.total || 0
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to load rules: ' + msg)
    rules.value = []
    total.value = 0
  }
  loading.value = false
}

// ---------------------------------------------------------------------------
// Selector Handlers
// ---------------------------------------------------------------------------
function onTenantChange() {
  vcns.value = []
  rules.value = []
  total.value = 0
  page.value = 1
  if (tenantId.value) {
    loadVCNs()
  }
}

function onVCNChange() {
  rules.value = []
  total.value = 0
  page.value = 1
  if (vcnId.value) {
    loadRules()
  }
}

// ---------------------------------------------------------------------------
// Pagination
// ---------------------------------------------------------------------------
function onSizeChange() {
  page.value = 1
  loadRules()
}

function onPageChange() {
  loadRules()
}

// ---------------------------------------------------------------------------
// Protocol Tag Colour
// ---------------------------------------------------------------------------
function protocolTagType(protocol) {
  switch ((protocol || '').toUpperCase()) {
    case 'TCP':
      return 'primary'
    case 'UDP':
      return 'success'
    case 'ICMP':
      return 'warning'
    default:
      return 'info'
  }
}

// ---------------------------------------------------------------------------
// 添加规则
// ---------------------------------------------------------------------------
function onDialogClosed() {
  ruleForm.type = 'ingress'
  ruleForm.protocol = 'TCP'
  ruleForm.port = ''
  ruleForm.source = '0.0.0.0/0'
  ruleForm.dest = '0.0.0.0/0'
}

async function handleAdd() {
  // Validation
  if (!ruleForm.port.trim()) {
    ElMessage.warning('Port is required')
    return
  }
  if (ruleForm.type === 'ingress' && !ruleForm.source.trim()) {
    ElMessage.warning('Source is required for ingress rules')
    return
  }
  if (ruleForm.type === 'egress' && !ruleForm.dest.trim()) {
    ElMessage.warning('Destination is required for egress rules')
    return
  }

  saving.value = true
  try {
    const payload = {
      tenant_id: tenantId.value,
      vcn_id: vcnId.value,
      protocol: ruleForm.protocol === 'all' ? '' : ruleForm.protocol,
      port: ruleForm.port
    }

    if (ruleForm.type === 'ingress') {
      payload.source = ruleForm.source.trim()
      await addIngressRule(payload)
    } else {
      payload.dest = ruleForm.dest.trim()
      await addEgressRule(payload)
    }

    ElMessage.success('Rule added successfully')
    dialogVisible.value = false
    await loadRules()
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to add rule: ' + msg)
  }
  saving.value = false
}

// ---------------------------------------------------------------------------
// 删除规则
// ---------------------------------------------------------------------------
async function handleDelete(rule) {
  try {
    await ElMessageBox.confirm(
      `Delete this ${rule.type} rule from "${rule.name}"?`,
      '删除规则',
      {
        confirmButtonText: 'Delete',
        cancelButtonText: 'Cancel',
        type: 'warning'
      }
    )
  } catch {
    return
  }

  deletingId.value = rule.id
  try {
    await removeSecurityRules({
      tenant_id: tenantId.value,
      vcn_id: vcnId.value,
      rule_ids: [rule.id]
    })
    ElMessage.success('Rule deleted')
    // Refresh current page; if it becomes empty, go back a page
    const prevTotal = total.value
    await loadRules()
    if (prevTotal > 0 && rules.value.length === 0 && page.value > 1) {
      page.value--
      await loadRules()
    }
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to delete rule: ' + msg)
  }
  deletingId.value = null
}

// ---------------------------------------------------------------------------
// Release All Ports
// ---------------------------------------------------------------------------
async function handleRelease() {
  try {
    await ElMessageBox.confirm(
      'This will open all TCP and UDP ports (0-65535) for both ingress and egress on this VCN. This is a security risk. Continue?',
      '放开所有端口',
      {
        confirmButtonText: '确认放开所有端口',
        cancelButtonText: 'Cancel',
        type: 'warning',
        confirmButtonClass: 'el-button--danger'
      }
    )
  } catch {
    return
  }

  loading.value = true
  try {
    await releaseAllPorts({
      tenant_id: tenantId.value,
      vcn_id: vcnId.value
    })
    ElMessage.success('All ports opened successfully')
    await loadRules()
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to open ports: ' + msg)
  }
  loading.value = false
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------
onMounted(() => {
  loadTenants()
})
</script>

<style scoped>
.security-rules-page {
  padding: 20px;
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
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  align-items: center;
  flex-wrap: wrap;
}

.action-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  align-items: center;
}

.pagination-wrapper {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
  padding: 8px 0;
}
</style>
