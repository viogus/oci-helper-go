<template>
  <div>
    <h3>{{ $t('traffic.title') }}</h3>

    <!-- Shared scope: one account, one region and one time range drive all three tabs -->
    <el-card class="query-bar">
      <el-form :inline="true">
        <el-form-item :label="$t('traffic.account')">
          <el-select v-model="accountId" style="width:200px" @change="onAccountChange">
            <el-option :label="$t('traffic.allAccounts')" value="all" />
            <el-option v-for="t in tenants" :key="t.id" :label="t.name" :value="t.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('traffic.region')">
          <el-select v-model="region" :placeholder="$t('traffic.selectRegion')" :disabled="!singleAccount" style="width:200px" @change="onRegionChange">
            <el-option :label="$t('traffic.allRegions')" value="" />
            <el-option v-for="r in regionOptions" :key="r.value" :label="r.label" :value="r.value" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('traffic.timeRange')">
          <el-date-picker v-model="range" type="datetimerange" :range-separator="$t('traffic.to')" :start-placeholder="$t('traffic.start')" :end-placeholder="$t('traffic.end')" value-format="YYYY-MM-DDTHH:mm:ss" style="width:380px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="activeLoading" @click="runQuery">{{ $t('traffic.query') }}</el-button>
          <el-button :disabled="activeLoading" @click="resetQuery">{{ $t('traffic.reset') }}</el-button>
        </el-form-item>
      </el-form>
      <div class="query-hint">{{ $t('traffic.scopeHint') }}</div>
    </el-card>

    <el-tabs v-model="activeTab" style="margin-top:8px">
      <!-- Live query: one VNIC over time -->
      <el-tab-pane :label="$t('traffic.liveQuery')" name="live">
        <el-card>
          <el-form :inline="true">
            <el-form-item :label="$t('traffic.instance')">
              <el-select v-model="selectedInstance" :placeholder="$t('traffic.selectInstance')" :disabled="!scopeReady" style="width:280px" @change="onInstanceChange">
                <el-option v-for="inst in instanceList" :key="inst.value" :label="inst.label" :value="inst.value" />
              </el-select>
            </el-form-item>
            <el-form-item :label="$t('traffic.vnic')">
              <el-select v-model="selectedVnic" :placeholder="$t('traffic.selectVnic')" :disabled="!selectedInstance" style="width:280px">
                <el-option v-for="v in vnicOptions" :key="v.value" :label="v.label" :value="v.value" />
              </el-select>
            </el-form-item>
          </el-form>

          <div v-if="!scopeReady" class="tab-hint">{{ $t(scopeHintText) }}</div>
          <template v-else>
            <div v-if="trafficData && trafficData.length > 0" ref="trafficChart" class="chart-box"></div>
            <el-empty v-else-if="!loading" :description="$t('traffic.noData')" />
          </template>
        </el-card>
      </el-tab-pane>

      <!-- Monthly summary: one account over the shared window -->
      <el-tab-pane :label="$t('traffic.monthlySummary')" name="summary">
        <el-card>
          <div v-if="!singleAccount" class="tab-hint">{{ $t('traffic.needSingleAccount') }}</div>
          <template v-else>
            <div v-if="summaryRow" class="summary-cards">
              <div class="cost-card total">
                <div class="cost-value">{{ summaryRow.instanceCount || 0 }}</div>
                <div class="cost-label">{{ $t('traffic.instanceCount') }}</div>
              </div>
              <div class="cost-card inbound">
                <div class="cost-value">{{ formatBytes(summaryRow.inboundBytes) }}</div>
                <div class="cost-label">{{ $t('traffic.inboundTotal') }}</div>
              </div>
              <div class="cost-card outbound">
                <div class="cost-value">{{ formatBytes(summaryRow.outboundBytes) }}</div>
                <div class="cost-label">{{ $t('traffic.outboundTotal') }}</div>
              </div>
              <div class="cost-card quota" :class="{ danger: summaryRow.exceeded, warning: summaryRow.partial && !summaryRow.exceeded }">
                <div class="cost-value">{{ quotaPercentText(summaryRow) }}</div>
                <div class="cost-label">{{ $t('traffic.quotaUsage') }} · {{ formatBytes(summaryRow.quotaBytes) }}</div>
              </div>
            </div>
            <TrafficStatsTable :rows="summaryRow ? [summaryRow] : []" :loading="summaryLoading" :show-account="false" :empty-text="$t('traffic.selectRegionPrompt')" />
          </template>
        </el-card>
      </el-tab-pane>

      <!-- Account stats: free-allowance overview across accounts -->
      <el-tab-pane :label="$t('traffic.accountStats')" name="accounts">
        <el-card>
          <div class="stats-hint">{{ $t('traffic.statsHint') }}</div>

          <div v-if="statsLoading" class="stats-progress">
            {{ $t('traffic.statsQuerying', { done: statsProgress, total: statsTotal }) }}
          </div>

          <el-alert v-if="exceededCount > 0" :title="$t('traffic.quotaExceededAlert', { count: exceededCount })" type="error" :closable="false" show-icon style="margin-bottom:12px" />

          <TrafficStatsTable :rows="statsRows" :loading="statsLoading" :empty-text="$t('traffic.statsPrompt')" />
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon style="margin-top:12px" />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, nextTick } from 'vue'
import { get, post } from '../api/index.js'
import { getAccountStats } from '../api/traffic.js'
import { listTenants } from '../api/tenants.js'
import { formatBytes, quotaPercentText } from '../utils/format.js'
import { use, init } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import TrafficStatsTable from '../components/TrafficStatsTable.vue'

