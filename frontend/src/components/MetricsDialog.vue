<template>
  <el-dialog
    v-model="show"
    title="Instance Metrics"
    width="700px"
    :close-on-click-modal="false"
    class="metrics-dialog"
    @closed="onClose"
    destroy-on-close
  >
    <!-- Header -->
    <div class="metrics-dialog__header">
      <div class="metrics-dialog__title">
        Metrics for <strong>{{ instance?.name }}</strong>
        <span class="metrics-dialog__shape">{{ instance?.shape }}</span>
      </div>
      <div class="metrics-dialog__toolbar">
        <span v-if="lastUpdated" class="metrics-dialog__updated">
          <el-icon><Clock /></el-icon>
          {{ lastUpdated }}
        </span>
        <el-button
          size="small"
          :icon="Refresh"
          :loading="loading"
          @click="refresh"
          circle
        />
      </div>
    </div>

    <!-- Loading skeleton -->
    <div v-if="loading" class="metrics-dialog__skeleton">
      <div class="sk-card">
        <div class="sk-card__title" />
        <div class="sk-card__chart" />
      </div>
      <div class="sk-card">
        <div class="sk-card__title" />
        <div class="sk-card__chart" />
      </div>
      <div class="sk-card sk-card--wide">
        <div class="sk-card__title" />
        <div class="sk-card__chart" />
      </div>
      <div class="sk-card sk-card--wide">
        <div class="sk-card__title" />
        <div class="sk-card__chart" />
      </div>
    </div>

    <!-- Error -->
    <el-alert
      v-else-if="error"
      :title="error"
      type="error"
      :closable="false"
      show-icon
    >
      <template #default>
        <p class="metrics-dialog__error-hint">
          Failed to load metrics.
          <el-button size="small" type="primary" @click="refresh">Retry</el-button>
        </p>
      </template>
    </el-alert>

    <!-- Charts -->
    <div v-else-if="metrics" class="metrics-dialog__grid">
      <div class="metrics-dialog__card">
        <h4 class="metrics-dialog__card-title">CPU Utilization</h4>
        <div class="metrics-dialog__card-body">
          <template v-if="metrics.cpu?.error">
            <el-alert :title="metrics.cpu.error" type="warning" :closable="false" show-icon />
          </template>
          <template v-else-if="metrics.cpu?.value != null">
            <VChart :option="cpuGauge" class="gauge-chart" autoresize />
          </template>
          <el-empty v-else description="No data" :image-size="60" />
        </div>
      </div>
      <div class="metrics-dialog__card">
        <h4 class="metrics-dialog__card-title">Memory Utilization</h4>
        <div class="metrics-dialog__card-body">
          <template v-if="metrics.memory?.error">
            <el-alert :title="metrics.memory.error" type="warning" :closable="false" show-icon />
          </template>
          <template v-else-if="metrics.memory?.value != null">
            <VChart :option="memGauge" class="gauge-chart" autoresize />
          </template>
          <el-empty v-else description="No data" :image-size="60" />
        </div>
      </div>
      <div class="metrics-dialog__card metrics-dialog__card--wide">
        <h4 class="metrics-dialog__card-title">Network I/O</h4>
        <div class="metrics-dialog__card-body">
          <VChart :option="networkBar" class="bar-chart" autoresize />
        </div>
      </div>
      <div class="metrics-dialog__card metrics-dialog__card--wide">
        <h4 class="metrics-dialog__card-title">Disk I/O</h4>
        <div class="metrics-dialog__card-body">
          <VChart :option="diskBar" class="bar-chart" autoresize />
        </div>
      </div>
    </div>

    <el-empty v-else description="No metrics data available" :image-size="80" />

    <template #footer>
      <el-button @click="show = false">Close</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import VChart from 'vue-echarts'
import 'echarts'
import { Clock, Refresh } from '@element-plus/icons-vue'
import { getMetrics } from '../api/metrics.js'

const props = defineProps({
  visible: Boolean,
  instance: Object
})

const emit = defineEmits(['update:visible'])

const show = computed({
  get: () => props.visible,
  set: (v) => emit('update:visible', v)
})

const metrics = ref(null)
const loading = ref(false)
const error = ref('')
const lastUpdated = ref('')

function fmtBytes(v) {
  if (!v || v === 0) return '0 B/s'
  if (v >= 1e9) return (v / 1e9).toFixed(1) + ' GB/s'
  if (v >= 1e6) return (v / 1e6).toFixed(1) + ' MB/s'
  if (v >= 1e3) return (v / 1e3).toFixed(1) + ' KB/s'
  return v.toFixed(0) + ' B/s'
}

function createGaugeOption(value) {
  return {
    series: [
      {
        type: 'gauge',
        startAngle: 220,
        endAngle: -40,
        min: 0,
        max: 100,
        splitNumber: 5,
        progress: { show: true, width: 8 },
        axisLine: {
          lineStyle: {
            width: 10,
            color: [
              [0.5, '#67C23A'],
              [0.8, '#E6A23C'],
              [1, '#F56C6C']
            ]
          }
        },
        axisTick: { show: false },
        splitLine: { length: 6 },
        axisLabel: { fontSize: 10 },
        pointer: { width: 4, length: '60%' },
        detail: {
          formatter: '{value}%',
          fontSize: 16,
          fontWeight: 600,
          offsetCenter: [0, '50%']
        },
        data: [{ value: value ?? 0 }]
      }
    ]
  }
}

