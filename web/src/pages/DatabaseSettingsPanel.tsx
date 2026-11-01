import { useCallback, useEffect, useState } from 'react'
import { AlertCircle, CheckCircle2, Database, Loader2, RefreshCw } from 'lucide-react'

import { adminAPI, type DatabaseStatus } from '../api/admin'

export function DatabaseSettingsPanel() {
  const [status, setStatus] = useState<DatabaseStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')

  const refreshStatus = useCallback(async (signal?: AbortSignal) => {
    setLoading(true)
    setLoadError('')
    try {
      const nextStatus = await adminAPI.getDatabaseStatus(signal)
      if (!signal?.aborted) setStatus(nextStatus)
    } catch {
      if (!signal?.aborted) {
        setStatus(null)
        setLoadError('获取数据库状态失败，请稍后重试。')
      }
    } finally {
      if (!signal?.aborted) setLoading(false)
    }
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    void refreshStatus(controller.signal)
    return () => controller.abort()
  }, [refreshStatus])

  const connected = status?.connected === true
  const label = loading ? '检查中' : loadError ? '获取失败' : connected ? '连接正常' : '连接异常'
  const StatusIcon = loading ? Loader2 : connected && !loadError ? CheckCircle2 : AlertCircle
  const message = loadError || status?.error || status?.counts_error
  const unavailable = loading && !status ? '—' : '获取失败'
  const metric = (value: number | undefined) => connected ? value ?? unavailable : unavailable

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Database className="h-5 w-5 text-brand-500" />
          <h2 className="font-display text-lg font-semibold text-ink-600">数据库状态</h2>
        </div>
        <button
          type="button"
          onClick={() => void refreshStatus()}
          disabled={loading}
          className="library-toolbar-button disabled:opacity-50"
        >
          <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          刷新状态
        </button>
      </div>

      <section className="glass-panel space-y-4 p-5" aria-label="PostgreSQL 运行状态" aria-busy={loading}>
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 pb-3">
          <h3 className="text-sm font-medium text-ink-600">PostgreSQL</h3>
          <span
            role="status"
            className={'inline-flex items-center gap-1.5 text-xs font-medium ' +
              (loading ? 'text-ink-100' : connected && !loadError ? 'text-emerald-700 dark:text-emerald-400' : 'text-rose-700 dark:text-rose-400')}
          >
            <StatusIcon size={14} className={loading ? 'animate-spin' : ''} />
            {label}
          </span>
        </div>

        {message && !loading && <p role="alert" className="text-sm text-ink-100">{message}</p>}

        <dl className="grid grid-cols-1 gap-4 text-sm md:grid-cols-2 xl:grid-cols-3">
          <div className="min-w-0 space-y-2 md:col-span-2 xl:col-span-1">
            <dt className="text-ink-100">连接地址</dt>
            <dd className="break-all font-mono text-xs leading-5 text-ink-600">
              {status?.dsn || (loading ? '—' : '获取失败')}
            </dd>
          </div>
          <div className="min-w-0 space-y-2">
            <dt className="text-ink-100">连接池</dt>
            <dd className="flex flex-wrap gap-x-3 gap-y-1 tabular-nums text-ink-600">
              {connected ? <>
                <span>活跃 {metric(status?.in_use)}</span>
                <span>空闲 {metric(status?.idle)}</span>
                <span>上限 {metric(status?.max_open_conns)}</span>
              </> : unavailable}
            </dd>
          </div>
          <div className="min-w-0 space-y-2">
            <dt className="text-ink-100">记录数</dt>
            <dd className="flex flex-wrap gap-x-3 gap-y-1 tabular-nums text-ink-600">
              {connected ? <>
                <span>媒体 {metric(status?.table_counts?.media)}</span>
                <span>用户 {metric(status?.table_counts?.users)}</span>
                <span>播放记录 {metric(status?.table_counts?.playback_histories)}</span>
              </> : unavailable}
            </dd>
          </div>
        </dl>
      </section>
    </div>
  )
}
