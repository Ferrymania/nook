import { useCallback, useEffect, useId, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Link } from 'react-router-dom'
import { CircleCheck, Database, ListVideo, Loader2, RefreshCw, X } from 'lucide-react'

import { playbackAPI, type MediaSegmentQueryResult } from '../api/playback'
import type { Media, PlaybackSegmentKind } from '../types'
import { formatEpisodeNumber } from '../utils/episodeNumber'
import { querySegmentBatch, segmentQueryError, segmentQueryTargets, type SegmentBatchResult } from '../utils/segmentQueryBatch'

const KIND_LABELS: Record<PlaybackSegmentKind, string> = { intro: '片头', credits: '片尾', recap: '前情回顾', preview: '预告' }

type MediaSegmentQueryProps = { media: Media; episodes?: Media[] }

export function MediaSegmentQueryButton({ media, episodes, disabled = false }: MediaSegmentQueryProps & { disabled?: boolean }) {
  const [open, setOpen] = useState(false)
  return <>
    <button type="button" disabled={disabled || (episodes !== undefined && segmentQueryTargets(episodes).length === 0)} aria-haspopup="dialog" onClick={() => setOpen(true)} className="media-management-action">
      <ListVideo size={14} aria-hidden="true" /><span>{episodes !== undefined ? '查询整剧片头片尾' : '查询片头片尾'}</span>
    </button>
    {open && <MediaSegmentQueryDialog media={media} episodes={episodes} onClose={() => setOpen(false)} />}
  </>
}

