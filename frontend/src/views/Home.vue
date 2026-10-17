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
  { labelKey: 'home.tenant', value: '—', icon: User, color: '#2563eb' },
  { labelKey: 'home.instance', value: '—', icon: Monitor, color: '#10b981' },
  { labelKey: 'home.running', value: '—', icon: Timer, color: '#f59e0b' },
  { labelKey: 'home.activeTasks', value: '—', icon: Timer, color: '#8b5cf6' },
])

const links = [
  { labelKey: 'home.instance', path: '/instances', icon: Monitor, bg: 'linear-gradient(135deg,#2563eb,#6366f1)' },
  { labelKey: 'home.create', path: '/instances/create', icon: Plus, bg: 'linear-gradient(135deg,#10b981,#059669)' },
  { labelKey: 'home.security', path: '/security-rules', icon: Lock, bg: 'linear-gradient(135deg,#f59e0b,#d97706)' },
  { labelKey: 'home.network', path: '/traffic', icon: Connection, bg: 'linear-gradient(135deg,#8b5cf6,#7c3aed)' },
  { labelKey: 'home.cloudflare', path: '/cloudflare', icon: Cloudy, bg: 'linear-gradient(135deg,#06b6d4,#0891b2)' },
  { labelKey: 'home.aiChat', path: '/ai-chat', icon: ChatDotRound, bg: 'linear-gradient(135deg,#ec4899,#db2777)' },
  { labelKey: 'home.tenant', path: '/tenants', icon: User, bg: 'linear-gradient(135deg,#64748b,#475569)' },
  { labelKey: 'home.settings', path: '/settings', icon: Setting, bg: 'linear-gradient(135deg,#78716c,#57534e)' },
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

// Key-less tile providers, tried in order, with automatic fallback on
// repeated tile errors so the map is never a blank grey box.
//
// CARTO is deliberately NOT in this list any more: its basemaps now require an
// API key (https://carto.com/basemaps/apikey/) and answer key-less requests
// with HTTP 200 carrying an "API KEY REQUIRED" watermark. Because that is a
// 200, Leaflet never fires `tileerror`, so the fallback below cannot rescue it
// and the watermark sticks forever. To use CARTO again, register a key and add
// back: { name: 'carto', url: 'https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png?api_key=YOUR_KEY', ... }
// (and add https://*.basemaps.cartocdn.com to the CSP img-src in handler.go).
const TILE_PROVIDERS = [
  { name: 'osm', url: 'https://tile.openstreetmap.org/{z}/{x}/{y}.png', opts: { maxZoom: 19 } },
  // OSM mirror (FOSSGIS) that also stays reachable from mainland China, where
  // the main OSM host is blocked.
  { name: 'osm-de', url: 'https://tile.openstreetmap.de/{z}/{x}/{y}.png', opts: { maxZoom: 19 } },
  // AMap / 高德, no key required. style=7 is the base map; style=8 is only the
  // road + label overlay, which renders as an empty tile at low zoom.
  { name: 'amap', url: 'https://webrd0{s}.is.autonavi.com/appmaptile?lang=zh_cn&size=1&scale=1&style=7&x={x}&y={y}&z={z}', opts: { maxZoom: 18, subdomains: '1234' } },
  { name: 'esri', url: 'https://server.arcgisonline.com/ArcGIS/rest/services/World_Street_Map/MapServer/tile/{z}/{y}/{x}', opts: { maxZoom: 19 } },
]
let tileProviderIdx = 0
let tileLayer = null
let tileRotationStopped = false

function loadTileLayer() {
  if (tileLayer) {
    // Detach first: a removed layer can still deliver in-flight tileerror
    // callbacks, which must not count against the newly loaded provider.
    tileLayer.off()
    map.removeLayer(tileLayer)
    tileLayer = null
  }
  const p = TILE_PROVIDERS[tileProviderIdx]
  let errors = 0
  const layer = L.tileLayer(p.url, p.opts)
  layer.on('tileerror', () => {
    if (tileRotationStopped || layer !== tileLayer) return
    errors++
    // A few errors may be transient; 6+ means the provider is unreachable.
    if (errors < 6) return
    if (tileProviderIdx + 1 >= TILE_PROVIDERS.length) {
      // Every provider failed once: keep this one instead of rotating forever.
      tileRotationStopped = true
      console.warn('[home-map] all tile providers unreachable, keeping ' + p.name)
      return
    }
    console.warn('[home-map] tile provider unreachable, switching: ' + p.name)
    tileProviderIdx++
    loadTileLayer()
  })
  tileLayer = layer.addTo(map)
}

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
    // Markers sit at public-IP (city) precision, so allow zooming in far
    // enough to separate instances inside one city.
    maxZoom: 10,
    zoomControl: true,
    attributionControl: false,
  })

  // These live at module scope, so reset them for every fresh map instance.
  tileProviderIdx = 0
  tileRotationStopped = false
  loadTileLayer()

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

