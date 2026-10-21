import { Fragment, useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { AlertCircle, CheckCircle2, ChevronDown, ChevronRight, Clock, Database, ListVideo, Loader2, MinusCircle, RefreshCw, Search, Trash2 } from 'lucide-react'
import clsx from 'clsx'
import toast from 'react-hot-toast'

import { segmentQueueAPI, type SegmentTask, type SegmentTaskCounts, type SegmentTaskStatus, type SegmentQueueSnapshot } from '../api/segments'
import { apiErrorMessage } from './StrmManagePage'

const STATUS = {
  running: { label: '查询中', icon: Loader2, color: 'text-brand-600 bg-brand-500/10' },
  found: { label: '查到时间点', icon: CheckCircle2, color: 'text-emerald-600 bg-emerald-500/10' },
  missing: { label: '暂无数据', icon: MinusCircle, color: 'text-ink-100 bg-sand-200/60' },
  failed: { label: '查询失败', icon: AlertCircle, color: 'text-rose-600 bg-rose-500/10' },
} as const
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
function dateTime(raw?: string) {
  return raw ? new Date(raw).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }) : '—'
}

export function SegmentQueuePage({ onCountsChange }: { onCountsChange?: (counts: SegmentTaskCounts) => void }) {
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
  useEffect(() => { if (snapshot) onCountsChange?.(snapshot.counts) }, [snapshot, onCountsChange])

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
  const filters = [{ key: 'all' as const, label: '全部', icon: ListVideo, color: 'text-ink-600' }, ...Object.entries(STATUS).map(([key, value]) => ({ ...value, key: key as SegmentTaskStatus }))]

  return <section className="space-y-4" aria-label="片头片尾队列">
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-5">
      {filters.map(item => <button key={item.key} type="button" aria-pressed={filter === item.key} onClick={() => { setFilter(item.key); setPage(1) }} className={clsx('glass-panel rounded-2xl border p-4 text-left transition', filter === item.key ? 'border-brand-500 ring-1 ring-brand-500/20' : 'border-sand-200 hover:border-brand-300')}>
        <div className="flex items-center justify-between gap-2 text-sm text-ink-50"><span>{item.label}</span><item.icon size={16} /></div>
        <div className="mt-3 text-2xl font-bold tabular-nums text-ink-600">{snapshot ? (item.key === 'all' ? total : snapshot.counts[item.key] ?? 0) : '—'}<span className="ml-1 text-xs font-normal text-ink-50">项</span></div>
      </button>)}
    </div>
    <div className="flex flex-wrap items-center gap-3">
      <label className="relative min-w-0 flex-1"><Search size={16} className="absolute left-3 top-3 text-ink-50" /><span className="sr-only">搜索片头片尾任务</span><input value={input} onChange={event => setInput(event.target.value)} maxLength={200} placeholder="搜索媒体标题、所属媒体库或失败原因…" className="input-field w-full pl-9 text-sm" /></label>
      <button type="button" className="btn-outline px-3! py-2! text-xs" disabled={refreshing} onClick={() => { void refresh(true) }}><RefreshCw size={14} className={refreshing ? 'animate-spin' : ''} />刷新</button>
      <button type="button" className="btn-outline px-3! py-2! text-xs" disabled={!!busy || total === 0} onClick={() => { void clear() }}><Trash2 size={14} />清理已结束记录</button>
    </div>
    <p className="text-xs leading-relaxed text-ink-50">记录向 TheIntroDB 发起的查询；使用本地缓存不会新增任务。暂无数据与查询失败分开显示，清理记录不会删除已保存的时间点。</p>
    {error && <div role="alert" className="rounded-xl border border-rose-500/20 bg-rose-500/5 p-3 text-sm text-rose-600">{snapshot ? '刷新失败，当前显示上次结果：' : '队列加载失败：'}{error}<button type="button" onClick={() => { void refresh(true) }} className="ml-3 underline">重试加载</button></div>}
    <div className="glass-panel overflow-hidden rounded-2xl border border-sand-200">
      <div className="overflow-x-auto"><table className="w-full min-w-[850px] text-left text-sm"><caption className="sr-only">片头片尾查询任务及所属媒体库</caption><thead className="border-b border-sand-200 bg-sand-50/50 text-xs text-ink-50"><tr>{['媒体文件', '所属媒体库', '触发方式', '查询结果', '状态', '时间', '操作'].map(label => <th key={label} className="px-4 py-3 font-medium">{label}</th>)}</tr></thead><tbody>
        {!snapshot && !error && <tr><td colSpan={7} className="p-10 text-center text-ink-50"><Loader2 size={20} className="mx-auto mb-2 animate-spin" />正在读取查询记录…</td></tr>}
        {snapshot?.tasks.length === 0 && <tr><td colSpan={7} className="p-12 text-center text-ink-50"><ListVideo size={28} className="mx-auto mb-3 opacity-60" />{filter !== 'all' || query ? '没有符合条件的查询记录' : '暂无查询记录，可在媒体详情页查询片头片尾，或在媒体库刮削设置中启用后台预取。'}</td></tr>}
        {snapshot?.tasks.map(task => {
          const meta = STATUS[task.status]
          const opened = expanded === task.id
          const spans = task.segments ?? []
          return <Fragment key={task.id}><tr className="border-b border-sand-200/60 align-top hover:bg-sand-50/40">
            <td className="max-w-[280px] px-4 py-4"><div className="flex items-start gap-2"><button type="button" aria-label={`查看${task.media_title || '媒体'}的查询详情`} aria-expanded={opened} aria-controls={`segment-task-${task.id}`} onClick={() => setExpanded(opened ? null : task.id)} className="mt-0.5 shrink-0 rounded-sm p-0.5 hover:bg-brand-50">{opened ? <ChevronDown size={16} /> : <ChevronRight size={16} />}</button><div className="min-w-0"><Link to={`/media/${encodeURIComponent(task.media_id)}`} className="font-medium text-ink-600 hover:text-brand-600">{task.media_title || '未命名媒体'}</Link>{task.season_num > 0 && <span className="ml-1 text-xs text-ink-50">S{task.season_num}E{task.episode_num}</span>}<div className="mt-1 truncate font-mono text-[11px] text-ink-50" title={task.media_path}>{task.media_path}</div></div></div></td>
            <td className="px-4 py-4"><span className="inline-block rounded-lg border border-sand-200 px-2 py-1 text-xs text-ink-100">{task.library_name || '原媒体库已移除'}</span></td>
            <td className="whitespace-nowrap px-4 py-4 text-xs text-ink-100">{TRIGGERS[task.trigger] || '查询'}</td>
            <td className="px-4 py-4 text-xs text-ink-100">{spans.length > 0 ? spans.filter(span => span.kind === 'intro' || span.kind === 'credits').map((span, index) => <div key={index} className="mb-1 whitespace-nowrap tabular-nums">{KINDS[span.kind]} {spanText(span)}</div>) : <span className="text-ink-50">—</span>}</td>
            <td className="px-4 py-4"><span className={clsx('inline-flex items-center gap-1 whitespace-nowrap rounded-full px-2 py-1 text-xs', meta.color)}><meta.icon size={12} className={task.status === 'running' ? 'animate-spin' : ''} />{meta.label}</span></td>
            <td className="whitespace-nowrap px-4 py-4 text-xs tabular-nums text-ink-50">{dateTime(task.finished_at || task.started_at || task.created_at)}</td>
            <td className="px-4 py-4"><div className="flex gap-2"><button type="button" disabled={!!busy || task.status === 'running'} onClick={() => { void action(task, 'retry') }} className="btn-outline px-2! py-1! text-xs whitespace-nowrap"><RefreshCw size={12} className={busy === task.id ? 'animate-spin' : ''} />重新查询</button><button type="button" disabled={!!busy || task.status === 'running'} aria-label={`删除${task.media_title || '媒体'}的查询记录`} title="仅删除查询记录" onClick={() => { void action(task, 'remove') }} className="rounded-lg border border-sand-200 px-2 text-ink-50 hover:text-rose-600 disabled:opacity-40"><Trash2 size={13} /></button></div></td>
          </tr>{opened && <tr id={`segment-task-${task.id}`} className="border-b border-sand-200 bg-sand-50/40"><td colSpan={7} className="px-6 py-4"><div className="space-y-3 text-xs text-ink-100">
            <div className="flex flex-wrap items-center gap-4"><span>来源：TheIntroDB · {TRIGGERS[task.trigger] || '查询'}</span><span className="inline-flex items-center gap-1"><Clock size={12} />{task.started_at ? `开始 ${dateTime(task.started_at)}` : '历史记录未保存开始时间'}</span>{task.finished_at && <span>结束 {dateTime(task.finished_at)}</span>}</div>
            {task.error && <p role="alert" className="rounded-lg bg-rose-500/10 p-3 text-rose-600">{task.error}</p>}
            {spans.length > 0 && <div className="flex flex-wrap gap-3">{spans.map((span, index) => <span key={index} className="rounded-lg border border-sand-200 bg-surface px-3 py-2 tabular-nums">{KINDS[span.kind] || span.kind}：{spanText(span)}</span>)}</div>}
            <p className="inline-flex items-center gap-1.5 text-ink-50"><Database size={13} />{task.status === 'found' ? '时间点已保存到数据库' : task.status === 'missing' ? 'TheIntroDB 暂无时间点，查询结果已记录' : task.status === 'running' ? '正在向 TheIntroDB 查询，不读取视频文件' : '本次查询失败，已有时间点保留'}</p>
          </div></td></tr>}</Fragment>
        })}
      </tbody></table></div>
      <div className="flex items-center justify-between gap-3 border-t border-sand-200 px-4 py-3 text-xs text-ink-50"><span>共 {snapshot?.total ?? 0} 条 · 第 {page} / {pages} 页</span><div className="flex gap-2"><button type="button" className="btn-outline px-3! py-1!" disabled={page <= 1} onClick={() => setPage(value => value - 1)}>上一页</button><button type="button" className="btn-outline px-3! py-1!" disabled={page >= pages} onClick={() => setPage(value => value + 1)}>下一页</button></div></div>
    </div>
  </section>
}