export function MediaSegmentQueryDialog({ media, episodes, onClose }: MediaSegmentQueryProps & { onClose: () => void }) {
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<MediaSegmentQueryResult | null>(null)
  const [error, setError] = useState('')
  const [batchResults, setBatchResults] = useState<SegmentBatchResult[]>([])
  const request = useRef<AbortController | null>(null)
  const dialogRef = useRef<HTMLDivElement>(null)
  const closeRef = useRef<HTMLButtonElement>(null)
  const titleId = useId()
  const isBatch = episodes !== undefined
  // 查询范围固定为打开弹窗时的剧集，避免父页面刷新时重启整批查询。
  const [targets] = useState(() => segmentQueryTargets(episodes ?? []))
  const title = isBatch ? '查询整剧片头片尾' : '查询片头片尾'
  const found = batchResults.filter(row => row.status === 'found').length
  const missing = batchResults.filter(row => row.status === 'missing').length
  const failed = batchResults.filter(row => row.status === 'failed').length

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    const overflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    closeRef.current?.focus()
    return () => { document.body.style.overflow = overflow; previous?.focus() }
  }, [])

  const close = () => {
    request.current?.abort()
    request.current = null
    onClose()
  }
  const query = useCallback(async () => {
    if (request.current) return
    const controller = new AbortController()
    request.current = controller
    setLoading(true)
    setError('')
    if (isBatch) setBatchResults([])
    try {
      if (isBatch) {
        await querySegmentBatch(targets, playbackAPI.querySegments, controller.signal, setBatchResults)
        return
      }
      const value = await playbackAPI.querySegments(media.id, controller.signal)
      if (!controller.signal.aborted) setResult(value)
    } catch (err: unknown) {
      if (!controller.signal.aborted) setError(segmentQueryError(err))
    } finally {
      if (request.current === controller) { request.current = null; setLoading(false) }
    }
  }, [isBatch, media.id, targets])

  useEffect(() => {
    void query()
    return () => { request.current?.abort(); request.current = null }
  }, [query])

  return createPortal(
      <div className="fixed inset-0 z-110 flex items-center justify-center bg-black/35 p-4 backdrop-blur-xs" onClick={close}>
        <div
          ref={dialogRef} role="dialog" aria-modal="true" aria-labelledby={titleId}
          className="flex max-h-[90dvh] w-full max-w-lg flex-col overflow-hidden rounded-3xl border border-(--app-border) bg-(--app-panel) text-(--app-text) shadow-2xl"
          onClick={event => event.stopPropagation()}
          onKeyDown={event => {
            if (event.key === 'Escape') { event.preventDefault(); close() }
            if (event.key !== 'Tab') return
            const focusable = Array.from(dialogRef.current?.querySelectorAll<HTMLElement>('button:not(:disabled), a[href]') ?? [])
            const first = focusable[0], last = focusable[focusable.length - 1]
            if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
            if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
          }}
        >
          <header className="flex shrink-0 items-start justify-between gap-3 border-b border-(--app-border) p-5">
            <div><h2 id={titleId} className="font-display text-lg font-semibold">{title}</h2><p className="mt-1 wrap-break-word text-xs text-(--app-muted)">{media.title}{isBatch ? ` · ${targets.length} 个媒体` : media.episode_num > 0 ? ` · S${media.season_num}E${formatEpisodeNumber(media)}` : ''}</p></div>
            <button type="button" ref={closeRef} aria-label="关闭" onClick={close} className="rounded-lg p-1.5 text-(--app-muted) hover:bg-(--app-panel-soft)"><X size={18} /></button>
          </header>
          <div className="min-h-0 space-y-4 overflow-y-auto p-5">
            <p className="text-xs leading-relaxed text-(--app-muted)">向 TheIntroDB 查询时间点，不读取视频文件，无需 API Key。</p>
            {isBatch && <div className="space-y-3">
              <div role="status" aria-live="polite" className="space-y-2 text-sm">
                <p className="flex items-center gap-2">{loading && <Loader2 size={16} className="animate-spin" aria-hidden="true" />}{loading ? '正在逐集查询' : batchResults.length === targets.length ? '查询完成' : '查询已停止'} · {batchResults.length} / {targets.length}</p>
                <p className="text-xs text-(--app-muted)">已找到 {found} · 未找到 {missing} · 失败 {failed}</p>
              </div>
              <p className="text-xs leading-relaxed text-(--app-muted)">查询记录可在片头片尾队列查看。关闭窗口会停止后续查询，当前查询的结果仍会保存。</p>
              {batchResults.length > 0 && <ul className="divide-y divide-(--app-border) rounded-xl border border-(--app-border) px-3">
                {batchResults.map(row => <li key={row.media.id} className="py-3 text-xs">
                  <div className="flex items-start justify-between gap-3"><span className="min-w-0 wrap-break-word">S{row.media.season_num}E{formatEpisodeNumber(row.media) || '—'} · {row.media.title}</span><span className={`shrink-0 ${row.status === 'failed' ? 'text-(--app-danger-text)' : 'text-(--app-muted)'}`}>{row.status === 'found' ? '已找到' : row.status === 'missing' ? '未找到' : '失败'}</span></div>
                  {row.error && <p className="mt-1.5 wrap-break-word text-(--app-danger-text)">{row.error}</p>}
                </li>)}
              </ul>}
            </div>}
            {loading && !isBatch && <div role="status" className="flex items-center justify-center gap-2 py-8 text-sm text-(--app-muted)"><Loader2 size={20} className="animate-spin" />正在查询…</div>}
            {error && <p role="alert" className="rounded-xl bg-red-500/10 p-3 text-sm text-red-600 dark:text-red-400">{error}</p>}
            {!isBatch && !loading && result && <div className="space-y-3" aria-live="polite">
              {result.found ? <>
                <p className="flex items-center gap-2 text-sm font-medium"><CircleCheck size={17} className="text-emerald-600" />{error ? '上次保存的时间点' : '已查到时间点'}</p>
                <div className="overflow-hidden rounded-xl border border-(--app-border)">
                  <table className="w-full text-left text-sm"><caption className="sr-only">影片片头片尾时间点</caption><thead className="bg-(--app-panel-soft) text-xs text-(--app-muted)"><tr><th className="px-3 py-2 font-normal">类型</th><th className="px-3 py-2 font-normal">开始</th><th className="px-3 py-2 font-normal">结束</th></tr></thead><tbody>
                    {result.segments.map((segment, index) => <tr key={`${segment.kind}-${index}`} className="border-t border-(--app-border)"><td className="px-3 py-2.5">{KIND_LABELS[segment.kind] || segment.kind}</td><td className="px-3 py-2.5 tabular-nums">{formatTime(segment.start_ms)}</td><td className="px-3 py-2.5 tabular-nums">{segment.end_ms > 0 ? formatTime(segment.end_ms) : '至片尾'}</td></tr>)}
                  </tbody></table>
                </div>
              </> : <p className="rounded-xl bg-(--app-panel-soft) p-4 text-sm leading-relaxed">TheIntroDB 暂无这部影片的时间点，已记录查询结果。可以稍后重新查询。</p>}
              <p className="flex items-start gap-1.5 text-xs text-(--app-muted)"><Database size={13} className="mt-0.5 shrink-0" /><span>已保存到数据库 · {new Date(result.fetched_at).toLocaleString('zh-CN', { hour12: false })}</span></p>
            </div>}
          </div>
          <footer className="flex shrink-0 flex-wrap justify-end gap-2 border-t border-(--app-border) bg-(--app-panel-soft) px-5 py-4">
            <Link to="/queue?type=segments" onClick={close} className="mr-auto self-center text-xs text-brand-600 hover:underline">查看查询队列</Link>
            <button type="button" disabled={loading} onClick={() => { void query() }} className="btn-outline px-3! py-2! text-xs"><RefreshCw size={13} />重新查询</button>
            <button type="button" onClick={close} className="btn-primary px-4! py-2! text-xs">{isBatch && loading ? '停止并关闭' : '关闭'}</button>
          </footer>
        </div>
      </div>, document.body,
    )
}

function formatTime(milliseconds: number): string {
  const total = Math.max(0, Math.floor(milliseconds / 1000))
  const seconds = String(total % 60).padStart(2, '0')
  const minutes = String(Math.floor(total / 60) % 60).padStart(2, '0')
  const hours = Math.floor(total / 3600)
  return hours > 0 ? `${hours}:${minutes}:${seconds}` : `${minutes}:${seconds}`
}