// Render city markers from IP geolocation data (glance cities).
// Must live at module scope: it is called from onMounted below, so nesting it
// inside updateMarkers would make that call a ReferenceError and leave the map
// with no markers at all.
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

// ── Per-instance markers ──────────────────────────────────
function escapeHtml(v) {
  return String(v ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]))
}

// instanceCoords returns [lat, lng] for an instance: the geolocated position of
// its public IP when available, otherwise the tenant/instance region.
function instanceCoords(p, regionByTenant) {
  if (p.lat && p.lng) return [p.lat, p.lng]
  const region = (p.region || regionByTenant[p.tenantId] || '').toLowerCase()
  return ociRegions[region] || null
}

// Render one marker per instance. Returns false when nothing could be placed,
// so the caller can fall back to the coarser sources below.
function updateInstanceMarkers(points, regionByTenant) {
  if (!markerLayer || !points?.length) return false
  markerLayer.clearLayers()

  // Instances sharing coordinates (same city, or the same fallback region)
  // would sit exactly on top of each other; spread the extras on a small ring.
  const seen = new Map()
  let drawn = 0
  for (const p of points) {
    const coords = instanceCoords(p, regionByTenant)
    if (!coords) continue
    const key = coords[0].toFixed(3) + ',' + coords[1].toFixed(3)
    const idx = seen.get(key) || 0
    seen.set(key, idx + 1)

    let lat = coords[0]
    let lng = coords[1]
    if (idx > 0) {
      const angle = ((idx - 1) % 8) * (Math.PI / 4)
      const radius = 0.04 * (1 + Math.floor((idx - 1) / 8))
      lat += Math.sin(angle) * radius
      lng += Math.cos(angle) * radius
    }

    const place = [p.city, p.area, p.country].filter(Boolean).join(', ')
    const tooltip = [
      '<b>' + escapeHtml(p.name) + '</b>',
      escapeHtml([p.state, p.region].filter(Boolean).join(' · ')),
      escapeHtml([place, p.publicIp].filter(Boolean).join(' · ')),
    ].filter(Boolean).join('<br>')

    const marker = L.marker([lat, lng], { icon: getIcon(p.state), title: p.name }).addTo(markerLayer)
    marker.bindTooltip(tooltip, { direction: 'top', offset: [0, -8] })
    drawn++
  }
  return drawn > 0
}

// renderMapMarkers picks the most precise source available: one point per
// instance, then geolocated IP-inventory cities, then one point per region.
function renderMapMarkers(points, cities, tenants, instances, regionByTenant) {
  if (updateInstanceMarkers(points, regionByTenant)) return
  if (cities?.length) {
    updateCityMarkers(cities)
    return
  }
  updateMarkers(tenants, instances)
}

onMounted(async () => {
  let tList = [], iList = [], cities = [], mapPoints = [], pending = 0
  try {
    const [tenants, instances, tasks, glance, mapData] = await Promise.all([
      get('/tenants', { size: 100 }),
      get('/instances', { size: 500 }),
      get('/tasks', { size: 100 }),
      get('/glance'),
      // Optional: the dashboard still works if the map endpoint is unavailable.
      get('/instances/map').catch(() => null),
    ])
    tList = tenants?.data || []
    iList = instances?.data || []
    const taList = tasks?.data || []
    cities = glance?.cities || []
    mapPoints = mapData?.points || []
    pending = mapData?.pending || 0

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

  const regionByTenant = {}
  for (const t of tList) {
    if (t.region) regionByTenant[t.id] = t.region.toLowerCase()
  }
  renderMapMarkers(mapPoints, cities, tList, iList, regionByTenant)

  // Public IPs are geolocated a few at a time (see handleInstanceMap), so poll
  // a couple of times: the first markers appear immediately and the rest fill
  // in without a manual refresh.
  for (let i = 0; i < 3 && pending > 0; i++) {
    await new Promise(resolve => setTimeout(resolve, 4000))
    if (!map) return
    try {
      const more = await get('/instances/map')
      mapPoints = more?.points || mapPoints
      pending = more?.pending || 0
      renderMapMarkers(mapPoints, cities, tList, iList, regionByTenant)
    } catch { return }
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
