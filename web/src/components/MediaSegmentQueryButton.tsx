import { useEffect, useId, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { CircleCheck, Database, ListVideo, Loader2, RefreshCw, X } from 'lucide-react'

import { playbackAPI, type MediaSegmentQueryResult } from '../api/playback'
import type { Media, PlaybackSegmentKind } from '../types'

const KIND_LABELS: Record<PlaybackSegmentKind, string> = { intro: '片头', credits: '片尾', recap: '前情回顾', preview: '预告' }

export function MediaSegmentQueryButton({ media }: { media: Media }) {
  const [open, setOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<MediaSegmentQueryResult | null>(null)
  const [error, setError] = useState('')
  const request = useRef<AbortController | null>(null)
  const dialogRef = useRef<HTMLDivElement>(null)
  const closeRef = useRef<HTMLButtonElement>(null)
  const titleId = useId()

  useEffect(() => () => { request.current?.abort() }, [])
  useEffect(() => {
    if (!open) return
    const previous = document.activeElement as HTMLElement | null
    const overflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    closeRef.current?.focus()
    return () => { document.body.style.overflow = overflow; previous?.focus() }
  }, [open])

  const close = () => {
    request.current?.abort()
    request.current = null
    setLoading(false)
    setOpen(false)
  }
  const query = async () => {
    if (request.current) return
    const controller = new AbortController()
    request.current = controller
    setLoading(true)
    setError('')
    try {
      const value = await playbackAPI.querySegments(media.id, controller.signal)
      if (!controller.signal.aborted) setResult(value)
    } catch (err: unknown) {
      if (!controller.signal.aborted) setError((err as { response?: { data?: { error?: string } } })?.response?.data?.error || '查询失败，请检查网络后重试；已有时间点保留')
    } finally {
      if (request.current === controller) { request.current = null; setLoading(false) }
    }
  }

  return <>
    <button type="button" aria-haspopup="dialog" onClick={() => { setOpen(true); void query() }} className="btn-outline h-9 rounded-lg px-3 text-xs gap-1.5 border-gray-200 hover:border-brand-500/50 hover:bg-brand-50">
      <ListVideo size={13} className="text-brand-500" /><span>查询片头片尾</span>
    </button>
    {open && createPortal(
      <div className="fixed inset-0 z-[110] flex items-center justify-center bg-black/35 p-4 backdrop-blur-sm" onClick={close}>
        <div
          ref={dialogRef} role="dialog" aria-modal="true" aria-labelledby={titleId}
          className="flex max-h-[90dvh] w-full max-w-lg flex-col overflow-hidden rounded-3xl border border-[var(--app-border)] bg-[var(--app-panel)] text-[var(--app-text)] shadow-2xl"
          onClick={event => event.stopPropagation()}
          onKeyDown={event => {
            if (event.key === 'Escape') { event.preventDefault(); close() }
            if (event.key !== 'Tab') return
            const targets = Array.from(dialogRef.current?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)') ?? [])
            const first = targets[0], last = targets[targets.length - 1]
            if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
            if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
          }}
        >
          <header className="flex shrink-0 items-start justify-between gap-3 border-b border-[var(--app-border)] p-5">
            <div><h2 id={titleId} className="font-display text-lg font-semibold">查询片头片尾</h2><p className="mt-1 break-words text-xs text-[var(--app-muted)]">{media.title}{media.season_num > 0 && media.episode_num > 0 ? ` · S${media.season_num}E${media.episode_num}` : ''}</p></div>
            <button type="button" ref={closeRef} aria-label="关闭" onClick={close} className="rounded-lg p-1.5 text-[var(--app-muted)] hover:bg-[var(--app-panel-soft)]"><X size={18} /></button>
          </header>
          <div className="min-h-0 space-y-4 overflow-y-auto p-5">
            <p className="text-xs leading-relaxed text-[var(--app-muted)]">向 TheIntroDB 查询时间点，不读取视频文件，无需 API Key。</p>
            {loading && <div role="status" className="flex items-center justify-center gap-2 py-8 text-sm text-[var(--app-muted)]"><Loader2 size={20} className="animate-spin" />正在查询…</div>}
            {error && <p role="alert" className="rounded-xl bg-red-500/10 p-3 text-sm text-red-600 dark:text-red-400">{error}</p>}
            {!loading && result && <div className="space-y-3" aria-live="polite">
              {result.found ? <>
                <p className="flex items-center gap-2 text-sm font-medium"><CircleCheck size={17} className="text-emerald-600" />{error ? '上次保存的时间点' : '已查到时间点'}</p>
                <div className="overflow-hidden rounded-xl border border-[var(--app-border)]">
                  <table className="w-full text-left text-sm"><caption className="sr-only">影片片头片尾时间点</caption><thead className="bg-[var(--app-panel-soft)] text-xs text-[var(--app-muted)]"><tr><th className="px-3 py-2 font-normal">类型</th><th className="px-3 py-2 font-normal">开始</th><th className="px-3 py-2 font-normal">结束</th></tr></thead><tbody>
                    {result.segments.map((segment, index) => <tr key={`${segment.kind}-${index}`} className="border-t border-[var(--app-border)]"><td className="px-3 py-2.5">{KIND_LABELS[segment.kind] || segment.kind}</td><td className="px-3 py-2.5 tabular-nums">{formatTime(segment.start_ms)}</td><td className="px-3 py-2.5 tabular-nums">{segment.end_ms > 0 ? formatTime(segment.end_ms) : '至片尾'}</td></tr>)}
                  </tbody></table>
                </div>
              </> : <p className="rounded-xl bg-[var(--app-panel-soft)] p-4 text-sm leading-relaxed">TheIntroDB 暂无这部影片的时间点，已记录查询结果。可以稍后重新查询。</p>}
              <p className="flex items-start gap-1.5 text-xs text-[var(--app-muted)]"><Database size={13} className="mt-0.5 shrink-0" /><span>已保存到数据库 · {new Date(result.fetched_at).toLocaleString('zh-CN', { hour12: false })}</span></p>
            </div>}
          </div>
          <footer className="flex shrink-0 justify-end gap-2 border-t border-[var(--app-border)] bg-[var(--app-panel-soft)] px-5 py-4">
            <button type="button" disabled={loading} onClick={() => { void query() }} className="btn-outline !px-3 !py-2 text-xs"><RefreshCw size={13} />重新查询</button>
            <button type="button" onClick={close} className="btn-primary !px-4 !py-2 text-xs">关闭</button>
          </footer>
        </div>
      </div>, document.body,
    )}
  </>
}

function formatTime(milliseconds: number): string {
  const total = Math.max(0, Math.floor(milliseconds / 1000))
  const seconds = String(total % 60).padStart(2, '0')
  const minutes = String(Math.floor(total / 60) % 60).padStart(2, '0')
  const hours = Math.floor(total / 3600)
  return hours > 0 ? `${hours}:${minutes}:${seconds}` : `${minutes}:${seconds}`
}
