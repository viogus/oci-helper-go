<template>
  <div class="limits-page">
    <div class="page-header">
      <h3>配额与限制</h3>
    </div>

    <!-- Filter Bar -->
    <div class="filter-bar">
      <el-select
        v-model="tenantId"
        placeholder="选择租户"
        @change="loadLimits"
        style="width: 220px"
      >
        <el-option
          v-for="t in tenants"
          :key="t.id"
          :label="t.name"
          :value="t.id"
        />
      </el-select>
      <el-input
        v-model="serviceName"
        placeholder="按服务名筛选..."
        clearable
        @input="onServiceInput"
        style="width: 240px"
      />
    </div>

    <!-- Summary Cards -->
    <div v-if="limits.length > 0" class="summary-row">
      <el-card class="summary-card" shadow="never">
        <div class="summary-value">{{ summary.total }}</div>
        <div class="summary-label">总计</div>
      </el-card>
      <el-card class="summary-card critical" shadow="never">
        <div class="summary-value">{{ summary.critical }}</div>
        <div class="summary-label">严重 (&gt;Critical (&gt;80%)&lt;80%)</div>
      </el-card>
      <el-card class="summary-card warning" shadow="never">
        <div class="summary-value">{{ summary.warning }}</div>
        <div class="summary-label">警告 (60-80%)</div>
      </el-card>
    </div>

    <!-- Limits Table -->
    <el-table
      :data="limits"
      v-loading="loading"
      stripe
      border
      style="width: 100%"
      empty-text="未找到配额"
    >
      <el-table-column label="服务名" width="160">
        <template #default="{ row }">
          <el-tag type="primary" effect="plain" size="small">
            {{ row.serviceName }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="name" label="配额名称" min-width="220" />
      <el-table-column label="已用" width="140" align="right">
        <template #default="{ row }">
          {{ used(row.used) }}
        </template>
      </el-table-column>
      <el-table-column label="可用" width="140" align="right">
        <template #default="{ row }">
          {{ available(row) }}
        </template>
      </el-table-column>
      <el-table-column label="最大 / 配额" width="140" align="right">
        <template #default="{ row }">
          {{ max(row.max) }}
        </template>
      </el-table-column>
      <el-table-column label="使用率 %" width="200" align="center">
        <template #default="{ row }">
          <el-progress
            :percentage="usagePct(row)"
            :status="usageStatus(usagePct(row))"
            :stroke-width="16"
            :text-inside="true"
          />
        </template>
      </el-table-column>
    </el-table>

    <!-- Empty State -->
    <el-empty
      v-if="!tenantId && !loading"
      description="选择租户查看配额"
    />
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { get } from '../api/index.js'
import { getLimits } from '../api/traffic.js'

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------
const tenants = ref([])
const limits = ref([])
const tenantId = ref(0)
const serviceName = ref('')
const loading = ref(false)
let inputTimer = null

// ---------------------------------------------------------------------------
// Computed
// ---------------------------------------------------------------------------
const summary = computed(() => {
  const total = limits.value.length
  const critical = limits.value.filter(
    (l) => l.max > 0 && l.used / l.max > 0.8
  ).length
  const warning = limits.value.filter(
    (l) => l.max > 0 && l.used / l.max > 0.6 && l.used / l.max <= 0.8
  ).length
  return { total, critical, warning }
})

// ---------------------------------------------------------------------------
// Formatting helpers
// ---------------------------------------------------------------------------
function used(val) {
  if (val == null) return '-'
  return Number(val).toLocaleString()
}

function max(val) {
  if (val == null) return '-'
  if (val === 0) return '0'
  return Number(val).toLocaleString()
}

function available(row) {
  if (row.max == null) return '-'
  if (row.max === 0) return '0'
  const avail = Math.max(0, row.max - row.used)
  return Number(avail).toLocaleString()
}

function usagePct(row) {
  if (!row.max || row.max === 0) return 0
  return Math.round((row.used / row.max) * 100)
}

function usageStatus(pct) {
  if (pct > 80) return 'exception'
  if (pct > 60) return 'warning'
  return ''
}

// ---------------------------------------------------------------------------
// Data loading
// ---------------------------------------------------------------------------
async function loadTenants() {
  try {
    const res = await get('/tenants')
    tenants.value = res.data || []
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to load tenants')
  }
}

async function loadLimits() {
  if (!tenantId.value) {
    limits.value = []
    return
  }
  loading.value = true
  try {
    const payload = { tenant_id: tenantId.value }
    if (serviceName.value) {
      payload.service_name = serviceName.value
    }
    const res = await getLimits(payload)
    limits.value = Array.isArray(res) ? res : res.data || []
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '无法加载配额')
    limits.value = []
  } finally {
    loading.value = false
  }
}

function onServiceInput() {
  clearTimeout(inputTimer)
  inputTimer = setTimeout(() => {
    loadLimits()
  }, 300)
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------
onMounted(() => {
  loadTenants()
})
</script>

<style scoped>
.limits-page {
  padding: 20px;
}

.page-header {
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
}

.summary-row {
  display: flex;
  gap: 16px;
  margin-bottom: 20px;
}

.summary-card {
  flex: 1;
  text-align: center;
}

.summary-card .summary-value {
  font-size: 28px;
  font-weight: 700;
  line-height: 1.2;
}

.summary-card .summary-label {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  margin-top: 4px;
}

.summary-card.critical .summary-value {
  color: var(--el-color-danger);
}

.summary-card.warning .summary-value {
  color: var(--el-color-warning);
}
</style>
