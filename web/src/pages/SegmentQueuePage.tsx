import { Fragment, useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { AlertCircle, CheckCircle2, ChevronDown, ChevronRight, Clock, Database, ListVideo, Loader2, MinusCircle, RefreshCw, Trash2 } from 'lucide-react'
import toast from 'react-hot-toast'

import { segmentQueueAPI, type SegmentTask, type SegmentTaskStatus, type SegmentQueueSnapshot } from '../api/segments'
import { QueuePagination, QueueSearch, QueueStatusFilters, QueueTableMessage, type QueueFilter } from './LibraryQueueControls'
import { apiErrorMessage, formatTime } from './StrmManagePage'

const STATUS = {
  running: { label: '查询中', icon: Loader2, tone: 'progress' },
  found: { label: '查到时间点', icon: CheckCircle2, tone: 'success' },
  missing: { label: '暂无数据', icon: MinusCircle, tone: 'muted' },
  failed: { label: '查询失败', icon: AlertCircle, tone: 'danger' },
} as const
const FILTERS: QueueFilter<'all' | SegmentTaskStatus>[] = [
  { key: 'all', label: '全部', icon: ListVideo, tone: 'neutral' },
  ...Object.entries(STATUS).map(([key, value]) => ({ ...value, key: key as SegmentTaskStatus })),
]
const TRIGGERS: Record<string, string> = { manual: '手动查询', playback: '播放时查询', prewarm: '后台预取', retry: '重新查询', history: '历史查询' }
const KINDS: Record<string, string> = { intro: '片头', credits: '片尾', recap: '前情回顾', preview: '预告' }

function timestamp(ms: number) {
  const seconds = Math.max(0, Math.floor(ms / 1000))
  const hh = Math.floor(seconds / 3600)
  const mm = String(Math.floor(seconds / 60) % 60).padStart(2, '0')
  const ss = String(seconds % 60).padStart(2, '0')
  return hh > 0 ? `${hh}:${mm}:${ss}` : `${mm}:${ss}`
}
function spanText(span: NonNullable<SegmentTask['segments']>[number]) {
  return span.end_ms > 0 ? `${timestamp(span.start_ms)} – ${timestamp(span.end_ms)}` : `${timestamp(span.start_ms)} 至片尾`
}

export function SegmentQueuePanel() {
  const [snapshot, setSnapshot] = useState<SegmentQueueSnapshot | null>(null)
  const [filter, setFilter] = useState<'all' | SegmentTaskStatus>('all')
  const [input, setInput] = useState('')
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(1)
  const [expanded, setExpanded] = useState<string | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState('')
  const sequence = useRef(0)
  const request = useRef<AbortController | null>(null)

  useEffect(() => {
    const timer = setTimeout(() => { setQuery(input.trim()); setPage(1) }, 300)
    return () => clearTimeout(timer)
  }, [input])

  const refresh = useCallback(async (manual = false) => {
    const current = ++sequence.current
    request.current?.abort()
    const controller = new AbortController()
    request.current = controller
    if (manual) setRefreshing(true)
    try {
      const data = await segmentQueueAPI.list(filter, query, page, controller.signal)
      if (current !== sequence.current || controller.signal.aborted) return
      const max = Math.max(1, Math.ceil(data.total / 50))
      if (page > max) { setPage(max); return }
      setSnapshot(data)
      setError('')
    } catch (err) {
      if (current === sequence.current && !controller.signal.aborted) setError(apiErrorMessage(err))
    } finally {
      if (current === sequence.current) setRefreshing(false)
    }
  }, [filter, query, page])

  const cancelRefresh = useCallback(() => {
    sequence.current++
    request.current?.abort()
  }, [])

  useEffect(() => {
    void refresh()
    const timer = setInterval(() => { if (!document.hidden) void refresh() }, 3000)
    return () => { clearInterval(timer); cancelRefresh() }
  }, [refresh, cancelRefresh])

  const action = async (task: SegmentTask, kind: 'retry' | 'remove') => {
    if (busy) return
    setBusy(task.id)
    try {
      if (kind === 'retry') { await segmentQueueAPI.retry(task.id); toast.success('查询完成，已记录本次结果') }
      else { await segmentQueueAPI.remove(task.id); toast.success('已删除查询记录，保存的时间点不受影响') }
    } catch (err) { toast.error(apiErrorMessage(err)) }
    finally { setBusy(null); void refresh() }
  }
  const clear = async () => {
    if (busy || !window.confirm('清理所有已结束的查询记录？正在查询的任务和已保存的片头片尾时间点都会保留。')) return
    setBusy('clear')
    try { const result = await segmentQueueAPI.clearFinished(); toast.success(`已清理 ${result.deleted} 条查询记录`) }
    catch (err) { toast.error(apiErrorMessage(err)) }
    finally { setBusy(null); void refresh() }
  }
  const total = Object.values(snapshot?.counts ?? {}).reduce((sum, count) => sum + (count || 0), 0)
  const pages = Math.max(1, Math.ceil((snapshot?.total ?? 0) / 50))

  return (
    <section className="space-y-6" aria-label="片头片尾队列">
      <p className="library-queue-description">
        记录向 TheIntroDB 发起的查询。使用本地缓存时无需新增任务，已保存的时间点在清理查询记录后仍可使用。
      </p>

      <QueueStatusFilters
        filters={FILTERS}
        selected={filter}
        count={(key) => snapshot ? (key === 'all' ? total : snapshot.counts[key] ?? 0) : undefined}
        onSelect={(key) => {
          setFilter(key)
          setPage(1)
        }}
      />

      <div className="library-queue-toolbar">
        <QueueSearch
          label="搜索片头片尾任务"
          placeholder="搜索媒体标题、所属媒体库或失败原因…"
          maxLength={200}
          value={input}
          onChange={setInput}
        />
        <div className="library-queue-toolbar-actions">
          <button type="button" className="library-queue-action" disabled={refreshing} onClick={() => { void refresh(true) }}>
            <RefreshCw size={14} className={refreshing ? 'animate-spin' : undefined} />
            刷新
          </button>
          <button type="button" className="library-queue-action" data-tone="danger" disabled={!!busy || total === 0} onClick={() => { void clear() }}>
            <Trash2 size={14} />
            清理已结束记录
          </button>
        </div>
      </div>

      {error && (
        <div role="alert" className="rounded-xl bg-(--app-danger-soft) p-3 text-sm text-(--app-danger-text)">
          {snapshot ? '刷新失败，当前显示上次结果：' : '队列加载失败：'}{error}
          <button type="button" onClick={() => { void refresh(true) }} className="ml-3 underline">重试加载</button>
        </div>
      )}

      <div className="glass-panel overflow-hidden p-0! shadow-xs">
        <div className="table-scroll">
          <table className="library-queue-table">
            <caption className="sr-only">片头片尾查询任务及所属媒体库</caption>
            <thead className="border-b border-gray-200/80 bg-gray-50/50 text-[11px] font-bold tracking-wider text-sand-500">
              <tr>
                {['媒体文件', '所属媒体库', '触发方式', '查询结果', '状态', '时间'].map((label) => <th key={label} scope="col">{label}</th>)}
                <th scope="col" className="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              {snapshot?.tasks.map((task) => {
                const meta = STATUS[task.status]
                const opened = expanded === task.id
                const spans = task.segments ?? []
                const summarySpans = spans.filter((span) => span.kind === 'intro' || span.kind === 'credits')
                const title = task.media_title || '未命名媒体'
                const canModify = !busy && task.status !== 'running'

                return (
                  <Fragment key={task.id}>
                    <tr className="library-queue-row">
                      <td className="max-w-60">
                        <div className="flex items-center gap-2">
                          <button
                            type="button"
                            aria-label={`查看${task.media_title || '媒体'}的查询详情`}
                            aria-expanded={opened}
                            aria-controls={opened ? `segment-task-${task.id}` : undefined}
                            onClick={() => setExpanded(opened ? null : task.id)}
                            className="shrink-0 rounded p-1 text-(--app-muted) hover:bg-(--app-hover) hover:text-(--app-text)"
                          >
                            {opened ? <ChevronDown size={16} aria-hidden="true" /> : <ChevronRight size={16} aria-hidden="true" />}
                          </button>
                          <div className="min-w-0">
                            <div className="flex min-w-0 items-baseline gap-1">
                              <Link to={`/media/${encodeURIComponent(task.media_id)}`} title={title} className="min-w-0 truncate font-medium text-(--app-text) hover:text-(--app-brand-text) hover:underline">{title}</Link>
                              {task.season_num > 0 && <span className="shrink-0 text-xs text-(--app-muted)">S{task.season_num}E{task.episode_num}</span>}
                            </div>
                            <div className="mt-0.5 truncate font-mono text-[11px] text-(--app-muted)" title={task.media_path}>{task.media_path}</div>
                          </div>
                        </div>
                      </td>
                      <td><span className="library-queue-badge">{task.library_name || '原媒体库已移除'}</span></td>
                      <td className="whitespace-nowrap text-xs text-(--app-subtle)">{TRIGGERS[task.trigger] || '查询'}</td>
                      <td className="whitespace-nowrap text-xs text-(--app-subtle) tabular-nums">
                        {summarySpans.length === 0 ? '—' : summarySpans.map((span, index) => (
                          <div key={index}>{KINDS[span.kind]} {spanText(span)}</div>
                        ))}
                      </td>
                      <td>
                        <span className="library-queue-badge library-queue-status" data-tone={meta.tone}>
                          <meta.icon size={12} className={task.status === 'running' ? 'animate-spin' : undefined} aria-hidden="true" />
                          {meta.label}
                        </span>
                      </td>
                      <td className="whitespace-nowrap text-xs text-(--app-muted) tabular-nums">
                        {formatTime(task.finished_at || task.started_at || task.created_at)}
                      </td>
                      <td>
                        <div className="flex items-center justify-end gap-1">
                          <button type="button" disabled={!canModify} onClick={() => { void action(task, 'retry') }} className="library-queue-action" data-compact data-tone="progress">
                            <RefreshCw size={12} className={busy === task.id ? 'animate-spin' : undefined} />
                            重新查询
                          </button>
                          <button
                            type="button"
                            disabled={!canModify}
                            aria-label={`删除${task.media_title || '媒体'}的查询记录`}
                            title="仅删除查询记录"
                            onClick={() => { void action(task, 'remove') }}
                            className="library-queue-action"
                            data-icon
                            data-tone="danger"
                          >
                            <Trash2 size={13} aria-hidden="true" />
                          </button>
                        </div>
                      </td>
                    </tr>
                    {opened && (
                      <tr id={`segment-task-${task.id}`} className="library-queue-expanded">
                        <td colSpan={7}>
                          <div className="space-y-3 py-2 text-xs text-(--app-subtle)">
                            <div className="flex flex-wrap items-center gap-4">
                              <span>来源：TheIntroDB · {TRIGGERS[task.trigger] || '查询'}</span>
                              <span className="inline-flex items-center gap-1">
                                <Clock size={12} />
                                {task.started_at ? `开始 ${formatTime(task.started_at)}` : '历史记录未保存开始时间'}
                              </span>
                              {task.finished_at && <span>结束 {formatTime(task.finished_at)}</span>}
                            </div>
                            {task.error && <p role="alert" className="rounded-lg bg-(--app-danger-soft) p-3 text-(--app-danger-text)">{task.error}</p>}
                            {spans.length > 0 && (
                              <div className="flex flex-wrap gap-2">
                                {spans.map((span, index) => <span key={index} className="library-queue-badge tabular-nums">{KINDS[span.kind] || span.kind}：{spanText(span)}</span>)}
                              </div>
                            )}
                            <p className="inline-flex items-center gap-1.5 text-(--app-muted)">
                              <Database size={13} />
                              {task.status === 'found' ? '时间点已保存到数据库' : task.status === 'missing' ? 'TheIntroDB 暂无时间点，查询结果已记录' : task.status === 'running' ? '正在向 TheIntroDB 查询，不读取视频文件' : '本次查询失败，已有时间点保留'}
                            </p>
                          </div>
                        </td>
                      </tr>
                    )}
                  </Fragment>
                )
              })}
            </tbody>
          </table>
        </div>

        {!snapshot && !error && <QueueTableMessage loading>正在读取查询记录…</QueueTableMessage>}
        {snapshot?.tasks.length === 0 && (
          <QueueTableMessage>
            {filter !== 'all' || query ? '没有符合条件的查询记录' : '暂无查询记录，可在媒体详情页查询片头片尾，或在媒体库刮削设置中启用后台预取。'}
          </QueueTableMessage>
        )}
        {snapshot && <QueuePagination total={snapshot.total} page={page} pages={pages} onPageChange={setPage} />}
      </div>
    </section>
  )
}
