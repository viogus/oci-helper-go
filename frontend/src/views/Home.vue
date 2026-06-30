<template>
  <div class="dashboard">
    <div class="page-header">
      <h3>{{ $t('home.dashboard') }}</h3>
    </div>

    <!-- Stat Cards -->
    <div class="stat-grid" v-loading="loading">
      <div
        v-for="stat in stats"
        :key="stat.label"
        class="stat-card"
        :style="{ '--accent': stat.color }"
      >
        <div class="stat-icon">
          <el-icon :size="24"><component :is="stat.icon" /></el-icon>
        </div>
        <div class="stat-body">
          <div class="stat-value">{{ stat.value }}</div>
          <div class="stat-label">{{ $t(stat.labelKey) }}</div>
        </div>
      </div>
    </div>

    <!-- Section: Quick Access & Info -->
    <div class="dashboard-grid">
      <!-- Quick Access -->
      <div class="dash-section">
        <h4>{{ $t('home.quickAccess') }}</h4>
        <div class="action-grid">
          <div
            v-for="link in links"
            :key="link.path"
            class="action-card"
            @click="$router.push(link.path)"
          >
            <div class="action-icon" :style="{ background: link.bg }">
              <el-icon :size="22"><component :is="link.icon" /></el-icon>
            </div>
            <div class="action-label">{{ $t(link.labelKey) }}</div>
          </div>
        </div>
      </div>

      <!-- Resources -->
      <div class="dash-section">
        <h4>{{ $t('home.resources') }}</h4>
        <div class="info-grid">
          <div class="info-item">
            <span class="info-label">{{ $t('home.region') }}</span>
            <span class="info-value">{{ regionCount }}</span>
          </div>
          <div class="info-item">
            <span class="info-label">{{ $t('home.runningInstances') }}</span>
            <span class="info-value">{{ runningCount }}</span>
          </div>
          <div class="info-item">
            <span class="info-label">{{ $t('home.activeTasks') }}</span>
            <span class="info-value">{{ activeTasks }}</span>
          </div>
          <div class="info-item">
            <span class="info-label">{{ $t('home.syncedTenants') }}</span>
            <span class="info-value">{{ syncedTenants }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- World Map -->
    <div class="dash-section map-section">
      <h4>{{ $t('home.serverMap') }}</h4>
      <div ref="mapContainer" class="map-container"></div>
      <div class="map-legend">
        <span class="legend-item"><span class="legend-dot running"></span> {{ $t('home.running') }}</span>
        <span class="legend-item"><span class="legend-dot stopped"></span> {{ $t('home.stopped') }}</span>
        <span class="legend-item"><span class="legend-dot other"></span> {{ $t('home.otherState') }}</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { get } from '../api/index.js'
import {
  Monitor, User, Timer, Plus, Lock, Connection,
  Cloudy, ChatDotRound, Setting
} from '@element-plus/icons-vue'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import ociRegions from '../data/ociRegions.js'

const loading = ref(true)
const regionCount = ref(0)
const runningCount = ref(0)
const activeTasks = ref(0)
const syncedTenants = ref(0)
const mapContainer = ref(null)

let map = null
let markerLayer = null

const stats = reactive([
  { labelKey: 'home.tenant', value: '—', icon: 'User', color: '#2563eb' },
  { labelKey: 'home.instance', value: '—', icon: 'Monitor', color: '#10b981' },
  { labelKey: 'home.running', value: '—', icon: 'Timer', color: '#f59e0b' },
  { labelKey: 'home.activeTasks', value: '—', icon: 'Timer', color: '#8b5cf6' },
])

const links = [
  { labelKey: 'home.instance', path: '/instances', icon: 'Monitor', bg: 'linear-gradient(135deg,#2563eb,#6366f1)' },
  { labelKey: 'home.create', path: '/instances/create', icon: 'Plus', bg: 'linear-gradient(135deg,#10b981,#059669)' },
  { labelKey: 'home.security', path: '/security-rules', icon: 'Lock', bg: 'linear-gradient(135deg,#f59e0b,#d97706)' },
  { labelKey: 'home.network', path: '/traffic', icon: 'Connection', bg: 'linear-gradient(135deg,#8b5cf6,#7c3aed)' },
  { labelKey: 'home.cloudflare', path: '/cloudflare', icon: 'Cloudy', bg: 'linear-gradient(135deg,#06b6d4,#0891b2)' },
  { labelKey: 'home.aiChat', path: '/ai-chat', icon: 'ChatDotRound', bg: 'linear-gradient(135deg,#ec4899,#db2777)' },
  { labelKey: 'home.tenant', path: '/tenants', icon: 'User', bg: 'linear-gradient(135deg,#64748b,#475569)' },
  { labelKey: 'home.settings', path: '/settings', icon: 'Setting', bg: 'linear-gradient(135deg,#78716c,#57534e)' },
]

// Custom Leaflet marker icons
const markerColors = {
  RUNNING: '#10b981',
  STOPPED: '#ef4444',
  STOPPING: '#f59e0b',
  STARTING: '#3b82f6',
  TERMINATED: '#6b7280',
  default: '#8b5cf6',
}
const iconCache = {}

function getIcon(state) {
  const color = markerColors[state] || markerColors.default
  if (!iconCache[color]) {
    iconCache[color] = L.divIcon({
      className: 'custom-marker',
      html: `<div style="width:12px;height:12px;border-radius:50%;background:${color};border:2px solid #fff;box-shadow:0 1px 4px rgba(0,0,0,0.3)"></div>`,
      iconSize: [12, 12],
      iconAnchor: [6, 6],
    })
  }
  return iconCache[color]
}

function initMap() {
  if (map) return
  map = L.map(mapContainer.value, {
    center: [20, 0],
    zoom: 2,
    minZoom: 2,
    maxZoom: 8,
    zoomControl: true,
    attributionControl: false,
  })

  L.tileLayer('https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png', {
    maxZoom: 19,
  }).addTo(map)

  markerLayer = L.layerGroup().addTo(map)

  // Invalidate size after container becomes visible
  setTimeout(() => map.invalidateSize(), 100)
}

function updateMarkers(tenants, instances) {
  if (!markerLayer) return
  markerLayer.clearLayers()

  // Build tenant id → region map
  const tenantMap = {}
  for (const t of tenants) {
    if (t.region) tenantMap[t.id] = t.region.toLowerCase()
  }

  // Group instances by region
  const regionGroups = {}
  for (const inst of instances) {
    const region = tenantMap[inst.tenantId]
    if (!region) continue
    const coords = ociRegions[region]
    if (!coords) continue

    const key = region
    if (!regionGroups[key]) {
      regionGroups[key] = { coords, region, count: 0, running: 0, instances: [] }
    }
    regionGroups[key].count++
    if (inst.state === 'RUNNING') regionGroups[key].running++
    regionGroups[key].instances.push(inst)
  }

// Render city markers from IP geolocation data (glance cities).
function updateCityMarkers(cities) {
  if (!markerLayer || !cities?.length) return
  markerLayer.clearLayers()

  for (const c of cities) {
    const label = [c.city, c.area, c.country].filter(Boolean).join(', ')
      || `${c.lat.toFixed(2)}, ${c.lng.toFixed(2)}`
    const subtitle = [c.org, c.asn].filter(Boolean).join(' / ')
    const tooltip = `${label}${subtitle ? ' — ' + subtitle : ''} (${c.count})`

    const color = '#8b5cf6'
    const icon = L.divIcon({
      className: 'custom-marker',
      html: '<div style="width:12px;height:12px;border-radius:50%;background:' + color + ';border:2px solid #fff;box-shadow:0 1px 4px rgba(0,0,0,0.3)"></div>',
      iconSize: [12, 12],
      iconAnchor: [6, 6],
    })

    const marker = L.marker([c.lat, c.lng], { icon }).addTo(markerLayer)
    marker.bindTooltip(tooltip, { direction: 'top', offset: [0, -8] })
  }
}

  for (const [region, group] of Object.entries(regionGroups)) {
    // Offset markers slightly so overlapping regions are visible
    const jitter = (Object.keys(regionGroups).indexOf(region) % 5) * 0.0003
    const lat = group.coords[0] + jitter
    const lng = group.coords[1] + jitter

    const dominantState = group.running > 0 ? 'RUNNING' : (group.instances[0]?.state || 'default')
    const icon = getIcon(dominantState)

    const displayRegion = region.replace('us-', '').replace('eu-', '').replace('ap-', '').replace('me-', '').replace('sa-', '')
    const label = `${displayRegion}: ${group.count} (${group.running} running)`

    const marker = L.marker([lat, lng], { icon }).addTo(markerLayer)
    marker.bindTooltip(label, { direction: 'top', offset: [0, -8] })
  }
}

onMounted(async () => {
  let tList = [], iList = [], cities = []
  try {
    const [tenants, instances, tasks, glance] = await Promise.all([
      get('/tenants', { size: 100 }),
      get('/instances', { size: 500 }),
      get('/tasks', { size: 100 }),
      get('/glance'),
    ])
    tList = tenants?.data || []
    iList = instances?.data || []
    const taList = tasks?.data || []
    cities = glance?.cities || []

    stats[0].value = glance?.tenants ?? tList.length
    stats[1].value = glance?.instances ?? iList.length
    stats[2].value = glance?.runningInstances ?? iList.filter(i => i.state === 'RUNNING').length
    stats[3].value = glance?.tasks ?? taList.filter(t => t.status === 'pending' || t.status === 'running').length

    runningCount.value = stats[2].value
    activeTasks.value = stats[3].value
    syncedTenants.value = tList.filter(t => t.status === 'active').length
    regionCount.value = glance?.regions ?? 0
  } catch { /* ignore load errors */ }
  loading.value = false

  // Init map after data loaded
  await nextTick()
  initMap()
  // Prefer IP geolocation cities markers; fall back to region-based markers.
  if (cities.length > 0) {
    updateCityMarkers(cities)
  } else {
    updateMarkers(tList, iList)
  }
})

onBeforeUnmount(() => {
  if (map) {
    map.remove()
    map = null
    markerLayer = null
  }
})
</script>

<style scoped>
.dashboard {
  max-width: 1100px;
}

/* ── Stats Grid ───────────────────────────────────────────── */
.stat-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 16px;
  margin-bottom: 28px;
}

