// Shared byte formatting for the traffic views (account rows, region and instance tables).
export function formatBytes(bytes) {
  const n = Number(bytes) || 0
  if (n >= 1024 ** 5) return (n / 1024 ** 5).toFixed(2) + ' PiB'
  if (n >= 1024 ** 4) return (n / 1024 ** 4).toFixed(2) + ' TiB'
  if (n >= 1024 ** 3) return (n / 1024 ** 3).toFixed(2) + ' GiB'
  if (n >= 1024 ** 2) return (n / 1024 ** 2).toFixed(2) + ' MiB'
  if (n >= 1024) return (n / 1024).toFixed(2) + ' KiB'
  return n.toFixed(0) + ' B'
}

// Percent of the free egress allowance consumed, clamped for the progress bar.
export function quotaPercent(row) {
  const p = Number(row?.quotaPercent)
  if (!Number.isFinite(p) || p <= 0) return 0
  return Math.min(100, Math.round(p * 10) / 10)
}

export function quotaPercentText(row) {
  const p = Number(row?.quotaPercent)
  if (!Number.isFinite(p) || p <= 0) return '0.0%'
  return p.toFixed(1) + '%'
}

// The progress bar mirrors the row status: over allowance, understated, or clean.
export function quotaProgressStatus(row) {
  if (row?.exceeded) return 'exception'
  if (row?.partial) return 'warning'
  return 'success'
}