use([LineChart, GridComponent, TooltipComponent, CanvasRenderer])

// Two accounts in flight: each account already fans out across its own regions server-side.
const ACCOUNT_BATCH = 2

const activeTab = ref('live')

// ── Shared scope ──
const tenants = ref([])
const accountId = ref('all') // 'all' aggregates every account; the live/summary views need one
const region = ref('') // '' means all regions for the aggregate views
const range = ref([])
const regionOptions = ref([])
const instanceOptions = ref({})
const vnicOptions = ref([])
const selectedInstance = ref('')
const selectedVnic = ref('')
const error = ref('')

const singleAccount = computed(() => accountId.value !== 'all' && accountId.value != null)
const currentTenant = computed(() => tenants.value.find(t => t.id === accountId.value) || null)
const instanceList = computed(() => instanceOptions.value[region.value] || [])
const scopeReady = computed(() => singleAccount.value && !!region.value)
const scopeHintText = computed(() => (singleAccount.value ? 'traffic.needRegion' : 'traffic.needSingleAccount'))

// ── Live query ──
const trafficData = ref(null)
const loading = ref(false)
const trafficChart = ref(null)
let chart = null

// ── Monthly summary ──
const summaryRow = ref(null)
const summaryLoading = ref(false)

// ── Account stats ──
const statsRows = ref([])
const statsLoading = ref(false)
const statsProgress = ref(0)
const statsTotal = ref(0)
const exceededCount = computed(() => statsRows.value.filter(r => r.exceeded).length)

const activeLoading = computed(() => {
  if (activeTab.value === 'live') return loading.value
  if (activeTab.value === 'summary') return summaryLoading.value
  return statsLoading.value
})

