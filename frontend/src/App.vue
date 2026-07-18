<template>
  <el-config-provider :locale="elLocale">
    <router-view />
  </el-config-provider>
</template>

<script setup>
import { computed } from 'vue'
import zhCn from 'element-plus/dist/locale/zh-cn.mjs'
import en from 'element-plus/dist/locale/en.mjs'

const savedLocale = localStorage.getItem('locale') || 'zh-CN'
const elLocale = computed(() => savedLocale === 'en' ? en : zhCn)
</script>
