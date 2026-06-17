<template>
  <div class="traffic-page">
    <!-- Filter Bar -->
    <div class="filter-bar">
      <el-select
        v-model="tenantId"
        placeholder="选择租户"
        @change="onTenantChange"
        style="width: 200px"
      >
        <el-option
          v-for="t in tenants"
          :key="t.id"
          :label="t.name"
          :value="t.id"
        />
      </el-select>

      <el-select
        v-model="instanceId"
        placeholder="选择实例"
        :disabled="!tenantId"
        style="width: 320px"
        filterable
      >
        <el-option
          v-for="inst in instances"
          :key="inst.id"
          :label="inst.name"
          :value="inst.id"
        />
      </el-select>

      <el-date-picker
        v-model="startTime"
        type="datetime"
        placeholder="开始时间"
        value-format="x"
        style="width: 200px"
      />

      <el-date-picker
        v-model="endTime"
        type="datetime"
        placeholder="结束时间"
        value-format="x"
        style="width: 200px"
      />

      <el-button
        type="primary"
        :loading="loading"
        :disabled="!instanceId"
        @click="handleQuery"
      >
        Query
      </el-button>
    </div>

    <!-- Error State -->
    <el-alert
      v-if="error"
      :title="error"
      type="error"
      :closable="true"
      show-icon
      @close="error = ''"
      style="margin-bottom: 16px"
    />

    <!-- Chart -->
    <div v-if="trafficData.length > 0" class="chart-wrapper">
      <VChart :option="chartOption" class="chart" autoresize />
    </div>

    <!-- Empty State -->
    <el-empty
      v-if="!loading && !error && trafficData.length === 0"
      description="选择租户和实例，点击查询"
    />
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import VChart from 'vue-echarts'
import 'echarts'
import { getTrafficData, getInstances } from '../api/traffic.js'
import { get } from '../api/index.js'

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------
const tenants = ref([])
const instances = ref([])
const tenantId = ref(0)
const instanceId = ref('')
const trafficData = ref([])
const startTime = ref(Date.now() - 3600000) // 1 hour ago (milliseconds)
const endTime = ref(Date.now())
const loading = ref(false)
const error = ref('')

// ---------------------------------------------------------------------------
// Chart option
// ---------------------------------------------------------------------------
const chartOption = computed(() => ({
  title: { text: '流量统计' },
  tooltip: { trigger: 'axis' },
  legend: { data: ['Bytes In', 'Bytes Out', 'Packets In', 'Packets Out'] },
  grid: { left: 60, right: 60, bottom: 40, top: 60 },
  xAxis: {
    type: 'category',
    data: trafficData.value.map((d) => d.timestamp?.slice(11, 16) || '')
  },
  yAxis: [
    { type: 'value', name: 'Bytes/s' },
    { type: 'value', name: 'Packets/s' }
  ],
  series: [
    {
      name: '入站字节',
      type: 'line',
      data: trafficData.value.map((d) => d.bytesInPerSec || 0),
      smooth: true,
      itemStyle: { color: '#5470c6' }
    },
    {
      name: '出站字节',
      type: 'line',
      data: trafficData.value.map((d) => d.bytesOutPerSec || 0),
      smooth: true,
      itemStyle: { color: '#91cc75' }
    },
    {
      name: '入站包数',
      type: 'line',
      yAxisIndex: 1,
      data: trafficData.value.map((d) => d.packetsInPerSec || 0),
      smooth: true,
      itemStyle: { color: '#fac858' }
    },
    {
      name: '出站包数',
      type: 'line',
      yAxisIndex: 1,
      data: trafficData.value.map((d) => d.packetsOutPerSec || 0),
      smooth: true,
      itemStyle: { color: '#ee6666' }
    }
  ]
}))

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

async function onTenantChange(val) {
  instanceId.value = ''
  trafficData.value = []
  error.value = ''
  if (!val) {
    instances.value = []
    return
  }
  try {
    const res = await getInstances(val)
    instances.value = res.data || []
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to load instances: ' + msg)
  }
}

async function handleQuery() {
  if (!tenantId.value || !instanceId.value) return

  loading.value = true
  error.value = ''
  trafficData.value = []

  try {
    const start = new Date(startTime.value).toISOString()
    const end = new Date(endTime.value).toISOString()

    const res = await getTrafficData({
      tenant_id: tenantId.value,
      instance_id: instanceId.value,
      start_time: start,
      end_time: end
    })

    trafficData.value = res.data || []
    if (trafficData.value.length === 0) {
      ElMessage.info('No traffic data available for the selected time range')
    }
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    error.value = 'Failed to query traffic data: ' + msg
    ElMessage.error(error.value)
  } finally {
    loading.value = false
  }
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------
onMounted(() => {
  loadTenants()
})
</script>

<style scoped>
.traffic-page {
  padding: 20px;
}

/* ── Filter bar ───────────────────────────────────────────────────────── */
.filter-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  align-items: center;
  flex-wrap: wrap;
}

/* ── Chart ────────────────────────────────────────────────────────────── */
.chart-wrapper {
  border: 1px solid #e4e7ed;
  border-radius: 6px;
  padding: 16px;
  background: #fff;
}

.dark .chart-wrapper {
  border-color: #363636;
  background: #1d1d1d;
}

.chart {
  width: 100%;
  height: 400px;
}
</style>