const cpuGauge = computed(() => createGaugeOption(metrics.value?.cpu?.value))
const memGauge = computed(() => createGaugeOption(metrics.value?.memory?.value))

const networkBar = computed(() => {
  const d = metrics.value
  return {
    tooltip: {
      trigger: 'axis',
      formatter: (p) => p[0].name + '<br/>' + fmtBytes(p[0].value)
    },
    grid: { left: 60, right: 20, top: 10, bottom: 30 },
    xAxis: { type: 'category', data: ['In', 'Out'], axisLabel: { fontWeight: 600 } },
    yAxis: { type: 'value', axisLabel: { formatter: (v) => fmtBytes(v) } },
    series: [
      {
        type: 'bar',
        barWidth: 40,
        data: [
          { value: d?.networkIn?.value || 0, itemStyle: { color: '#5470c6' } },
          { value: d?.networkOut?.value || 0, itemStyle: { color: '#91cc75' } }
        ]
      }
    ]
  }
})

const diskBar = computed(() => {
  const d = metrics.value
  return {
    tooltip: {
      trigger: 'axis',
      formatter: (p) => p[0].name + '<br/>' + fmtBytes(p[0].value)
    },
    grid: { left: 60, right: 20, top: 10, bottom: 30 },
    xAxis: { type: 'category', data: ['Read', 'Write'], axisLabel: { fontWeight: 600 } },
    yAxis: { type: 'value', axisLabel: { formatter: (v) => fmtBytes(v) } },
    series: [
      {
        type: 'bar',
        barWidth: 40,
        data: [
          { value: d?.diskRead?.value || 0, itemStyle: { color: '#fac858' } },
          { value: d?.diskWrite?.value || 0, itemStyle: { color: '#ee6666' } }
        ]
      }
    ]
  }
})

async function refresh() {
  if (!props.instance) return
  loading.value = true
  error.value = ''
  try {
    const res = await getMetrics({
      tenant_id: props.instance.tenantId,
      instance_id: props.instance.id
    })
    metrics.value = res
    lastUpdated.value = new Date().toLocaleTimeString('zh-CN', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit'
    })
  } catch (e) {
    error.value = e.response?.data?.error || e.message
  } finally {
    loading.value = false
  }
}

function onClose() {
  metrics.value = null
  error.value = ''
  lastUpdated.value = ''
}

watch(
  () => props.visible,
  (v) => {
    if (v && !metrics.value) {
      refresh()
    }
  }
)
</script>

<style scoped>
/* ── Header ─────────────────────────────────────────────── */
.metrics-dialog__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.metrics-dialog__title {
  font-size: 14px;
  color: var(--text-secondary, #606266);
}

.metrics-dialog__shape {
  display: inline-block;
  margin-left: 8px;
  padding: 1px 8px;
  font-size: 11px;
  font-weight: 500;
  color: var(--primary, #2563eb);
  background: var(--primary-light, #dbeafe);
  border-radius: 4px;
}

.metrics-dialog__toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
}

.metrics-dialog__updated {
  font-size: 12px;
  color: var(--text-muted, #94a3b8);
  display: flex;
  align-items: center;
  gap: 4px;
  white-space: nowrap;
}

.metrics-dialog__error-hint {
  margin: 4px 0 0;
  line-height: 1.6;
}

/* ── Grid ───────────────────────────────────────────────── */
.metrics-dialog__grid {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.metrics-dialog__card {
  width: calc(50% - 6px);
  background: var(--card-bg, #fff);
  border: 1px solid var(--border-color, #ebeef5);
  border-radius: 8px;
  padding: 12px;
  box-sizing: border-box;
}

.metrics-dialog__card--wide {
  width: 100%;
}

.metrics-dialog__card-title {
  margin: 0 0 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary, #606266);
  text-align: center;
}

.metrics-dialog__card-body {
  min-height: 100px;
}

.gauge-chart {
  height: 180px;
  width: 100%;
}

.bar-chart {
  height: 200px;
  width: 100%;
}

/* ── Skeleton ────────────────────────────────────────────── */
.metrics-dialog__skeleton {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.sk-card {
  width: calc(50% - 6px);
  border: 1px solid var(--border-color, #ebeef5);
  border-radius: 8px;
  padding: 12px;
  box-sizing: border-box;
}

.sk-card--wide {
  width: 100%;
}

.sk-card__title {
  height: 14px;
  width: 60%;
  margin: 0 auto 12px;
  background: linear-gradient(90deg, var(--border-color, #eee) 25%, #e0e0e0 50%, var(--border-color, #eee) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite ease-in-out;
  border-radius: 4px;
}

.sk-card__chart {
  height: 140px;
  background: linear-gradient(90deg, var(--border-color, #eee) 25%, #e0e0e0 50%, var(--border-color, #eee) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite ease-in-out;
  border-radius: 6px;
}

.sk-card--wide .sk-card__chart {
  height: 160px;
}

@keyframes shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}

/* ── Dark mode overrides ────────────────────────────────── */
.dark .sk-card__title,
.dark .sk-card__chart {
  background: linear-gradient(90deg, #1e293b 25%, #334155 50%, #1e293b 75%);
  background-size: 200% 100%;
}

:deep(.el-dialog__body) {
  padding-top: 16px;
}
</style>
