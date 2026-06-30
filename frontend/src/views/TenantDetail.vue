<template>
  <div>
    <div class="page-header">
      <el-button @click="$router.push('/tenants')" :icon="'ArrowLeft'" text>{{ $t('common.back') }}</el-button>
      <h3>{{ tenant.name || $t('tenantDetail.title') }}</h3>
      <el-tag :type="tenant.status==='active'?'success':'warning'" size="small">{{ tenant.status }}</el-tag>
    </div>

    <el-card v-loading="loading">
      <template v-if="!loading && tenant.id">
        <!-- Summary cards -->
        <div class="stat-row">
          <div class="stat-card">
            <div class="stat-value">{{ instanceStats?.total || 0 }}</div>
            <div class="stat-label">{{ $t('tenantDetail.totalInstances') }}</div>
          </div>
          <div class="stat-card running">
            <div class="stat-value">{{ instanceStats?.RUNNING || 0 }}</div>
            <div class="stat-label">{{ $t('tenantDetail.running') }}</div>
          </div>
          <div class="stat-card stopped">
            <div class="stat-value">{{ instanceStats?.STOPPED || 0 }}</div>
            <div class="stat-label">{{ $t('tenantDetail.stopped') }}</div>
          </div>
          <div class="stat-card">
            <div class="stat-value">{{ totalOCPU?.toFixed(0) || 0 }}</div>
            <div class="stat-label">{{ $t('tenantDetail.totalOCPU') }}</div>
          </div>
          <div class="stat-card">
            <div class="stat-value">{{ totalMemoryGB?.toFixed(0) || 0 }} GB</div>
            <div class="stat-label">{{ $t('tenantDetail.totalMemory') }}</div>
          </div>
        </div>

        <!-- Regions -->
        <div v-if="regions && regions.length > 0" style="margin-bottom:16px">
          <el-tag v-for="r in regions" :key="r" size="small" style="margin-right:4px;margin-bottom:4px">{{ r }}</el-tag>
        </div>

        <el-descriptions :column="2" border>
          <el-descriptions-item :label="$t('tenantDetail.id')">{{ tenant.id }}</el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.name')">{{ tenant.name }}</el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.region')">
            <el-tag size="small">{{ tenant.region }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.homeRegion')">
            <el-tag size="small" type="info">{{ tenant.homeRegion || '—' }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.userOcid')" :span="2">
            <code>{{ tenant.userOcid }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.tenancyOcid')" :span="2">
            <code>{{ tenant.tenancyOcid }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.fingerprint')">
            <code>{{ tenant.fingerprint }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.keyFile')">
            <code>{{ tenant.keyFile || '—' }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.createdAt')">{{ formatTime(tenant.createdAt) }}</el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.updatedAt')">{{ formatTime(tenant.updatedAt) }}</el-descriptions-item>
        </el-descriptions>

        <h4 style="margin-top:24px">{{ $t('tenantDetail.instances') }}</h4>
        <el-table :data="instances" stripe size="small" style="margin-top:12px">
          <el-table-column prop="name" :label="$t('tenantDetail.instanceName')" min-width="140" />
          <el-table-column prop="shape" :label="$t('tenantDetail.shape')" width="200" />
          <el-table-column :label="$t('tenantDetail.config')" width="160">
            <template #default="{ row }">{{ row.ocpu }}c / {{ row.memoryGB }}G / {{ row.bootVolumeGB }}G</template>
          </el-table-column>
          <el-table-column prop="publicIp" :label="$t('tenantDetail.publicIP')" width="150" />
          <el-table-column prop="state" :label="$t('tenantDetail.state')" width="110">
            <template #default="{ row }">
              <el-tag :type="row.state==='RUNNING'?'success':row.state==='STOPPED'?'danger':'info'" size="small">{{ row.state }}</el-tag>
            </template>
          </el-table-column>
        </el-table>
      </template>
      <el-empty v-if="!loading && !tenant.id" :description="$t('tenantDetail.notFound')" />
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { get } from '../api/index.js'

const route = useRoute()
const loading = ref(true)
const tenant = ref({})
const instances = ref([])
const regions = ref([])
const instanceStats = ref(null)
const totalOCPU = ref(0)
const totalMemoryGB = ref(0)

onMounted(async () => {
  const id = route.params.id
  try {
    const [infoRes, iRes] = await Promise.all([
      get(`/tenants/${id}/info`),
      get('/instances', { tenant_id: id, size: 500 }),
    ])
    tenant.value = infoRes?.tenant || {}
    regions.value = infoRes?.regions || []
    instanceStats.value = infoRes?.instanceStats || {}
    totalOCPU.value = infoRes?.totalOCPU || 0
    totalMemoryGB.value = infoRes?.totalMemoryGB || 0
    instances.value = iRes?.data || []
  } catch {
    // Fallback: basic tenant info
    try {
      const tRes = await get(`/tenants/${id}`)
      tenant.value = tRes || {}
    } catch {}
  }
  loading.value = false
})

function formatTime(t) {
  if (!t) return '—'
  return new Date(t).toLocaleString()
}
</script>

<style scoped>
.page-header { display:flex; align-items:center; gap:12px; margin-bottom:12px }
.page-header h3 { margin:0 }
code { font-size:12px; word-break:break-all }
.stat-row { display:flex; gap:12px; margin-bottom:16px; flex-wrap:wrap }
.stat-card {
  flex:1; background:var(--card-bg); border-radius:8px; padding:14px 18px;
  box-shadow:var(--shadow-sm); max-width:140px; min-width:100px; text-align:center
}
.stat-card.running { border-left:3px solid #67C23A }
.stat-card.stopped { border-left:3px solid #F56C6C }
.stat-value { font-size:24px; font-weight:700; color:var(--text-primary) }
.stat-label { font-size:11px; color:var(--text-muted); margin-top:2px }
</style>
