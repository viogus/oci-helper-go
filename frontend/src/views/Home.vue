<template>
  <div>
    <h3>Dashboard</h3>
    <el-row :gutter="16">
      <el-col :span="8" v-for="stat in stats" :key="stat.label">
        <el-card shadow="hover">
          <div style="text-align:center">
            <div style="font-size:32px;font-weight:bold;color:#409eff">{{ stat.value }}</div>
            <div style="color:#909399;margin-top:8px">{{ stat.label }}</div>
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { reactive } from 'vue'

const stats = reactive([
  { label: 'Tenants', value: 0 },
  { label: 'Instances', value: 0 },
  { label: 'Running Tasks', value: 0 },
])
</script>
