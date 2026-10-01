<template>
  <div class="stats-table">
    <el-table v-if="rows.length" :data="rows" row-key="tenantId" v-loading="loading" style="width:100%">
      <!-- Expand: per-region breakdown, each region expands again to its instances -->
      <el-table-column type="expand">
        <template #default="{ row }">
          <el-table :data="row.regions || []" size="small" class="region-table">
            <el-table-column type="expand">
              <template #default="{ row: reg }">
                <el-table v-if="(reg.instances || []).length" :data="reg.instances" size="small" class="instance-table">
                  <el-table-column prop="instanceName" :label="$t('traffic.instance')" min-width="220" show-overflow-tooltip />
                  <el-table-column :label="$t('traffic.inbound')" width="150" align="right">
                    <template #default="{ row: inst }">{{ formatBytes(inst.inboundBytes) }}</template>
                  </el-table-column>
                  <el-table-column :label="$t('traffic.outbound')" width="150" align="right">
                    <template #default="{ row: inst }">{{ formatBytes(inst.outboundBytes) }}</template>
                  </el-table-column>
                </el-table>
                <el-empty v-else :description="$t('traffic.noData')" :image-size="40" />
              </template>
            </el-table-column>
            <el-table-column prop="region" :label="$t('traffic.region')" min-width="160" />
            <el-table-column prop="instanceCount" :label="$t('traffic.instanceCount')" width="100" align="right" />
            <el-table-column :label="$t('traffic.inboundTotal')" width="150" align="right">
              <template #default="{ row: reg }">{{ formatBytes(reg.inboundBytes) }}</template>
            </el-table-column>
            <el-table-column :label="$t('traffic.outboundTotal')" width="150" align="right">
              <template #default="{ row: reg }">{{ formatBytes(reg.outboundBytes) }}</template>
            </el-table-column>
            <el-table-column :label="$t('traffic.status')" width="130">
              <template #default="{ row: reg }">
                <el-tooltip v-if="reg.error" :content="reg.error" placement="top">
                  <el-tag type="danger" size="small">{{ $t('traffic.error') }}</el-tag>
                </el-tooltip>
                <el-tag v-else-if="reg.partial" type="warning" size="small">{{ $t('traffic.partialData') }}</el-tag>
                <el-tag v-else type="success" size="small">{{ $t('traffic.withinQuota') }}</el-tag>
              </template>
            </el-table-column>
          </el-table>
          <el-alert v-if="(row.errors || []).length" :title="(row.errors || []).join('; ')" type="warning" :closable="false" class="region-errors" />
        </template>
      </el-table-column>

      <el-table-column v-if="showAccount" prop="tenantName" :label="$t('traffic.account')" min-width="160" show-overflow-tooltip />
      <el-table-column prop="instanceCount" :label="$t('traffic.instanceCount')" width="100" align="right" />
      <el-table-column prop="regionCount" :label="$t('traffic.regionCount')" width="90" align="right" />
      <el-table-column :label="$t('traffic.inboundTotal')" width="140" align="right">
        <template #default="{ row }">{{ formatBytes(row.inboundBytes) }}</template>
      </el-table-column>
      <el-table-column :label="$t('traffic.outboundTotal')" width="140" align="right">
        <template #default="{ row }">{{ formatBytes(row.outboundBytes) }}</template>
      </el-table-column>
      <el-table-column :label="$t('traffic.quota')" width="130" align="right">
        <template #default="{ row }">{{ formatBytes(row.quotaBytes) }}</template>
      </el-table-column>
      <el-table-column :label="$t('traffic.quotaUsage')" width="200">
        <template #default="{ row }">
          <el-progress :percentage="quotaPercent(row)" :status="quotaProgressStatus(row)" :stroke-width="10" />
          <div class="quota-detail">{{ quotaPercentText(row) }}</div>
        </template>
      </el-table-column>
      <el-table-column :label="$t('traffic.status')" width="130">
        <template #default="{ row }">
          <el-tag v-if="row.exceeded" type="danger" size="small">{{ $t('traffic.exceeded') }}</el-tag>
          <el-tag v-else-if="row.partial" type="warning" size="small">{{ $t('traffic.partialData') }}</el-tag>
          <el-tag v-else type="success" size="small">{{ $t('traffic.withinQuota') }}</el-tag>
        </template>
      </el-table-column>
    </el-table>

    <el-empty v-else-if="!loading" :description="emptyText || $t('traffic.noData')" />
  </div>
</template>

<script setup>
import { formatBytes, quotaPercent, quotaPercentText, quotaProgressStatus } from '../utils/format.js'

defineProps({
  rows: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  // The monthly summary shows a single tenant, so the account column is redundant there.
  showAccount: { type: Boolean, default: true },
  emptyText: { type: String, default: '' },
})
</script>

<style scoped>
.stats-table { margin-top: 8px }
.region-table { margin: 0 12px 12px 48px; width: calc(100% - 60px) }
.instance-table { margin: 0 12px 12px 60px; width: calc(100% - 72px) }
.region-errors { margin: 0 12px 12px 48px; width: calc(100% - 60px) }
.quota-detail { font-size: 11px; color: var(--text-muted); margin-top: 2px }
</style>