// Local wall-clock string for the picker (value-format has no timezone suffix).
function toLocalDT(d) {
  const p = n => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

// OCI bills the free egress allowance per calendar month, so every tab defaults to it.
function initRange() {
  const now = new Date()
  range.value = [toLocalDT(new Date(now.getFullYear(), now.getMonth(), 1, 0, 0, 0)), toLocalDT(now)]
}

onMounted(async () => {
  initRange()
  try {
    const tRes = await listTenants()
    tenants.value = tRes?.data || []
  } catch {}
})

async function loadCondition() {
  if (!singleAccount.value) return
  try {
    const res = await get('/traffic/getCondition', { tenant_id: accountId.value })
    regionOptions.value = res?.regionOptions || []
    instanceOptions.value = res?.instanceOptions || {}
  } catch {}
}

function resetCascade() {
  selectedInstance.value = ''
  selectedVnic.value = ''
  vnicOptions.value = []
}

// Scope changed → previous results no longer describe the current query.
function clearResults() {
  trafficData.value = null
  summaryRow.value = null
  statsRows.value = []
  statsProgress.value = 0
  statsTotal.value = 0
  error.value = ''
  if (chart) { chart.dispose(); chart = null }
}

function onAccountChange() {
  region.value = ''
  regionOptions.value = []
  resetCascade()
  clearResults()
  if (singleAccount.value) loadCondition()
}

function onRegionChange() {
  resetCascade()
  clearResults()
}

async function onInstanceChange() {
  selectedVnic.value = ''
  vnicOptions.value = []
  if (!region.value || !selectedInstance.value) return
  try {
    const res = await get('/traffic/fetchVnics', { tenant_id: accountId.value, instance_id: selectedInstance.value, region: region.value })
    vnicOptions.value = Array.isArray(res) ? res : []
    if (vnicOptions.value.length === 1) selectedVnic.value = vnicOptions.value[0].value
  } catch {}
}

function resetQuery() {
  initRange()
  region.value = ''
  resetCascade()
  clearResults()
}

// Both aggregate views read the same endpoint: region '' means every subscribed region.
function windowBody() {
  return {
    start_time: new Date(range.value[0]).toISOString(),
    end_time: new Date(range.value[1]).toISOString(),
  }
}

function normalizeStats(row, tenant) {
  return {
    ...row,
    tenantId: row.tenantId ?? row.tenant_id ?? tenant?.id,
    tenantName: row.tenantName || row.tenant_name || tenant?.name || '',
  }
}

function requireWindow() {
  if (!range.value || range.value.length !== 2) {
    error.value = 'Invalid time range'
    return false
  }
  return true
}

async function fetchAccountStats(list, onBatch) {
  const body = windowBody()
  const out = []
  statsProgress.value = 0
  statsTotal.value = list.length
  for (let i = 0; i < list.length; i += ACCOUNT_BATCH) {
    const batch = list.slice(i, i + ACCOUNT_BATCH)
    const got = await Promise.all(batch.map(t =>
      getAccountStats({ ...body, tenant_id: t.id, region: region.value })
        .catch(e => ({ tenant_id: t.id, tenant_name: t.name, regions: [], errors: [e.response?.data?.error || 'query failed'] }))
    ))
    out.push(...got.map((r, idx) => normalizeStats(r, batch[idx])))
    statsProgress.value = Math.min(i + batch.length, list.length)
    if (onBatch) onBatch(out)
  }
  return out
}

function runQuery() {
  if (activeTab.value === 'live') return loadTraffic()
  if (activeTab.value === 'summary') return loadSummary()
  return loadAccountStats()
}

// ── Live query ──
async function loadTraffic() {
  if (!scopeReady.value || !selectedVnic.value || !requireWindow()) return
  loading.value = true
  error.value = ''
  try {
    const res = await post('/traffic', {
      tenant_id: accountId.value,
      region: region.value,
      vnic_id: selectedVnic.value,
      ...windowBody(),
    })
    trafficData.value = res?.data || []
    await nextTick()
    renderChart()
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to load traffic data'
  }
  loading.value = false
}

function renderChart() {
  if (!trafficData.value?.length) return
  if (chart) { chart.dispose(); chart = null }
  if (!trafficChart.value) return

  chart = init(trafficChart.value)
  const times = trafficData.value.map(d => d.timestamp)
  const bytesIn = trafficData.value.map(d => scaleBps(d.bytesInPerSec || 0))
  const bytesOut = trafficData.value.map(d => scaleBps(d.bytesOutPerSec || 0))

  chart.setOption({
    tooltip: { trigger: 'axis' },
    legend: { data: ['Bytes In/s', 'Bytes Out/s'], top: 0 },
    grid: { left: 70, right: 30, top: 40, bottom: 60 },
    xAxis: { type: 'category', data: times, axisLabel: { rotate: 45, fontSize: 10 } },
    yAxis: { type: 'value', name: 'bps' },
    series: [
      { name: 'Bytes In/s', type: 'line', data: bytesIn, smooth: true, symbol: 'none', lineStyle: { color: '#67C23A' } },
      { name: 'Bytes Out/s', type: 'line', data: bytesOut, smooth: true, symbol: 'none', lineStyle: { color: '#F56C6C' } },
    ],
  })
}

function scaleBps(bps) {
  if (bps >= 1e9) return parseFloat((bps / 1e9).toFixed(2))
  if (bps >= 1e6) return parseFloat((bps / 1e6).toFixed(2))
  if (bps >= 1e3) return parseFloat((bps / 1e3).toFixed(2))
  return parseFloat(bps.toFixed(2))
}

// ── Monthly summary ──
async function loadSummary() {
  const tenant = currentTenant.value
  if (!tenant || !requireWindow()) return
  summaryLoading.value = true
  error.value = ''
  try {
    const rows = await fetchAccountStats([tenant])
    summaryRow.value = rows[0] || null
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to load summary'
  }
  summaryLoading.value = false
}

// ── Account stats ──
async function loadAccountStats() {
  const list = singleAccount.value ? tenants.value.filter(t => t.id === accountId.value) : tenants.value
  if (!list.length || !requireWindow()) return

  statsLoading.value = true
  statsRows.value = []
  error.value = ''
  const byOutbound = rows => [...rows].sort((a, b) => (b.outboundBytes || 0) - (a.outboundBytes || 0))
  try {
    await fetchAccountStats(list, rows => { statsRows.value = byOutbound(rows) })
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to load account stats'
  }
  statsLoading.value = false
}
</script>

<style scoped>
.query-bar { margin-bottom: 8px }
.query-bar :deep(.el-form-item) { margin-bottom: 8px }
.query-hint { font-size: 12px; color: var(--text-muted) }
.tab-hint { font-size: 13px; color: var(--text-muted); padding: 8px 0 }
.chart-box { width:100%; height:380px; margin-top:12px }
.summary-cards { display:flex; gap:16px; margin-top:16px; flex-wrap:wrap }
.cost-card {
  flex:1; background:var(--card-bg); border-radius:8px; padding:16px 20px;
  box-shadow:var(--shadow-sm); max-width:240px; min-width:160px
}
.cost-card.total { border-left:3px solid #2563eb }
.cost-card.inbound { border-left:3px solid #67C23A }
.cost-card.outbound { border-left:3px solid #F56C6C }
.cost-card.quota { border-left:3px solid #e6a23c }
.cost-card.quota.warning { border-left-color:#e6a23c }
.cost-card.quota.danger { border-left-color:#F56C6C }
.cost-value { font-size:22px; font-weight:700; color:var(--text-primary) }
.cost-label { font-size:12px; color:var(--text-muted); margin-top:4px }
.stats-hint { font-size:12px; color:var(--text-muted); margin-bottom:8px }
.stats-progress { font-size:13px; color:var(--text-muted); margin-bottom:8px }
</style>
