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

      <!-- Bar chart (DAILY: stacked by service; MONTHLY: grouped by category) -->
      <div v-if="result && result.total > 0" ref="barChart" class="chart-box"></div>

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
import { use, init } from 'echarts/core'
import { BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

use([BarChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

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

const barChart = ref(null)
let chart = null

// Color palette — distinct colors per category, cycling for many services
const COLORS = [
  '#5470c6', '#91cc75', '#fac858', '#ee6666', '#73c0de',
  '#3ba272', '#fc8452', '#9a60b4', '#ea7ccc', '#48b8d0',
  '#f56c6c', '#409EFF', '#e6a23c', '#67c23a', '#909399',
  '#ff85c0', '#5cdbd3', '#b37feb', '#ffd666', '#95de64',
]

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
    renderDailyStackedBar()
  } else {
    renderMonthlyGroupedBar()
  }
}

// Pick display name for an item based on report type
function categoryName(item) {
  if (reportType.value === 'COST_BY_COMPARTMENT') return item.compartmentName || 'Unknown'
  if (reportType.value === 'COST_BY_SERVICE_AND_SKU') return item.skuName || item.service || 'Unknown'
  if (reportType.value === 'COST_BY_SERVICE_AND_DESCRIPTION') return `${item.service || '?'} / ${item.description || '?'}`
  return item.service || item.skuName || item.compartmentName || 'Unknown'
}

// DAILY: stacked bar chart — each day's bar broken down by service with distinct colors
function renderDailyStackedBar() {
  if (!barChart.value) return
  chart = init(barChart.value)

  const items = result.value.items.filter(i => i.cost > 0)
  const currency = result.value.currency || 'USD'

  // Collect all dates (sorted) and all unique services
  const dateSet = new Set()
  const serviceSet = new Set()
  for (const item of items) {
    dateSet.add(item.date || 'unknown')
    serviceSet.add(item.service || 'Other')
  }
  const dates = [...dateSet].sort()
  const services = [...serviceSet]

  // Assign color per service
  const colorMap = {}
  services.forEach((s, i) => { colorMap[s] = COLORS[i % COLORS.length] })

  // Build data per service: stacked bars
  const series = services.map(svc => {
    const byDate = {}
    for (const item of items) {
      if ((item.service || 'Other') === svc) {
        byDate[item.date || 'unknown'] = (byDate[item.date || 'unknown'] || 0) + (item.cost || 0)
      }
    }
    return {
      name: svc,
      type: 'bar',
      stack: 'cost',
      data: dates.map(d => parseFloat((byDate[d] || 0).toFixed(4))),
      itemStyle: { color: colorMap[svc] },
      emphasis: { focus: 'series' },
    }
  })

  // Build color legend: { svc → hex }
  const svcLegend = {}
  services.forEach((s, i) => { svcLegend[s] = COLORS[i % COLORS.length] })

  chart.setOption({
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      formatter: (params) => {
        // params = array of { seriesName, value, color } for each stack segment
        let html = `<div style="font-weight:600;margin-bottom:6px">${params[0].axisValue}</div>`
        let total = 0
        params.forEach(p => {
          if (p.value > 0) {
            html += `<div style="display:flex;align-items:center;gap:6px;margin:2px 0">
              <span style="display:inline-block;width:10px;height:10px;border-radius:2px;background:${p.color};flex-shrink:0"></span>
              <span style="flex:1">${p.seriesName}</span>
              <span style="font-weight:600">${currency} ${p.value.toFixed(2)}</span>
            </div>`
            total += p.value
          }
        })
        html += `<div style="border-top:1px solid #ddd;margin-top:6px;padding-top:4px;font-weight:700">
          Total: ${currency} ${total.toFixed(2)}
        </div>`
        return html
      },
    },
    legend: {
      type: 'scroll',
      bottom: 0,
      textStyle: { fontSize: 11 },
    },
    grid: { left: 70, right: 30, top: 20, bottom: 60 },
    xAxis: {
      type: 'category',
      data: dates,
      axisLabel: { rotate: 45, fontSize: 10 },
    },
    yAxis: {
      type: 'value',
      name: currency,
    },
    series,
  })
}

// MONTHLY: grouped bar chart — each service/category gets own bar with distinct color
function renderMonthlyGroupedBar() {
  if (!barChart.value) return
  chart = init(barChart.value)

  const items = result.value.items.filter(i => i.cost > 0)
  const currency = result.value.currency || 'USD'

  // Collect all sub-categories for this category to show breakdown on hover
  const catSubs = {}  // catName → { service → cost }
  for (const item of items) {
    const cat = categoryName(item)
    if (!catSubs[cat]) catSubs[cat] = {}
    const sub = item.service || item.skuName || item.compartmentName || 'Other'
    catSubs[cat][sub] = (catSubs[cat][sub] || 0) + (item.cost || 0)
  }

  // Aggregate by category name
  const byCat = {}
  for (const item of items) {
    const name = categoryName(item)
    byCat[name] = (byCat[name] || 0) + (item.cost || 0)
  }

  // Sort by cost descending, keep top 15, rest → Other
  let entries = Object.entries(byCat).sort((a, b) => b[1] - a[1])
  let other = 0
  const otherSubs = {}
  if (entries.length > 15) {
    const rest = entries.slice(15)
    for (const [cat, v] of rest) {
      other += v
      if (catSubs[cat]) {
        for (const [sub, sv] of Object.entries(catSubs[cat])) {
          otherSubs[sub] = (otherSubs[sub] || 0) + sv
        }
      }
    }
    entries = entries.slice(0, 15)
  }
  if (other > 0) {
    entries.push(['Other', other])
    catSubs['Other'] = otherSubs
  }

  const names = entries.map(([n]) => n)
  const values = entries.map(([, v]) => parseFloat(v.toFixed(4)))
  const barColors = names.map((_, i) => COLORS[i % COLORS.length])

  chart.setOption({
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      formatter: (params) => {
        const p = params[0]
        const catName = p.name
        const subs = catSubs[catName] || {}
        // Sort sub-categories by cost desc
        const subEntries = Object.entries(subs).sort((a, b) => b[1] - a[1])
        let html = `<div style="font-weight:600;margin-bottom:6px">
          <span style="display:inline-block;width:10px;height:10px;border-radius:2px;background:${p.color};margin-right:6px;vertical-align:middle"></span>
          ${catName}
        </div>`
        subEntries.forEach(([sub, v]) => {
          html += `<div style="display:flex;align-items:center;gap:6px;margin:2px 0;padding-left:16px">
            <span style="flex:1;font-size:12px;color:#666">${sub}</span>
            <span style="font-weight:600">${currency} ${v.toFixed(2)}</span>
          </div>`
        })
        html += `<div style="border-top:1px solid #ddd;margin-top:6px;padding-top:4px;font-weight:700">
          Total: ${currency} ${p.value.toFixed(2)}
        </div>`
        return html
      },
    },
    grid: { left: 90, right: 30, top: 20, bottom: 100 },
    xAxis: {
      type: 'category',
      data: names,
      axisLabel: {
        rotate: 45,
        fontSize: 10,
        interval: 0,
        formatter: v => v.length > 20 ? v.slice(0, 19) + '…' : v,
      },
    },
    yAxis: {
      type: 'value',
      name: currency,
    },
    series: [{
      type: 'bar',
      data: values.map((v, i) => ({ value: v, itemStyle: { color: barColors[i] } })),
      label: {
        show: true,
        position: 'top',
        fontSize: 10,
        formatter: p => p.value > 0 ? p.value.toFixed(1) : '',
      },
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
