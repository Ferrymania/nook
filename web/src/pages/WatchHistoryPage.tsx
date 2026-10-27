import { useEffect, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { ArrowRight, BarChart3, Clock, Play, Trash2 } from 'lucide-react'
import toast from 'react-hot-toast'

import { historyAPI } from '../api/history'
import { ARTWORK, imageURL } from '../api/client'
import { confirmAction } from '../components/confirmAction'
import { isRemoteEmbyID } from '../utils/remoteEmby'
import type { HistoryItem, Media } from '../types'

function fmtDuration(ms: number): string {
  if (!ms || ms <= 0) return '—'
  const sec = Math.floor(ms / 1000)
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return h > 0 ? `${h}h ${m}m` : `${m}m`
}

export function WatchHistoryPage() {
  return <WatchHistoryPanel />
}

// The personal homepage previews recent history; /history keeps the full list
// and its bulk actions. Both views use the same history data and resume links.
export function WatchHistoryPanel({ embedded = false }: { embedded?: boolean }) {
  const location = useLocation()
  const [items, setItems] = useState<HistoryItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [reloadKey, setReloadKey] = useState(0)
  const [busy, setBusy] = useState('')
  const limit = embedded ? 5 : 200
  const from = embedded ? `${location.pathname}${location.search}` : '/history'
  const Heading = embedded ? 'h2' : 'h1'

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError('')
    historyAPI
      .list(limit)
      .then((rows) => { if (!cancelled) setItems(rows) })
      .catch((err: unknown) => {
        const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '加载观看历史失败'
        if (!cancelled) setError(msg)
      })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [limit, reloadKey])

  const removeOne = async (id: string) => {
    if (!(await confirmAction({ title: '移除观看历史', message: '确定移除此条观看历史？不会删除媒体文件。', confirmText: '移除' }))) return
    setBusy(id)
    try {
      await historyAPI.remove(id)
      setItems((prev) => prev.filter((item) => item.id !== id))
      toast.success('已移除观看历史')
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '移除观看历史失败'
      toast.error(msg)
    } finally {
      setBusy('')
    }
  }

  const clearByStatus = async (status: 'completed' | 'incomplete') => {
    const label = status === 'completed' ? '已看完' : '未看完'
    if (!(await confirmAction({ title: '清除观看历史', message: `确定清除所有${label}的观看历史？不会删除媒体文件。`, confirmText: '清除' }))) return
    setBusy(status)
    try {
      await historyAPI.clear(undefined, status)
      setItems((prev) => prev.filter((item) => status === 'completed' ? !item.completed : item.completed))
      toast.success(`已清除${label}记录`)
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? `清除${label}记录失败`
      toast.error(msg)
    } finally {
      setBusy('')
    }
  }

  return (
    <section aria-labelledby="watch-history-heading" className={embedded ? 'space-y-3 rounded-xl bg-(--app-panel) p-4 sm:p-5' : 'space-y-6'}>
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          {!embedded && <Clock className="h-6 w-6 text-brand-500" />}
          <Heading id="watch-history-heading" className={embedded ? 'text-base font-semibold' : 'font-display text-3xl font-bold text-ink-600'}>观看历史</Heading>
        </div>
        {embedded ? (
          <Link to="/history" className="inline-flex min-h-11 items-center gap-1.5 text-sm text-(--app-muted) transition-colors hover:text-(--app-text)">
            查看全部 <ArrowRight size={14} />
          </Link>
        ) : <div className="flex flex-wrap items-center gap-2">
          <Link
            to="/history/stats"
            className="inline-flex items-center gap-1.5 rounded-xl border border-gray-200 bg-white px-3 py-2 text-xs font-bold text-gray-600 shadow-xs transition hover:border-brand-300 hover:text-brand-600"
          >
            <BarChart3 size={14} />
            观看统计
          </Link>
          <button
            onClick={() => clearByStatus('incomplete')}
            disabled={busy !== '' || items.every((item) => item.completed)}
            className="rounded-xl border border-gray-200 bg-white px-3 py-2 text-xs font-bold text-gray-600 shadow-xs transition hover:border-amber-300 hover:text-amber-600 disabled:cursor-not-allowed disabled:opacity-45"
          >
            清除未看完
          </button>
          <button
            onClick={() => clearByStatus('completed')}
            disabled={busy !== '' || items.every((item) => !item.completed)}
            className="rounded-xl border border-gray-200 bg-white px-3 py-2 text-xs font-bold text-gray-600 shadow-xs transition hover:border-emerald-300 hover:text-emerald-600 disabled:cursor-not-allowed disabled:opacity-45"
          >
            清除已看完
          </button>
        </div>}
      </header>

      {loading && <p role="status" className="py-4 text-sm text-(--app-muted)">加载中…</p>}
      {!loading && error && (
        <div role="alert" className="flex flex-wrap items-center gap-3 py-4 text-sm text-(--app-muted)">
          <p>{error}</p>
          <button type="button" className="min-h-11 text-(--app-text) underline underline-offset-4" onClick={() => setReloadKey((key) => key + 1)}>重试</button>
        </div>
      )}
      {!loading && !error && items.length === 0 && (
        <p className="py-4 text-sm text-(--app-muted)">暂无观看记录。</p>
      )}

      <div className={embedded ? 'divide-y divide-(--app-border)' : 'space-y-3'}>
        {!loading && !error && items.map((h) => {
          const m: Media = h.media || ({
            id: h.media_id,
            title: isRemoteEmbyID(h.media_id) ? '远程媒体' : h.media_id,
            poster_url: '',
            updated_at: h.watched_at,
          } as Media)
          const progress =
            h.duration_ms > 0 ? h.position_ms / h.duration_ms : 0
          return (
            <div
              key={h.id}
              className={embedded ? 'flex items-center gap-3 py-3' : 'glass-panel flex flex-col gap-3 p-3! sm:flex-row sm:items-center sm:gap-4'}
            >
              <div className="h-16 w-12 shrink-0 overflow-hidden rounded-lg bg-surface-900">
                {m.poster_url ? (
                  <img
                    src={imageURL(m.poster_url, m.updated_at, ARTWORK.posterCard)}
                    alt={m.title}
                    className="h-full w-full object-cover"
                    referrerPolicy="no-referrer"
                  />
                ) : null}
              </div>
              <div className="min-w-0 flex-1 space-y-1">
                <Link
                  to={`/media/${m.id}`}
                  state={{ from }}
                  className="block truncate font-medium text-ink-600 transition hover:text-brand-500"
                >
                  {m.title}
                </Link>
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-ink-50">
                  <span>{fmtDuration(h.position_ms)} / {fmtDuration(h.duration_ms)}</span>
                  <time dateTime={h.watched_at}>{new Date(h.watched_at).toLocaleString()}</time>
                  {h.completed && (
                    <span className="rounded-lg border border-emerald-400/40 px-1.5 py-0.5 text-emerald-400">
                      已看完
                    </span>
                  )}
                </div>
                <div className="h-1 w-full overflow-hidden rounded-lg bg-gray-200">
                  <div
                    className="h-full bg-primary-400"
                    style={{ width: `${Math.min(100, Math.max(0, Math.round(progress * 100)))}%` }}
                  />
                </div>
              </div>
              <div className={embedded ? 'shrink-0' : 'flex w-full items-center gap-2 sm:w-auto sm:shrink-0'}>
                <Link
                  to={`/play/${m.id}`}
                  state={{ from }}
                  className={embedded ? 'inline-flex min-h-11 items-center gap-1.5 rounded-full border border-(--app-border) px-3 text-sm transition-colors hover:bg-(--app-hover)' : 'neon-button px-3! py-1! text-xs!'}
                >
                  <Play size={12} /> {embedded && h.completed ? '播放' : '继续'}
                </Link>
                {!embedded && <button
                  onClick={() => removeOne(h.id)}
                  disabled={busy === h.id}
                  className="inline-flex items-center gap-1 rounded-xl border border-gray-200 bg-white px-3 py-1.5 text-xs font-bold text-gray-500 transition hover:border-red-200 hover:bg-red-50 hover:text-red-500 disabled:opacity-50"
                  title="移除此条观看历史"
                >
                  <Trash2 size={12} />
                  移除
                </button>}
              </div>
            </div>
          )
        })}
      </div>
    </section>
  )
}