.stat-card {
  background: var(--card-bg);
  border-radius: var(--border-radius);
  padding: 20px;
  display: flex;
  align-items: center;
  gap: 16px;
  box-shadow: var(--shadow-sm);
  transition: all var(--transition);
  position: relative;
  overflow: hidden;
}

.stat-card::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 3px;
  background: var(--accent);
  border-radius: 0 2px 2px 0;
}

.stat-card:hover {
  box-shadow: var(--shadow);
  transform: translateY(-1px);
}

.stat-icon {
  width: 44px;
  height: 44px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--accent);
  background: color-mix(in srgb, var(--accent) 12%, transparent);
  flex-shrink: 0;
}

.stat-body { flex: 1; }

.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--text-primary);
  line-height: 1.2;
  letter-spacing: -0.02em;
}

.stat-label {
  font-size: 13px;
  color: var(--text-muted);
  margin-top: 2px;
}

/* ── Dashboard Grid ────────────────────────────────────────── */
.dashboard-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 20px;
}

@media (max-width: 768px) {
  .dashboard-grid { grid-template-columns: 1fr; }
}

.dash-section h4 {
  margin: 0 0 14px;
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
}

/* ── Action Cards ──────────────────────────────────────────── */
.action-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 10px;
}

.action-card {
  background: var(--card-bg);
  border-radius: var(--border-radius);
  padding: 18px 12px;
  text-align: center;
  cursor: pointer;
  box-shadow: var(--shadow-sm);
  transition: all var(--transition);
}

