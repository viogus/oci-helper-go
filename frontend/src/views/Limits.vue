<template>
  <div>
    <h3>{{ $t('limits.title') }}</h3>

    <!-- Filter bar: tenant → region → service cascade -->
    <el-card>
      <el-form :inline="true">
        <el-form-item :label="$t('limits.selectTenant')">
          <el-select v-model="tenantId" :placeholder="$t('limits.selectTenant')" @change="onTenantChange" style="width:200px">
            <el-option v-for="t in tenants" :key="t.id" :label="t.name" :value="t.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('limits.region')">
          <el-select v-model="region" :placeholder="$t('limits.selectRegion')" @change="onRegionChange" :disabled="!tenantId" style="width:200px">
            <el-option v-for="r in regionOptions" :key="r.value" :label="r.label" :value="r.value" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('limits.service')">
          <el-select v-model="serviceName" :placeholder="$t('limits.allServices')" clearable :disabled="!region" style="width:220px">
            <el-option v-for="s in serviceOptions" :key="s" :label="s" :value="s" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="loadLimits" :loading="loading" :disabled="!region">{{ $t('limits.query') }}</el-button>
        </el-form-item>
      </el-form>

      <!-- Summary cards -->
      <div v-if="items.length > 0" class="summary-row">
        <div class="cost-card total">
          <div class="cost-value">{{ total }}</div>
          <div class="cost-label">{{ $t('limits.total') }}</div>
        </div>
        <div class="cost-card critical">
          <div class="cost-value">{{ critical }}</div>
          <div class="cost-label">{{ $t('limits.critical') }} (&gt;80%)</div>
        </div>
        <div class="cost-card warning">
          <div class="cost-value">{{ warning }}</div>
          <div class="cost-label">{{ $t('limits.warning') }} (60-80%)</div>
        </div>
      </div>

      <!-- Table -->
      <el-table v-if="items.length > 0" :data="items" stripe border size="small" style="margin-top:12px" :default-sort="{prop: 'serviceName', order: 'ascending'}">
        <el-table-column prop="serviceName" :label="$t('limits.service')" width="150" sortable>
          <template #default="{ row }">
            <el-tag type="primary" effect="plain" size="small">{{ row.serviceName }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="limitName" :label="$t('limits.limitName')" min-width="200" sortable />
        <el-table-column prop="description" :label="$t('limits.description')" min-width="180" show-overflow-tooltip />
        <el-table-column prop="scopeType" :label="$t('limits.scopeType')" width="100" sortable>
          <template #default="{ row }">
            <el-tag :type="row.scopeType === 'AD' ? 'warning' : 'info'" size="small">{{ row.scopeType }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="availabilityDomain" :label="$t('limits.ad')" width="100">
          <template #default="{ row }">{{ row.availabilityDomain || '—' }}</template>
        </el-table-column>
        <el-table-column :label="$t('limits.serviceLimit')" width="120" align="right" sortable prop="serviceLimit">
          <template #default="{ row }">{{ fmt(row.serviceLimit) }}</template>
        </el-table-column>
        <el-table-column :label="$t('limits.used')" width="120" align="right" sortable prop="used">
          <template #default="{ row }">{{ fmt(row.used) }}</template>
        </el-table-column>
        <el-table-column :label="$t('limits.available')" width="120" align="right" sortable prop="available">
          <template #default="{ row }">{{ fmt(row.available) }}</template>
        </el-table-column>
        <el-table-column :label="$t('limits.usagePct')" width="180" align="center">
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

      <el-empty v-if="!loading && !tenantId" :description="$t('limits.selectTenantHint')" />
      <el-empty v-if="!loading && tenantId && !region" :description="$t('limits.selectRegionHint')" />
      <el-empty v-if="!loading && region && queried && items.length === 0" :description="$t('limits.noData')" />
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { get, post } from '../api/index.js'
import { listTenants } from '../api/tenants.js'

const tenants = ref([])
const tenantId = ref(null)
const regionOptions = ref([])
const region = ref('')
const serviceOptions = ref([])
const serviceName = ref('')
const items = ref([])
const loading = ref(false)
const queried = ref(false)

const total = computed(() => items.value.length)
const critical = computed(() => items.value.filter(l => l.serviceLimit > 0 && l.used / l.serviceLimit > 0.8).length)
const warning = computed(() => items.value.filter(l => l.serviceLimit > 0 && l.used / l.serviceLimit > 0.6 && l.used / l.serviceLimit <= 0.8).length)

function fmt(v) {
  if (v == null) return '—'
  return Number(v).toLocaleString()
}

function usagePct(row) {
  if (!row.serviceLimit || row.serviceLimit === 0) return 0
  return Math.round((row.used / row.serviceLimit) * 100)
}

function usageStatus(pct) {
  if (pct > 80) return 'exception'
  if (pct > 60) return 'warning'
  return ''
}

onMounted(async () => {
  try {
    const res = await listTenants()
    tenants.value = res?.data || []
  } catch {}
})

async function onTenantChange() {
  region.value = ''
  serviceOptions.value = []
  serviceName.value = ''
  items.value = []
  if (!tenantId.value) return
  try {
    const res = await get('/traffic/getCondition', { tenant_id: tenantId.value })
    regionOptions.value = res?.regionOptions || []
  } catch (e) {
    ElMessage.error('Failed to load regions')
  }
}

async function onRegionChange() {
  serviceName.value = ''
  items.value = []
  if (!region.value || !tenantId.value) return
  try {
    const res = await get('/limits/services', { tenant_id: tenantId.value, region: region.value })
    serviceOptions.value = Array.isArray(res) ? res : []
  } catch (e) {
    ElMessage.error('Failed to load services')
  }
}

async function loadLimits() {
  if (!tenantId.value || !region.value) return
  loading.value = true
  queried.value = true
  try {
    const res = await post('/limits', {
      tenant_id: tenantId.value,
      region: region.value,
      service_name: serviceName.value || '',
    })
    items.value = res?.items || []
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to load limits')
    items.value = []
  }
  loading.value = false
}
</script>

<style scoped>
.summary-row { display:flex; gap:16px; margin-top:12px; flex-wrap:wrap }
.cost-card {
  flex:1; background:var(--card-bg); border-radius:8px; padding:16px 20px;
  box-shadow:var(--shadow-sm); max-width:200px; min-width:140px; text-align:center
}
.cost-card.total { border-left:3px solid #2563eb }
.cost-card.critical { border-left:3px solid #F56C6C }
.cost-card.warning { border-left:3px solid #E6A23C }
.cost-value { font-size:22px; font-weight:700; color:var(--text-primary) }
.cost-label { font-size:12px; color:var(--text-muted); margin-top:4px }
</style>
