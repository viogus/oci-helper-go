<template>
  <div>
    <h3>{{ $t('vcn.title') }}</h3>
    <el-card>
      <el-form :inline="true">
        <el-form-item :label="$t('vcn.selectTenant')">
          <el-select v-model="tenantId" :placeholder="$t('vcn.selectTenant')" @change="loadVCNs" style="width:240px">
            <el-option v-for="t in tenants" :key="t.id" :label="t.name" :value="t.id" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="loadVCNs" :loading="loading">{{ $t('vcn.refresh') }}</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="vcns" stripe v-loading="loading" size="small" style="margin-top:12px">
        <el-table-column prop="displayName" :label="$t('vcn.name')" min-width="160" />
        <el-table-column prop="id" :label="$t('vcn.ocid')" min-width="280">
          <template #default="{ row }"><code>{{ row.id }}</code></template>
        </el-table-column>
        <el-table-column :label="$t('vcn.cidrBlocks')" min-width="180">
          <template #default="{ row }">
            <el-tag v-for="c in (row.cidrBlocks||[])" :key="c" size="small" style="margin:1px">{{ c }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="lifecycleState" :label="$t('vcn.state')" width="120">
          <template #default="{ row }">
            <el-tag :type="row.lifecycleState==='AVAILABLE'?'success':'info'" size="small">{{ row.lifecycleState }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="$t('vcn.subnets')" width="100">
          <template #default="{ row }">
            <el-button size="small" text type="primary" @click="loadSubnets(row.id)">{{ subnetCount(row.id) }}</el-button>
          </template>
        </el-table-column>
        <el-table-column :label="$t('vcn.actions')" width="120">
          <template #default="{ row }">
            <el-popconfirm :title="$t('vcn.confirmDelete')" @confirm="deleteVCN(row.id)">
              <template #reference>
                <el-button size="small" type="danger" text>{{ $t('vcn.delete') }}</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
      </el-table>

      <!-- Subnets drawer -->
      <el-drawer v-model="subnetVisible" :title="$t('vcn.subnetList')" size="500px">
        <el-table :data="subnetList" stripe size="small">
          <el-table-column prop="displayName" :label="$t('vcn.subnetName')" min-width="140" />
          <el-table-column prop="cidrBlock" :label="$t('vcn.cidr')" min-width="140">
            <template #default="{ row }"><code>{{ row.cidrBlock }}</code></template>
          </el-table-column>
          <el-table-column :label="$t('vcn.subnetState')" width="100">
            <template #default="{ row }">
              <el-tag :type="row.lifecycleState==='AVAILABLE'?'success':'info'" size="small">{{ row.lifecycleState }}</el-tag>
            </template>
          </el-table-column>
        </el-table>
      </el-drawer>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { get, del } from '../api/index.js'
import { listTenants } from '../api/tenants.js'

const tenants = ref([])
const vcns = ref([])
const loading = ref(false)
const tenantId = ref(null)
const subnetVisible = ref(false)
const subnetList = ref([])
const subnetCounts = ref({})

onMounted(async () => {
  try {
    const res = await listTenants()
    tenants.value = res.data || []
  } catch {}
})

async function loadVCNs() {
  if (!tenantId.value) return
  loading.value = true
  try {
    const res = await get('/vcns', { tenant_id: tenantId.value, size: 100 })
    vcns.value = res?.data || []
    // Load subnets for each VCN to get counts
    for (const v of vcns.value) {
      try {
        const subRes = await get('/subnets', { tenant_id: tenantId.value, vcn_id: v.id })
        subnetCounts.value[v.id] = (subRes?.data || []).length
      } catch { subnetCounts.value[v.id] = 0 }
    }
  } catch {}
  loading.value = false
}

async function loadSubnets(vcnId) {
  try {
    const res = await get('/subnets', { tenant_id: tenantId.value, vcn_id: vcnId })
    subnetList.value = res?.data || []
    subnetVisible.value = true
  } catch {}
}

function subnetCount(vcnId) {
  return subnetCounts.value[vcnId] ?? '...'
}

async function deleteVCN(vcnId) {
  try {
    await del(`/vcns/${vcnId}`, { tenant_id: tenantId.value })
    await loadVCNs()
  } catch {}
}
</script>

<style scoped>
code { font-size:12px; word-break:break-all }
</style>
