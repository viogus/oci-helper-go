<template>
  <div>
    <h3>{{ $t('traffic.title') }}</h3>

    <!-- Tenant selector -->
    <el-select v-model="tenantId" @change="loadCondition" :placeholder="$t('traffic.selectTenant')" style="width:200px;margin-bottom:8px">
      <el-option v-for="t in tenants" :key="t.id" :label="t.name" :value="t.id" />
    </el-select>

    <!-- Tab: query mode vs monthly summary -->
    <el-tabs v-model="activeTab" style="margin-top:8px">
      <el-tab-pane :label="$t('traffic.liveQuery')" name="live">
        <el-card>
          <!-- Region → Instance → VNIC cascade -->
          <el-form :inline="true">
            <el-form-item :label="$t('traffic.region')">
              <el-select v-model="selectedRegion" :placeholder="$t('traffic.selectRegion')" @change="onRegionChange" style="width:200px">
                <el-option v-for="r in regionOptions" :key="r.value" :label="r.label" :value="r.value" />
              </el-select>
            </el-form-item>
            <el-form-item :label="$t('traffic.instance')">
              <el-select v-model="selectedInstance" :placeholder="$t('traffic.selectInstance')" @change="onInstanceChange" :disabled="!selectedRegion" style="width:280px">
                <el-option v-for="inst in (instanceOptions[selectedRegion] || [])" :key="inst.value" :label="inst.label" :value="inst.value" />
              </el-select>
            </el-form-item>
            <el-form-item :label="$t('traffic.vnic')">
              <el-select v-model="selectedVnic" :placeholder="$t('traffic.selectVnic')" :disabled="!selectedInstance" style="width:280px">
                <el-option v-for="v in vnicOptions" :key="v.value" :label="v.label" :value="v.value" />
              </el-select>
            </el-form-item>
          </el-form>

          <el-form :inline="true">
            <el-form-item :label="$t('traffic.timeRange')">
              <el-date-picker v-model="timeRange" type="datetimerange" :range-separator="$t('traffic.to')" :start-placeholder="$t('traffic.start')" :end-placeholder="$t('traffic.end')" value-format="YYYY-MM-DDTHH:mm:ss" style="width:380px" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="loadTraffic" :loading="loading">{{ $t('traffic.query') }}</el-button>
              <el-button @click="resetTimeRange">{{ $t('traffic.reset') }}</el-button>
            </el-form-item>
          </el-form>

          <!-- Chart -->
          <div v-if="trafficData && trafficData.length > 0" ref="trafficChart" class="chart-box"></div>
          <el-empty v-if="!loading && trafficData && trafficData.length === 0 && selectedVnic" :description="$t('traffic.noData')" />
        </el-card>
      </el-tab-pane>

      <el-tab-pane :label="$t('traffic.monthlySummary')" name="summary">
        <el-card>
          <el-form :inline="true">
            <el-form-item :label="$t('traffic.region')">
              <el-select v-model="summaryRegion" :placeholder="$t('traffic.selectRegion')" style="width:200px">
                <el-option v-for="r in regionOptions" :key="r.value" :label="r.label" :value="r.value" />
              </el-select>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="loadSummary" :loading="summaryLoading">{{ $t('traffic.query') }}</el-button>
            </el-form-item>
          </el-form>

          <!-- Summary cards -->
          <div v-if="summaryResult" class="summary-cards">
            <div class="cost-card total">
              <div class="cost-value">{{ summaryResult.instanceCount }}</div>
              <div class="cost-label">{{ $t('traffic.instanceCount') }}</div>
            </div>
            <div class="cost-card inbound">
              <div class="cost-value">{{ summaryResult.inboundTraffic }}</div>
              <div class="cost-label">{{ $t('traffic.inboundTotal') }}</div>
            </div>
            <div class="cost-card outbound">
              <div class="cost-value">{{ summaryResult.outboundTraffic }}</div>
              <div class="cost-label">{{ $t('traffic.outboundTotal') }}</div>
            </div>
          </div>
          <el-empty v-if="!summaryLoading && !summaryResult" :description="$t('traffic.selectRegionPrompt')" />
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon style="margin-top:12px" />
  </div>
</template>

<script setup>
import { ref, onMounted, nextTick } from 'vue'
import { get, post } from '../api/index.js'
import { listTenants } from '../api/tenants.js'
import { use, init } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

