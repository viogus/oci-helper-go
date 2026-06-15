<template>
  <div>
    <h3>Cloud Shell / VNC Console</h3>
    <el-card>
      <el-empty description="VNC console requires websockify proxy (port 6080).">
        <template v-if="vncUrl">
          <p style="margin-bottom:12px">Console URL: <code>{{ vncUrl }}</code></p>
          <el-button type="primary" @click="openConsole">Open Console</el-button>
        </template>
        <template v-else>
          <p style="color:#909399">Configure a tenant and instance to start a VNC session.</p>
        </template>
      </el-empty>
    </el-card>
  </div>
</template>

<script setup>
import { ref } from 'vue'

const vncUrl = ref('')

function openConsole() {
  if (vncUrl.value) window.open(vncUrl.value, '_blank')
}
</script>
