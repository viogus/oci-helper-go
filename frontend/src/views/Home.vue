<template>
  <div class="dashboard">
    <div class="page-header">
      <h3>Dashboard</h3>
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
          <div class="stat-label">{{ stat.label }}</div>
        </div>
        <div class="stat-trend" v-if="stat.trend !== undefined">
          <span :class="stat.trend >= 0 ? 'up' : 'down'">
            {{ stat.trend >= 0 ? '+' : '' }}{{ stat.trend }}%
          </span>
        </div>
      </div>
    </div>

    <!-- Section: Quick Access & Info -->
    <div class="dashboard-grid">
      <!-- Quick Access -->
      <div class="dash-section">
        <h4>Quick Access</h4>
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
            <div class="action-label">{{ link.label }}</div>
          </div>
        </div>
      </div>

      <!-- Recent Activity -->
      <div class="dash-section">
        <h4>Resources</h4>
        <div class="info-grid">
          <div class="info-item">
            <span class="info-label">Regions</span>
            <span class="info-value">{{ regionCount }}</span>
          </div>
          <div class="info-item">
            <span class="info-label">Running Instances</span>
            <span class="info-value">{{ runningCount }}</span>
          </div>
          <div class="info-item">
            <span class="info-label">Active Tasks</span>
            <span class="info-value">{{ activeTasks }}</span>
          </div>
          <div class="info-item">
            <span class="info-label">Synced Tenants</span>
            <span class="info-value">{{ syncedTenants }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { get } from '../api/index.js'
import {
  Monitor, User, Timer, Plus, Lock, Connection,
  Cloudy, ChatDotRound, Setting
} from '@element-plus/icons-vue'

const loading = ref(true)
const regionCount = ref(0)
const runningCount = ref(0)
const activeTasks = ref(0)
const syncedTenants = ref(0)

const stats = reactive([
  { label: 'Tenants', value: '—', icon: 'User', color: '#2563eb' },
  { label: 'Instances', value: '—', icon: 'Monitor', color: '#10b981' },
  { label: 'Running', value: '—', icon: 'Timer', color: '#f59e0b' },
  { label: 'Tasks Active', value: '—', icon: 'Timer', color: '#8b5cf6' },
])

const links = [
  { label: 'Instances', path: '/instances', icon: 'Monitor', bg: 'linear-gradient(135deg,#2563eb,#6366f1)' },
  { label: 'Create', path: '/instances/create', icon: 'Plus', bg: 'linear-gradient(135deg,#10b981,#059669)' },
  { label: 'Security', path: '/security-rules', icon: 'Lock', bg: 'linear-gradient(135deg,#f59e0b,#d97706)' },
  { label: 'Network', path: '/traffic', icon: 'Connection', bg: 'linear-gradient(135deg,#8b5cf6,#7c3aed)' },
  { label: 'Cloudflare', path: '/cloudflare', icon: 'Cloudy', bg: 'linear-gradient(135deg,#06b6d4,#0891b2)' },
  { label: 'AI Chat', path: '/ai-chat', icon: 'ChatDotRound', bg: 'linear-gradient(135deg,#ec4899,#db2777)' },
  { label: 'Tenants', path: '/tenants', icon: 'User', bg: 'linear-gradient(135deg,#64748b,#475569)' },
  { label: 'Settings', path: '/settings', icon: 'Setting', bg: 'linear-gradient(135deg,#78716c,#57534e)' },
]

onMounted(async () => {
  try {
    const [tenants, instances, tasks] = await Promise.all([
      get('/tenants', { size: 100 }),
      get('/instances', { size: 100 }),
      get('/tasks', { size: 100 }),
    ])
    const tList = tenants?.data || []
    const iList = instances?.data || []
    const taList = tasks?.data || []

    stats[0].value = tList.length
    stats[1].value = iList.length
    stats[2].value = iList.filter(i => i.state === 'RUNNING').length
    stats[3].value = taList.filter(t => t.status === 'pending' || t.status === 'running').length

    runningCount.value = stats[2].value
    activeTasks.value = stats[3].value
    syncedTenants.value = tList.filter(t => t.status === 'active').length

    // Count unique regions
    const regions = new Set(iList.map(i => '—'))
    tList.forEach(t => { if (t.region) regions.add(t.region) })
    regionCount.value = regions.size
  } catch {}
  loading.value = false
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
</style>
