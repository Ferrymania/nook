import type { LibraryScanTask } from '../api/library'

export function scanTaskIsOlder(current: LibraryScanTask | null, next: LibraryScanTask): boolean {
  if (!current) return false
  if (current.id === next.id) {
    if (current.status !== 'running' && next.status === 'running') return true
    return Date.parse(next.updated_at) < Date.parse(current.updated_at)
  }
  return Date.parse(next.started_at) < Date.parse(current.started_at)
}

export function scanTaskMessage(task: LibraryScanTask): string {
  const metrics = task.metrics
  if (task.status === 'failed') return `扫描失败：${task.error || task.message || '请查看日志后重试'}`
  const counts = metrics ? `已扫描 ${metrics.visited ?? 0} 项 · 新增 ${metrics.added ?? 0} · 更新 ${metrics.updated ?? 0}` : ''
  if (task.status === 'running') return counts ? `正在扫描：${counts}` : '正在扫描当前媒体库…'
  return `扫描完成${counts ? `：${counts}` : ''}${metrics?.errors ? ` · ${metrics.errors} 项异常` : ''}`
}
