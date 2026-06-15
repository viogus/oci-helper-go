<template>
  <div class="ip-info-page">
    <el-card class="ip-card" shadow="always">
      <h2 style="text-align:center;margin-bottom:24px">IP Information</h2>
      <el-descriptions :column="1" border v-if="info">
        <el-descriptions-item label="Your IP">{{ info.ip }}</el-descriptions-item>
        <el-descriptions-item label="X-Forwarded-For">{{ info.forwarded || 'N/A' }}</el-descriptions-item>
        <el-descriptions-item label="User Agent">{{ info.userAgent || 'N/A' }}</el-descriptions-item>
      </el-descriptions>
      <div v-else style="text-align:center;padding:40px">
        <el-icon :size="48"><Loading /></el-icon>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import api from '../api/index.js'

const info = ref(null)

onMounted(async () => {
  try {
    const r = await api.get('/ip-info')
    // /api/ip-info returns {ip: "..."}
    // Also capture from browser
    info.value = {
      ip: r.ip || r.data?.ip || 'Unknown',
      forwarded: r.forwarded || 'N/A',
      userAgent: navigator.userAgent,
    }
  } catch {
    info.value = { ip: 'Error loading', forwarded: 'N/A', userAgent: navigator.userAgent }
  }
})
</script>

<style scoped>
.ip-info-page {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 60vh;
}
.ip-card { width: 500px; }
</style>
