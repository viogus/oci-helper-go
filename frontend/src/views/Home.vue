<template>
  <div>
    <h3 style="margin-bottom:20px">Dashboard</h3>
    <el-row :gutter="16" v-loading="loading">
      <el-col :span="6" v-for="stat in stats" :key="stat.label">
        <el-card shadow="hover" :body-style="{padding:'20px'}">
          <div class="stat-card">
            <el-icon :size="32" :color="stat.color"><component :is="stat.icon" /></el-icon>
            <div>
              <div class="stat-value">{{ stat.value }}</div>
              <div class="stat-label">{{ stat.label }}</div>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>
    <h3 style="margin:24px 0 16px">Quick Access</h3>
    <el-row :gutter="16">
      <el-col :span="6" v-for="link in links" :key="link.path">
        <el-card shadow="hover" class="quick-card" @click="$router.push(link.path)">
          <div style="text-align:center;cursor:pointer">
            <el-icon :size="28"><component :is="link.icon" /></el-icon>
            <div style="margin-top:8px;font-weight:bold">{{ link.label }}</div>
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { get } from '../api/index.js'
import { Monitor, User, Timer, Plus, Lock } from '@element-plus/icons-vue'

const loading = ref(true)
const stats = reactive([
  { label: 'Tenants', value: 0, icon: 'User', color: '#409eff' },
  { label: 'Instances', value: 0, icon: 'Monitor', color: '#67c23a' },
  { label: 'Running Tasks', value: 0, icon: 'Timer', color: '#e6a23c' },
])

const links = [
  { label: 'Instances', path: '/instances', icon: 'Monitor' },
  { label: 'Create Instance', path: '/instances/create', icon: 'Plus' },
  { label: 'Tenants', path: '/tenants', icon: 'User' },
  { label: 'Security Rules', path: '/security-rules', icon: 'Lock' },
]

onMounted(async () => {
  try {
    const [tenants, instances, tasks] = await Promise.all([
      get('/tenants', { size: 1 }),
      get('/instances', { size: 1 }),
      get('/tasks', { size: 100 }),
    ])
    stats[0].value = tenants?.total || 0
    stats[1].value = instances?.total || 0
    stats[2].value = (tasks?.data || []).filter(t => t.status === 'running').length
  } catch {}
  loading.value = false
})
</script>

<style scoped>
.stat-card { display: flex; align-items: center; gap: 16px; }
.stat-value { font-size: 28px; font-weight: bold; }
.stat-label { color: #909399; font-size: 13px; margin-top: 4px; }
.quick-card { cursor: pointer; transition: transform 0.2s; }
.quick-card:hover { transform: translateY(-2px); }
</style>
