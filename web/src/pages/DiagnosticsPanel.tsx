import { useEffect, useRef, useState } from 'react'
import { ChevronRight, Download, Loader2, RefreshCw } from 'lucide-react'

import { diagnosticsAPI, diagnosticsError, type DiagnosticsProfile, type DiagnosticsStatus } from '../api/diagnostics'

const PROFILES: { key: DiagnosticsProfile; label: string }[] = [
  { key: 'heap', label: '内存快照' },
  { key: 'allocs', label: '分配快照' },
  { key: 'goroutine', label: '协程快照' },
  { key: 'profile', label: 'CPU 采集 10 秒' },
]

export function DiagnosticsPanel() {
  const [expanded, setExpanded] = useState(false)
  const [status, setStatus] = useState<DiagnosticsStatus | null>(null)
  const [loading, setLoading] = useState(false)
  const [changing, setChanging] = useState(false)
  const [capturing, setCapturing] = useState<DiagnosticsProfile | null>(null)
  const [error, setError] = useState('')
  const [downloaded, setDownloaded] = useState('')
  const [now, setNow] = useState(Date.now())
  const captureController = useRef<AbortController | null>(null)
  const statusController = useRef<AbortController | null>(null)
  const mutating = useRef(false)

  useEffect(() => () => {
    captureController.current?.abort()
    statusController.current?.abort()
  }, [])

  useEffect(() => {
    if (!expanded) return
    let disposed = false
    let pending = false
    const load = async () => {
      if (pending || mutating.current) return
      pending = true
      const controller = new AbortController()
      statusController.current = controller
      try {
        const value = await diagnosticsAPI.status(controller.signal)
        if (!disposed && !controller.signal.aborted && !mutating.current) {
          setStatus(value)
          setNow(Date.now())
          setError('')
        }
      } catch (err) {
        if (!disposed && !controller.signal.aborted) setError(await diagnosticsError(err))
      } finally {
        pending = false
        if (!disposed) setLoading(false)
      }
    }
    setLoading(true)
    void load()
    const poll = window.setInterval(() => { void load() }, 5000)
    const clock = window.setInterval(() => setNow(Date.now()), 1000)
    return () => {
      disposed = true
      statusController.current?.abort()
      window.clearInterval(poll)
      window.clearInterval(clock)
    }
  }, [expanded])

  const change = async (enable: boolean) => {
    mutating.current = true
    statusController.current?.abort()
    setChanging(true)
    setError('')
    try {
      setStatus(await (enable ? diagnosticsAPI.enable() : diagnosticsAPI.disable()))
      setNow(Date.now())
    } catch (err) {
      setError(await diagnosticsError(err))
    } finally {
      mutating.current = false
      setChanging(false)
    }
  }

  const capture = async (profile: DiagnosticsProfile) => {
    const controller = new AbortController()
    captureController.current = controller
    setCapturing(profile)
    setError('')
    setDownloaded('')
    try {
      const { blob, filename } = await diagnosticsAPI.capture(profile, controller.signal)
      if (controller.signal.aborted) return
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = filename
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      window.setTimeout(() => URL.revokeObjectURL(url), 1000)
      setDownloaded(filename)
    } catch (err) {
      if (!controller.signal.aborted) setError(await diagnosticsError(err))
    } finally {
      if (!controller.signal.aborted) {
        setCapturing(null)
        diagnosticsAPI.status().then(setStatus).catch(() => undefined)
      }
      if (captureController.current === controller) captureController.current = null
    }
  }

  const expires = status?.expires_at ? new Date(status.expires_at).getTime() : 0
  const remaining = Math.max(0, Math.ceil((expires - now) / 1000))
  const enabled = Boolean(status?.enabled && remaining > 0)
  const busy = Boolean(capturing || status?.capturing)
  const stateLabel = loading && !status ? '加载中…' : !status ? '状态未知' : enabled
    ? `已开启 · ${Math.floor(remaining / 60)}:${String(remaining % 60).padStart(2, '0')} 后关闭`
    : '已关闭'

  return (
    <details className="about-panel about-diagnostics" onToggle={(event) => setExpanded(event.currentTarget.open)}>
      <summary className="about-diagnostics-heading">
        <h3>运行诊断</h3>
        <ChevronRight size={18} aria-hidden="true" className={`shrink-0 text-(--app-muted) transition-transform ${expanded ? 'rotate-90' : ''}`} />
      </summary>
      {expanded && (
        <div className="mt-5 space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p role="status" className="text-sm text-(--app-text)">{stateLabel}{busy && ' · 采集中…'}</p>
            <button type="button" className="about-button" disabled={changing || !status}
              onClick={() => { void change(!enabled) }}>
              {changing && <Loader2 size={15} className="animate-spin" aria-hidden="true" />}
              {enabled ? '关闭诊断' : '开启 15 分钟'}
            </button>
          </div>
          <div className="flex flex-wrap gap-2" aria-busy={busy}>
            {PROFILES.map((profile) => (
              <button key={profile.key} type="button" className="about-button"
                disabled={!enabled || busy || changing} onClick={() => { void capture(profile.key) }}>
                {capturing === profile.key ? <Loader2 size={15} className="animate-spin" aria-hidden="true" /> : <Download size={15} aria-hidden="true" />}
                {profile.label}
              </button>
            ))}
          </div>
          <p className="text-sm text-(--app-muted)">开启后可下载 pprof 文件；采集有额外开销，重启后自动关闭。</p>
          {downloaded && <p role="status" className="break-all text-sm text-(--app-muted)">已下载 {downloaded}</p>}
          {error && (
            <div role="alert" className="flex flex-wrap items-center gap-3 text-sm text-red-600 dark:text-red-400">
              <p>{error}</p>
              <button type="button" className="inline-flex min-h-11 items-center gap-1.5" onClick={() => {
                setError('')
                diagnosticsAPI.status().then(setStatus).catch(async (err) => setError(await diagnosticsError(err)))
              }}><RefreshCw size={14} aria-hidden="true" />重试</button>
            </div>
          )}
        </div>
      )}
    </details>
  )
}
