<template>
  <div class="sysmetrics-page">
    <div class="sysmetrics-toolbar">
      <h2 class="sysmetrics-title">{{ $t('sysMetrics.title') }}</h2>
      <div class="sysmetrics-toolbar-right">
        <el-switch
          v-model="autoRefresh"
          active-text="Auto refresh"
          inactive-text="Auto refresh"
          @change="onAutoRefreshChange"
        />
        <el-button :loading="loading" @click="load">{{ $t('sysMetrics.refresh') }}</el-button>
      </div>
    </div>

    <div v-if="error" class="sysmetrics-error">
      <el-alert :title="error" type="error" :closable="false" show-icon />
    </div>

    <template v-else-if="data">
      <!-- Host info -->
      <el-card class="sysmetrics-card" shadow="never">
        <template #header>
          <span>{{ $t('sysMetrics.hostInfo') }}</span>
        </template>
        <el-descriptions :column="3" border>
          <el-descriptions-item :label="$t('sysMetrics.hostname')">{{ data.hostname || '—' }}</el-descriptions-item>
          <el-descriptions-item :label="$t('sysMetrics.os')">{{ data.os || '—' }}</el-descriptions-item>
          <el-descriptions-item :label="$t('sysMetrics.platform')">{{ data.platform || '—' }}</el-descriptions-item>
          <el-descriptions-item :label="$t('sysMetrics.arch')">{{ data.arch || '—' }}</el-descriptions-item>
          <el-descriptions-item :label="$t('sysMetrics.uptime')">{{ formatUptime(data.uptime_seconds) }}</el-descriptions-item>
          <el-descriptions-item :label="$t('sysMetrics.cpuCount')">{{ data.cpu_count || '—' }}</el-descriptions-item>
        </el-descriptions>
        <div v-if="data.cpu_model" class="sysmetrics-cpumodel">
          {{ data.cpu_model }}
        </div>
      </el-card>

      <!-- CPU -->
      <el-card class="sysmetrics-card" shadow="never">
        <template #header>
          <span>{{ $t('sysMetrics.cpu') }}</span>
          <span class="sysmetrics-percent">{{ data.cpu.used_percent }}%</span>
        </template>
        <el-progress
          :percentage="clampPercent(data.cpu.used_percent)"
          :stroke-width="16"
          :color="usageColor(data.cpu.used_percent)"
        />
        <div class="sysmetrics-split">
          <span>{{ $t('sysMetrics.user') }}: {{ data.cpu.user_percent }}%</span>
          <span>{{ $t('sysMetrics.system') }}: {{ data.cpu.system_percent }}%</span>
          <span>{{ $t('sysMetrics.idle') }}: {{ data.cpu.idle_percent }}%</span>
        </div>
      </el-card>

      <!-- Memory -->
      <el-card class="sysmetrics-card" shadow="never">
        <template #header>
          <span>{{ $t('sysMetrics.memory') }}</span>
          <span class="sysmetrics-percent">{{ data.memory.used_percent }}%</span>
        </template>
        <el-progress
          :percentage="clampPercent(data.memory.used_percent)"
          :stroke-width="16"
          :color="usageColor(data.memory.used_percent)"
        />
        <div class="sysmetrics-split">
          <span>{{ $t('sysMetrics.used') }}: {{ formatBytes(data.memory.used_bytes) }}</span>
          <span>{{ $t('sysMetrics.free') }}: {{ formatBytes(data.memory.available_bytes) }}</span>
          <span>{{ $t('sysMetrics.total') }}: {{ formatBytes(data.memory.total_bytes) }}</span>
        </div>
      </el-card>

      <!-- Disk -->
      <el-card class="sysmetrics-card" shadow="never">
        <template #header>
          <span>{{ $t('sysMetrics.disk') }}</span>
        </template>
        <el-empty v-if="data.disks.length === 0" :description="$t('sysMetrics.noDisk')" :image-size="60" />
        <div v-for="d in data.disks" :key="d.mount" class="sysmetrics-disk">
          <div class="sysmetrics-disk-head">
            <span class="sysmetrics-disk-mount">{{ d.mount }}</span>
            <span class="sysmetrics-disk-percent">{{ d.used_percent }}%</span>
          </div>
          <el-progress
            :percentage="clampPercent(d.used_percent)"
            :stroke-width="12"
            :color="usageColor(d.used_percent)"
          />
          <div class="sysmetrics-split">
            <span>{{ $t('sysMetrics.used') }}: {{ formatBytes(d.used_bytes) }}</span>
            <span>{{ $t('sysMetrics.free') }}: {{ formatBytes(d.free_bytes) }}</span>
            <span>{{ $t('sysMetrics.total') }}: {{ formatBytes(d.total_bytes) }}</span>
          </div>
        </div>
      </el-card>

      <!-- Network -->
      <el-card class="sysmetrics-card" shadow="never">
        <template #header>
          <span>{{ $t('sysMetrics.network') }}</span>
        </template>
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="$t('sysMetrics.txRate')">{{ data.network.tx_rate_kbps }} KB/s</el-descriptions-item>
          <el-descriptions-item :label="$t('sysMetrics.rxRate')">{{ data.network.rx_rate_kbps }} KB/s</el-descriptions-item>
          <el-descriptions-item :label="$t('sysMetrics.txTotal')">{{ formatBytes(data.network.bytes_sent) }}</el-descriptions-item>
          <el-descriptions-item :label="$t('sysMetrics.rxTotal')">{{ formatBytes(data.network.bytes_recv) }}</el-descriptions-item>
        </el-descriptions>
      </el-card>
    </template>

    <el-skeleton v-else :rows="6" animated />
  </div>
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { get } from '../api/index.js'