.action-card:hover {
  box-shadow: var(--shadow);
  transform: translateY(-2px);
}

.action-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  margin-bottom: 8px;
}

.action-label {
  font-size: 12px;
  font-weight: 500;
  color: var(--text-secondary);
}

/* ── Info Grid ────────────────────────────────────────────── */
.info-grid {
  background: var(--card-bg);
  border-radius: var(--border-radius);
  box-shadow: var(--shadow-sm);
  overflow: hidden;
}

.info-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 18px;
  border-bottom: 1px solid var(--border-color);
}

.info-item:last-child { border-bottom: none; }

.info-label {
  font-size: 13px;
  color: var(--text-secondary);
}

.info-value {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

/* ── Map ──────────────────────────────────────────────────── */
.map-section {
  margin-top: 24px;
}

.map-container {
  width: 100%;
  height: 380px;
  border-radius: var(--border-radius);
  overflow: hidden;
  box-shadow: var(--shadow-sm);
  background: #e8e8e8;
}

.map-legend {
  display: flex;
  gap: 18px;
  margin-top: 10px;
  font-size: 12px;
  color: var(--text-secondary);
}

.legend-item {
  display: flex;
  align-items: center;
  gap: 6px;
}

.legend-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  display: inline-block;
  border: 1.5px solid rgba(255,255,255,0.8);
}

.legend-dot.running { background: #10b981; }
.legend-dot.stopped { background: #ef4444; }
.legend-dot.other   { background: #8b5cf6; }
</style>
