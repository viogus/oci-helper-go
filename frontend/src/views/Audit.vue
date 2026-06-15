<template>
  <div class="audit-page">
    <div class="filter-bar">
      <el-input
        v-model="keyword"
        placeholder="Search by action or detail..."
        clearable
        @input="handleSearch"
        style="width: 360px"
      />
    </div>

    <el-table
      :data="logs"
      v-loading="loading"
      border
      stripe
      style="width: 100%"
      element-loading-text="Loading audit logs..."
    >
      <el-table-column label="Time" width="180">
        <template #default="{ row }">
          {{ formatTime(row.createdAt) }}
        </template>
      </el-table-column>
      <el-table-column prop="action" label="Action" width="220" />
      <el-table-column prop="detail" label="Detail" min-width="300" />
      <el-table-column prop="ip" label="IP" width="160" />
    </el-table>

    <el-empty
      v-if="!loading && logs.length === 0"
      description="No audit logs found"
    />

    <div class="pagination-wrapper">
      <el-pagination
        v-model:current-page="page"
        v-model:page-size="size"
        :total="total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next"
        @size-change="onSizeChange"
        @current-change="loadAudit"
      />
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { get } from '../api/index.js'

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------
const logs = ref([])
const total = ref(0)
const page = ref(1)
const size = ref(20)
const keyword = ref('')
const loading = ref(false)

// ---------------------------------------------------------------------------
// Debounced search
// ---------------------------------------------------------------------------
let searchTimer = null

function handleSearch() {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    page.value = 1
    loadAudit()
  }, 300)
}

// ---------------------------------------------------------------------------
// Data loading
// ---------------------------------------------------------------------------
async function loadAudit() {
  loading.value = true
  try {
    const params = {
      page: page.value,
      size: size.value
    }
    if (keyword.value) {
      params.keyword = keyword.value
    }
    const res = await get('/audit', params)
    logs.value = res.data || []
    total.value = res.total || 0
  } catch (e) {
    const msg = e.response?.data?.error || e.message
    ElMessage.error('Failed to load audit logs: ' + msg)
  } finally {
    loading.value = false
  }
}

// ---------------------------------------------------------------------------
// Pagination
// ---------------------------------------------------------------------------
function onSizeChange() {
  page.value = 1
  loadAudit()
}

// ---------------------------------------------------------------------------
// Formatting
// ---------------------------------------------------------------------------
function formatTime(t) {
  if (!t) return ''
  const d = new Date(t)
  const pad = (n) => String(n).padStart(2, '0')
  return (
    d.getFullYear() +
    '-' +
    pad(d.getMonth() + 1) +
    '-' +
    pad(d.getDate()) +
    ' ' +
    pad(d.getHours()) +
    ':' +
    pad(d.getMinutes()) +
    ':' +
    pad(d.getSeconds())
  )
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------
onMounted(() => {
  loadAudit()
})
</script>

<style scoped>
.audit-page {
  padding: 20px;
}

.filter-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  align-items: center;
}

.pagination-wrapper {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
  padding: 8px 0;
}
</style>