const data = ref(null)
const loading = ref(false)
const error = ref('')
const autoRefresh = ref(true)

let timer = null

function formatBytes(bytes) {
  if (bytes == null) return '—'
  if (bytes < 1024) return bytes + ' B'
  const units = ['KB', 'MB', 'GB', 'TB', 'PB']
  let i = -1
  let v = bytes
  do {
    v /= 1024
    i++
  } while (v >= 1024 && i < units.length - 1)
  return v.toFixed(1) + ' ' + units[i]
}

function formatUptime(secs) {
  if (secs == null) return '—'
  const d = Math.floor(secs / 86400)
  const h = Math.floor((secs % 86400) / 3600)
  const m = Math.floor((secs % 3600) / 60)
  if (d > 0) return `${d}d ${h}h ${m}m`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

function clampPercent(p) {
  if (p == null) return 0
  return Math.max(0, Math.min(100, Math.round(p)))
}

function usageColor(p) {
  if (p >= 90) return '#f56c6c'
  if (p >= 70) return '#e6a23c'
  return '#67c23a'
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await get('/system/metrics')
    data.value = res
  } catch (e) {
    error.value = e?.response?.data?.error || String(e)
  } finally {
    loading.value = false
  }
}

function onAutoRefreshChange(val) {
  clearInterval(timer)
  timer = null
  if (val) {
    timer = setInterval(load, 5000)
  }
}

onMounted(() => {
  load()
  if (autoRefresh.value) {
    timer = setInterval(load, 5000)
  }
})

onBeforeUnmount(() => {
  clearInterval(timer)
})
</script>

<style scoped>
.sysmetrics-page {
  padding: 4px;
}

.sysmetrics-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.sysmetrics-title {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}

.sysmetrics-toolbar-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.sysmetrics-card {
  margin-bottom: 16px;
}

.sysmetrics-percent {
  float: right;
  font-weight: 600;
  color: var(--el-color-primary);
}

.sysmetrics-split {
  display: flex;
  gap: 24px;
  margin-top: 8px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.sysmetrics-cpumodel {
  margin-top: 10px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.sysmetrics-disk {
  margin-bottom: 16px;
}

.sysmetrics-disk:last-child {
  margin-bottom: 0;
}

.sysmetrics-disk-head {
  display: flex;
  justify-content: space-between;
  margin-bottom: 4px;
  font-size: 13px;
}

.sysmetrics-disk-mount {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

.sysmetrics-disk-percent {
  font-weight: 600;
}

.sysmetrics-error {
  margin-bottom: 16px;
}
</style>