use([LineChart, GridComponent, TooltipComponent, CanvasRenderer])

const activeTab = ref('live')

// ── Tenant ──
const tenants = ref([])
const tenantId = ref(null)

// ── Cascade data ──
const regionOptions = ref([])
const instanceOptions = ref({})
const vnicOptions = ref([])

const selectedRegion = ref('')
const selectedInstance = ref('')
const selectedVnic = ref('')

// ── Time range ──
const timeRange = ref([])
function initTimeRange() {
  const now = new Date()
  const oneHourAgo = new Date(now.getTime() - 3600000)
  timeRange.value = [oneHourAgo.toISOString().slice(0, 19), now.toISOString().slice(0, 19)]
}
function resetTimeRange() { initTimeRange() }

// ── Live query ──
const trafficData = ref(null)
const loading = ref(false)
const error = ref('')
const trafficChart = ref(null)
let chart = null

// ── Monthly summary ──
const summaryRegion = ref('')
const summaryResult = ref(null)
const summaryLoading = ref(false)

// Load tenants then cascade on mount
onMounted(async () => {
  initTimeRange()
  try {
    const tRes = await listTenants()
    tenants.value = tRes?.data || []
    if (tenants.value.length > 0) {
      tenantId.value = tenants.value[0].id
      loadCondition()
    }
  } catch {}
})

async function loadCondition() {
  if (!tenantId.value) return
  try {
    const res = await get('/traffic/getCondition', { tenant_id: tenantId.value })
    regionOptions.value = res?.regionOptions || []
    instanceOptions.value = res?.instanceOptions || {}
  } catch {}
}

// Region changed → clear downstream
async function onRegionChange() {
  selectedInstance.value = ''
  selectedVnic.value = ''
  vnicOptions.value = []
}

// Instance changed → fetch VNICs
async function onInstanceChange() {
  selectedVnic.value = ''
  vnicOptions.value = []
  if (!selectedRegion.value || !selectedInstance.value) return
  try {
    const res = await get('/traffic/fetchVnics', { tenant_id: tenantId.value, instance_id: selectedInstance.value, region: selectedRegion.value })
    vnicOptions.value = (Array.isArray(res) ? res : [])
    if (vnicOptions.value.length === 1) selectedVnic.value = vnicOptions.value[0].value
  } catch {}
}

// Query traffic
async function loadTraffic() {
  if (!selectedVnic.value || !timeRange.value || timeRange.value.length !== 2) return
  loading.value = true
  error.value = ''
  try {
    const res = await post('/traffic', {
      tenant_id: tenantId.value,
      region: selectedRegion.value,
      vnic_id: selectedVnic.value,
      start_time: new Date(timeRange.value[0]).toISOString(),
      end_time: new Date(timeRange.value[1]).toISOString(),
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
  const bytesIn = trafficData.value.map(d => formatBytesForChart(d.bytesInPerSec || 0))
  const bytesOut = trafficData.value.map(d => formatBytesForChart(d.bytesOutPerSec || 0))

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

function formatBytesForChart(bps) {
  if (bps >= 1e9) return parseFloat((bps / 1e9).toFixed(2))
  if (bps >= 1e6) return parseFloat((bps / 1e6).toFixed(2))
  if (bps >= 1e3) return parseFloat((bps / 1e3).toFixed(2))
  return parseFloat(bps.toFixed(2))
}

// Monthly summary
async function loadSummary() {
  if (!summaryRegion.value) return
  summaryLoading.value = true
  try {
    const res = await get('/traffic/fetchInstances', { tenant_id: tenantId.value, region: summaryRegion.value })
    summaryResult.value = res || null
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to load summary'
  }
  summaryLoading.value = false
}
</script>

<style scoped>
.chart-box { width:100%; height:380px; margin-top:12px }
.summary-cards { display:flex; gap:16px; margin-top:16px; flex-wrap:wrap }
.cost-card {
  flex:1; background:var(--card-bg); border-radius:8px; padding:16px 20px;
  box-shadow:var(--shadow-sm); max-width:240px; min-width:160px
}
.cost-card.total { border-left:3px solid #2563eb }
.cost-card.inbound { border-left:3px solid #67C23A }
.cost-card.outbound { border-left:3px solid #F56C6C }
.cost-value { font-size:22px; font-weight:700; color:var(--text-primary) }
.cost-label { font-size:12px; color:var(--text-muted); margin-top:4px }
</style>
