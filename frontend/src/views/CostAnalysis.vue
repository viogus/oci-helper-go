<template>
  <div>
    <h3>{{ $t('cost.title') }}</h3>
    <el-card>
      <!-- Row 1: tenant + report type -->
      <el-form :inline="true">
        <el-form-item :label="$t('cost.selectTenant')">
          <el-select v-model="tenantId" :placeholder="$t('cost.selectTenant')" style="width:180px">
            <el-option v-for="t in tenants" :key="t.id" :label="t.name" :value="t.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('cost.reportType')">
          <el-select v-model="reportType" style="width:240px">
            <el-option
              v-for="rt in reportTypes"
              :key="rt.value"
              :label="rt.label"
              :value="rt.value"
            />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('cost.granularity')">
          <el-select v-model="granularity" style="width:120px">
            <el-option label="DAILY" value="DAILY" />
            <el-option label="MONTHLY" value="MONTHLY" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('cost.queryType')">
          <el-select v-model="queryType" style="width:120px">
            <el-option label="COST" value="COST" />
            <el-option label="USAGE" value="USAGE" />
          </el-select>
        </el-form-item>
      </el-form>

      <!-- Row 2: date + query button -->
      <el-form :inline="true">
        <el-form-item :label="$t('cost.dateRange')">
          <el-date-picker
            v-model="dateRange"
            :type="granularity === 'MONTHLY' ? 'monthrange' : 'daterange'"
            :range-separator="$t('cost.to')"
            :start-placeholder="$t('cost.start')"
            :end-placeholder="$t('cost.end')"
            :format="granularity === 'MONTHLY' ? 'YYYY-MM' : 'YYYY-MM-DD'"
            :value-format="granularity === 'MONTHLY' ? 'YYYY-MM-DD' : 'YYYY-MM-DD'"
            style="width:260px"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="loadCost" :loading="loading">{{ $t('cost.query') }}</el-button>
          <el-button @click="resetDateRange">{{ $t('cost.resetDate') }}</el-button>
        </el-form-item>
      </el-form>

      <!-- Summary cards -->
      <div v-if="result && result.total > 0" class="cost-summary">
        <div class="cost-card total">
          <div class="cost-value">{{ result.currency || 'USD' }} {{ result.totalCost?.toFixed(2) }}</div>
          <div class="cost-label">{{ $t('cost.totalCost') }}</div>
        </div>
        <div class="cost-card">
          <div class="cost-value">{{ result.total }}</div>
          <div class="cost-label">{{ $t('cost.records') }}</div>
        </div>
        <div class="cost-card">
          <div class="cost-value">{{ result.currency || 'USD' }}</div>
          <div class="cost-label">{{ $t('cost.currency') }}</div>
        </div>
      </div>

      <!-- Trend chart (DAILY) -->
      <div v-if="result && result.total > 0 && granularity === 'DAILY'" ref="trendChart" class="chart-box"></div>

      <!-- Pie chart (MONTHLY) -->
      <div v-if="result && result.total > 0 && granularity === 'MONTHLY'" ref="pieChart" class="chart-box"></div>

      <!-- Table -->
      <el-table v-if="result && result.total > 0" :data="result.items" stripe size="small" style="margin-top:16px" :default-sort="{prop: 'cost', order: 'descending'}">
        <el-table-column prop="service" :label="$t('cost.service')" min-width="140" sortable />
        <el-table-column v-if="hasField('description')" prop="description" :label="$t('cost.description')" min-width="160" />
        <el-table-column v-if="hasField('skuName')" prop="skuName" :label="$t('cost.skuName')" min-width="160" />
        <el-table-column v-if="hasField('compartmentName')" prop="compartmentName" :label="$t('cost.compartment')" min-width="140" />
        <el-table-column v-if="hasField('region')" prop="region" :label="$t('cost.region')" width="120" />
        <el-table-column prop="date" :label="$t('cost.date')" width="120" sortable />
        <el-table-column :label="$t('cost.amount')" width="150" sortable prop="cost">
          <template #default="{ row }">{{ row.currency || 'USD' }} {{ (row.cost || 0).toFixed(4) }}</template>
        </el-table-column>
        <el-table-column v-if="hasField('computedQuantity')" prop="computedQuantity" :label="$t('cost.quantity')" width="130" sortable>
          <template #default="{ row }">{{ (row.computedQuantity || 0).toFixed(4) }} {{ row.unit || '' }}</template>
        </el-table-column>
      </el-table>

      <el-empty v-if="!loading && tenantId && result && result.total === 0" :description="$t('cost.noData')" />
      <el-empty v-if="!tenantId" :description="$t('cost.selectTenantHint')" />
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, nextTick } from 'vue'
import { post } from '../api/index.js'
import { listTenants } from '../api/tenants.js'
import * as echarts from 'echarts'

