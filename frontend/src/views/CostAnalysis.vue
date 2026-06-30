<template>
  <div>
    <h3>{{ $t('cost.title') }}</h3>
    <el-card>
      <el-form :inline="true">
        <el-form-item :label="$t('cost.selectTenant')">
          <el-select v-model="tenantId" :placeholder="$t('cost.selectTenant')" @change="loadCost" style="width:200px">
            <el-option v-for="t in tenants" :key="t.id" :label="t.name" :value="t.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('cost.dateRange')">
          <el-date-picker v-model="dateRange" type="monthrange" :range-separator="$t('cost.to')" :start-placeholder="$t('cost.start')" :end-placeholder="$t('cost.end')" format="YYYY-MM" value-format="YYYY-MM-DD" @change="loadCost" style="width:240px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="loadCost" :loading="loading">{{ $t('cost.query') }}</el-button>
        </el-form-item>
      </el-form>

      <!-- Summary cards -->
      <div v-if="totalCost > 0" class="cost-summary">
        <div class="cost-card total">
          <div class="cost-value">{{ currency }} {{ totalCost.toFixed(2) }}</div>
          <div class="cost-label">{{ $t('cost.totalCost') }}</div>
        </div>
        <div class="cost-card">
          <div class="cost-value">{{ items.length }}</div>
          <div class="cost-label">{{ $t('cost.services') }}</div>
        </div>
      </div>

      <!-- Pie chart -->
      <div v-if="items.length > 0" ref="pieChart" class="chart-box"></div>

      <!-- Table -->
      <el-table v-if="items.length > 0" :data="items" stripe size="small" style="margin-top:16px">
        <el-table-column prop="service" :label="$t('cost.service')" min-width="200" />
        <el-table-column :label="$t('cost.amount')" width="160" sortable prop="amount">
          <template #default="{ row }">{{ row.currency || 'USD' }} {{ (row.amount||0).toFixed(4) }}</template>
        </el-table-column>
        <el-table-column prop="date" :label="$t('cost.month')" width="120" />
      </el-table>

      <el-empty v-if="!loading && tenantId && items.length === 0" :description="$t('cost.noData')" />
      <el-empty v-if="!tenantId" :description="$t('cost.selectTenantHint')" />
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted, nextTick } from 'vue'
import { get } from '../api/index.js'
import { listTenants } from '../api/tenants.js'
import * as echarts from 'echarts'

const tenants = ref([])
const tenantId = ref(null)
const dateRange = ref([])
const loading = ref(false)
const items = ref([])
const totalCost = ref(0)
const currency = ref('USD')
const pieChart = ref(null)
let chart = null

// Default to last 3 months
const now = new Date()
const d2 = `${now.getFullYear()}-${String(now.getMonth()+1).padStart(2,'0')}-01`
const d1 = new Date(now.getFullYear(), now.getMonth()-2, 1)
const d1s = `${d1.getFullYear()}-${String(d1.getMonth()+1).padStart(2,'0')}-01`
dateRange.value = [d1s, d2]

onMounted(async () => {
  try {
    const res = await listTenants()
    tenants.value = res.data || []
  } catch {}
})

async function loadCost() {
  if (!tenantId.value || !dateRange.value || dateRange.value.length !== 2) return
  loading.value = true
  try {
    const res = await get('/cost', {
      tenant_id: tenantId.value,
      start: dateRange.value[0],
      end: dateRange.value[1],
    })
    items.value = res?.data || []
    totalCost.value = 0
    currency.value = 'USD'
    for (const it of items.value) {
      totalCost.value += it.amount || 0
      if (it.currency) currency.value = it.currency
    }
    await nextTick()
    renderChart()
  } catch {}
  loading.value = false
}

function renderChart() {
  if (!pieChart.value) return
  if (chart) chart.dispose()
  chart = echarts.init(pieChart.value)
  const data = items.value
    .filter(i => i.amount > 0)
    .sort((a, b) => b.amount - a.amount)
    .slice(0, 10)
    .map(i => ({ name: i.service, value: parseFloat(i.amount.toFixed(4)) }))

  const other = items.value
    .filter(i => i.amount > 0)
    .sort((a, b) => b.amount - a.amount)
    .slice(10)
    .reduce((s, i) => s + (i.amount||0), 0)
  if (other > 0) data.push({ name: 'Other', value: parseFloat(other.toFixed(4)) })

  chart.setOption({
    tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
    series: [{
      type: 'pie',
      radius: ['40%', '70%'],
      center: ['50%', '50%'],
      data,
      label: { formatter: '{b}\n{d}%', fontSize: 11 },
      emphasis: { label: { fontSize: 16 } },
    }],
  })
}
</script>

<style scoped>
.cost-summary { display:flex; gap:16px; margin-top:16px }
.cost-card {
  flex:1; background:var(--card-bg); border-radius:8px; padding:16px 20px;
  box-shadow:var(--shadow-sm); max-width:240px
}
.cost-card.total { border-left:3px solid #2563eb }
.cost-value { font-size:22px; font-weight:700; color:var(--text-primary) }
.cost-label { font-size:12px; color:var(--text-muted); margin-top:4px }
.chart-box { width:100%; height:340px; margin-top:16px }
</style>
