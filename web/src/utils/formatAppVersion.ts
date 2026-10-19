export function formatAppVersion(version?: string): string {
  if (!version) return '未知'
  const normalized = version.trim().replace(/^nook-/, '').replace(/^v(?=\d)/, '')
  return /^\d+\.\d+\.\d+/.test(normalized) ? `v${normalized}` : normalized
}
