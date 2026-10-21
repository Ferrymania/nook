// Unknown source sizes are different from an empty library's zero total.
export function formatKnownVideoSize(bytes: number, knownCount: number, mediaCount: number): string {
  if (mediaCount === 0) return '0 B'
  if (knownCount === 0 || bytes <= 0) return '—'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return `${unit === 0 ? value : value.toFixed(2)} ${units[unit]}`
}

export function formatKnownDuration(seconds: number, mediaCount: number): string {
  if (mediaCount === 0) return '0 h'
  if (seconds <= 0) return '—'
  if (seconds < 60) return '< 1 min'
  if (seconds < 3600) return `${Math.floor(seconds / 60)} min`
  return `${Math.floor(seconds / 3600).toLocaleString('zh-CN')} h`
}