const tenants = ref([])
const tenantId = ref(null)
const dateRange = ref([])
const loading = ref(false)
const result = ref(null)

// New filters
const reportTypes = [
  { value: 'MONTHLY_COST', label: 'Cost by Service (Monthly)' },
  { value: 'COST_BY_SERVICE', label: 'Cost by Service' },
  { value: 'COST_BY_SERVICE_AND_DESCRIPTION', label: 'Cost by Service + Description' },
  { value: 'COST_BY_SERVICE_AND_SKU', label: 'Cost by Service + SKU' },
  { value: 'COST_BY_SERVICE_AND_TAG', label: 'Cost by Service + Tag' },
  { value: 'COST_BY_COMPARTMENT', label: 'Cost by Compartment' },
]
const reportType = ref('MONTHLY_COST')
const granularity = ref('MONTHLY')
const queryType = ref('COST')

const trendChart = ref(null)
const pieChart = ref(null)
let chart = null

// Default to last 3 months
function initDateRange() {
  const now = new Date()
  if (granularity.value === 'MONTHLY') {
    const d2 = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-01`
    const d1 = new Date(now.getFullYear(), now.getMonth() - 2, 1)
    const d1s = `${d1.getFullYear()}-${String(d1.getMonth() + 1).padStart(2, '0')}-01`
    dateRange.value = [d1s, d2]
  } else {
    // Default to last 30 days
    const d2 = now.toISOString().slice(0, 10)
    const d1 = new Date(now.getTime() - 30 * 86400000)
    dateRange.value = [d1.toISOString().slice(0, 10), d2]
  }
}

function resetDateRange() {
  initDateRange()
}

onMounted(async () => {
  try {
    const res = await listTenants()
    tenants.value = res.data || []
  } catch {}
  initDateRange()
})

// Check if any item has a non-empty value for a field
function hasField(field) {
  if (!result.value?.items) return false
  return result.value.items.some(item => item[field] && item[field] !== '' && item[field] !== 0)
}

async function loadCost() {
  if (!tenantId.value || !dateRange.value || dateRange.value.length !== 2) return
  loading.value = true
  try {
    const res = await post('/cost/analysis', {
      tenant_id: tenantId.value,
      report_type: reportType.value,
      start_date: dateRange.value[0],
      end_date: dateRange.value[1],
      granularity: granularity.value,
      query_type: queryType.value,
    })
    result.value = res || null
    await nextTick()
    renderChart()
  } catch {}
  loading.value = false
}

function renderChart() {
  if (!result.value?.items?.length) return
  if (chart) { chart.dispose(); chart = null }

  if (granularity.value === 'DAILY') {
    renderTrendChart()
  } else {
    renderPieChart()
  }
}

function renderTrendChart() {
  if (!trendChart.value) return
  chart = echarts.init(trendChart.value)

  // Group by date, sum cost per day
  const byDate = {}
  for (const item of result.value.items) {
    const d = item.date || 'unknown'
    byDate[d] = (byDate[d] || 0) + (item.cost || 0)
  }
  const dates = Object.keys(byDate).sort()
  const values = dates.map(d => parseFloat(byDate[d].toFixed(4)))

  chart.setOption({
    tooltip: { trigger: 'axis' },
    grid: { left: 60, right: 30, top: 20, bottom: 40 },
    xAxis: { type: 'category', data: dates, axisLabel: { rotate: 45, fontSize: 10 } },
    yAxis: { type: 'value', name: result.value.currency || 'USD' },
    series: [{
      type: 'bar',
      data: values,
      itemStyle: { color: '#409EFF' },
      markLine: {
        data: [{ type: 'average', name: 'Avg' }],
        lineStyle: { color: '#E6A23C', type: 'dashed' },
      },
    }],
  })
}

function renderPieChart() {
  if (!pieChart.value) return
  chart = echarts.init(pieChart.value)

  const data = result.value.items
    .filter(i => i.cost > 0)
    .sort((a, b) => b.cost - a.cost)
    .slice(0, 10)
    .map(i => ({ name: i.service || i.skuName || i.compartmentName || 'Unknown', value: parseFloat(i.cost.toFixed(4)) }))

  const other = result.value.items
    .filter(i => i.cost > 0)
    .sort((a, b) => b.cost - a.cost)
    .slice(10)
    .reduce((s, i) => s + (i.cost || 0), 0)
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
.cost-summary { display:flex; gap:16px; margin-top:16px; flex-wrap:wrap }
.cost-card {
  flex:1; background:var(--card-bg); border-radius:8px; padding:16px 20px;
  box-shadow:var(--shadow-sm); max-width:240px; min-width:160px
}
.cost-card.total { border-left:3px solid #2563eb }
.cost-value { font-size:22px; font-weight:700; color:var(--text-primary) }
.cost-label { font-size:12px; color:var(--text-muted); margin-top:4px }
.chart-box { width:100%; height:380px; margin-top:16px }
</style>
