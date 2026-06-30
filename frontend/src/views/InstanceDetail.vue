<template>
  <div>
    <div class="page-header">
      <el-button @click="$router.push('/instances')" :icon="'ArrowLeft'" text>{{ $t('common.back') }}</el-button>
      <h3>{{ inst.name || $t('instanceDetail.title') }}</h3>
      <el-tag :type="stateType" size="small">{{ inst.state }}</el-tag>
    </div>

    <el-card v-loading="loading">
      <template v-if="!loading && inst.id">
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="$t('instanceDetail.name')">{{ inst.name }}</el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.region')">
            <el-tag size="small">{{ regionName }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.status')">
            <el-tag :type="stateType" size="small">{{ inst.state }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.shape')">
            <code>{{ inst.shape }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.publicIP')">
            <code v-if="inst.publicIp">{{ inst.publicIp }}</code>
            <span v-else style="color:var(--text-muted)">—</span>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.privateIP')">
            <code v-if="inst.privateIp">{{ inst.privateIp }}</code>
            <span v-else style="color:var(--text-muted)">—</span>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.config')">
            {{ inst.ocpu }} {{ $t('instanceDetail.cores') }} / {{ inst.memoryGB }} GB / {{ inst.bootVolumeGB }} GB
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.availabilityDomain')">
            <code>{{ inst.availabilityDomain || '—' }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.faultDomain')">
            <code>{{ inst.faultDomain || '—' }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.imageID')">
            <code>{{ inst.imageId || '—' }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.ocid')" :span="2">
            <code>{{ inst.ocid }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.subnetID')">
            <code>{{ inst.subnetId || '—' }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.tenantID')">
            {{ inst.tenantId }}
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.createdAt')">{{ formatTime(inst.createdAt) }}</el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.syncedAt')">{{ formatTime(inst.syncedAt) }}</el-descriptions-item>
        </el-descriptions>
      </template>
      <el-empty v-if="!loading && !inst.id" :description="$t('instanceDetail.notFound')" />
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { get } from '../api/index.js'

const route = useRoute()
const loading = ref(true)
const inst = ref({})
const regionName = ref('')

const stateType = computed(() => {
  const s = inst.value.state
  if (s === 'RUNNING') return 'success'
  if (s === 'STOPPED' || s === 'TERMINATED') return 'danger'
  if (s === 'STARTING' || s === 'STOPPING') return 'warning'
  return 'info'
})

onMounted(async () => {
  const id = decodeURIComponent(route.params.id)
  try {
    const res = await get(`/instances/${id}`)
    inst.value = res || {}

    // Also fetch tenant to get region name
    if (inst.value.tenantId) {
      try {
        const tRes = await get(`/tenants/${inst.value.tenantId}`)
        regionName.value = tRes?.region || tRes?.homeRegion || ''
      } catch {}
    }
  } catch {}
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
