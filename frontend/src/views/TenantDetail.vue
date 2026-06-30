<template>
  <div>
    <div class="page-header">
      <el-button @click="$router.push('/tenants')" :icon="'ArrowLeft'" text>{{ $t('common.back') }}</el-button>
      <h3>{{ tenant.name || $t('tenantDetail.title') }}</h3>
    </div>

    <el-card v-loading="loading">
      <template v-if="!loading && tenant.id">
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="$t('tenantDetail.id')">{{ tenant.id }}</el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.name')">{{ tenant.name }}</el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.region')">
            <el-tag size="small">{{ tenant.region }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.homeRegion')">
            <el-tag size="small" type="info">{{ tenant.homeRegion || '—' }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.status')">
            <el-tag :type="tenant.status==='active'?'success':'warning'" size="small">{{ tenant.status }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('tenantDetail.subscribed')">{{ tenant.subscribed || '—' }}</el-descriptions-item>
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

onMounted(async () => {
  const id = route.params.id
  try {
    const [tRes, iRes] = await Promise.all([
      get(`/tenants/${id}`),
      get('/instances', { tenant_id: id, size: 500 }),
    ])
    tenant.value = tRes || {}
    instances.value = iRes?.data || []
  } catch { /* ignore */ }
  loading.value = false
})

function formatTime(t) {
  if (!t) return '—'
  return new Date(t).toLocaleString()
}
</script>

<style scoped>
.page-header { display:flex; align-items:center; gap:12px; margin-bottom:16px }
.page-header h3 { margin:0 }
code { font-size:12px; word-break:break-all }
</style>
