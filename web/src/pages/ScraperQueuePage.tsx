import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import toast from 'react-hot-toast'
import {
  AlertCircle,
  Ban,
  CheckCircle2,
  Clock,
  Eye,
  Film,
  Image as ImageIcon,
  Layers,
  Loader2,
  PlayCircle,
  RefreshCw,
  Sparkles,
  Trash2,
  Tv,
  X,
} from 'lucide-react'

import { ARTWORK, imageURL } from '../api/client'
import { scraperAPI } from '../api/scraper'
import type { ScrapeQueueSnapshot, ScrapeTask, ScrapeTaskStatus } from '../types/scraper'
import { QueuePagination, QueueSearch, QueueStatusFilters, QueueTableMessage, type QueueFilter } from './LibraryQueueControls'
import { apiErrorMessage, formatTime, taskStatusMeta } from './StrmManagePage'
import { ScrapeDetailModal } from './scraper-queue/ScrapeDetailModal'
import { PROVIDER_LABELS } from './scraper-queue/scrapeLabels'
import { copyToClipboard, useTaskSelection } from './queue-shared'

const FILTERS: QueueFilter<'all' | ScrapeTaskStatus>[] = [
  { key: 'all', label: '全部', icon: Sparkles, tone: 'neutral' },
  { key: 'pending', label: '排队中', icon: Clock, tone: 'muted' },
  { key: 'running', label: '刮削中', icon: PlayCircle, tone: 'progress' },
  { key: 'done', label: '已匹配', icon: CheckCircle2, tone: 'success' },
  { key: 'failed', label: '未匹配/失败', icon: AlertCircle, tone: 'danger' },
  { key: 'canceled', label: '已取消', icon: Ban, tone: 'warning' },
]

const TYPE_ICONS: Record<string, ReactNode> = {
  movie: <Film size={14} className="text-blue-500" />,
  tv: <Tv size={14} className="text-purple-500" />,
  anime: <Layers size={14} className="text-emerald-500" />,
}

const PAGE_SIZE = 50

export function ScraperQueuePanel() {
  const [snapshot, setSnapshot] = useState<ScrapeQueueSnapshot | null>(null)
  const [filter, setFilter] = useState<'all' | ScrapeTaskStatus>('all')
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [totalPages, setTotalPages] = useState(1)
  const [loading, setLoading] = useState(true)
  const [batchBusy, setBatchBusy] = useState(false)
  const { selectedIds, setSelectedIds, reset: clearSelection, toggleRow: toggleSelectRow, toggleAll: toggleAllIds } = useTaskSelection()
  const [detailTask, setDetailTask] = useState<ScrapeTask | null>(null)
  // 轮询/翻页/切筛选并发时用递增序号丢弃过期响应
  const refreshSeqRef = useRef(0)
  const [pollFailures, setPollFailures] = useState(0)

  const refresh = useCallback(
    async () => {
      const seq = ++refreshSeqRef.current
      try {
        const status = filter === 'all' ? undefined : filter
        const data = await scraperAPI.queue(status, page, PAGE_SIZE)
        // 序号不符说明已有更新的请求发出（翻页/切筛选/轮询并发），丢弃旧响应
        if (seq !== refreshSeqRef.current) return
        setPollFailures(0)
        const tp = Math.max(1, Math.ceil((data.total ?? data.tasks.length) / PAGE_SIZE))
        if (page > tp) {
          setPage(tp)
          return
        }
        setTotalPages(tp)
        setSnapshot(data)
      } catch {
        // 保留旧数据；连续失败 ≥3 次时页头徽标切换为「连接失败，重试中」
        if (seq === refreshSeqRef.current) setPollFailures((n) => n + 1)
      } finally {
        setLoading(false)
      }
    },
    [filter, page],
  )

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [refresh])

  useEffect(() => {
    const timer = setInterval(() => {
      if (document.hidden) return
      refresh().catch(() => undefined)
    }, 3000)
    return () => clearInterval(timer)
  }, [refresh])

  useEffect(() => {
    clearSelection()
  }, [filter, page, clearSelection])

  const copyText = copyToClipboard

  // Row actions
  const cancelTask = async (task: ScrapeTask) => {
    try {
      await scraperAPI.cancelTask(task.id)
      toast.success('已取消刮削任务')
      await refresh()
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }

  const retryTask = async (task: ScrapeTask) => {
    try {
      await scraperAPI.retryTask(task.id)
      toast.success('已重新推入刮削队列')
      await refresh()
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }

  const deleteTask = async (task: ScrapeTask) => {
    try {
      await scraperAPI.deleteTask(task.id)
      toast.success('已删除刮削记录')
      if (detailTask?.id === task.id) setDetailTask(null)
      await refresh()
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }

  // Batch actions
  const runSelectedBatch = async (action: 'retry' | 'cancel' | 'delete') => {
    const ids = Array.from(selectedIds)
    if (ids.length === 0) return

    const actionText = action === 'retry' ? '重新刮削' : action === 'cancel' ? '取消' : '删除'
    if (action === 'delete' && !window.confirm(`确定删除选中的 ${ids.length} 条刮削记录？`)) return
    if (action === 'cancel' && !window.confirm(`确定取消选中的 ${ids.length} 个刮削任务？`)) return

    setBatchBusy(true)
    try {
      const res = await scraperAPI.batchAction(action, ids)
      toast.success(`已成功${actionText} ${res.affected} 项`)
      setSelectedIds(new Set())
      await refresh()
    } catch (err) {
      toast.error(apiErrorMessage(err))
    } finally {
      setBatchBusy(false)
    }
  }

  const runGlobalBatch = async (
    action: () => Promise<{ deleted?: number; retried?: number; canceled?: number }>,
    confirmMsg?: string,
  ) => {
    if (confirmMsg && !window.confirm(confirmMsg)) return
    setBatchBusy(true)
    try {
      const res = await action()
      if (res.deleted !== undefined) toast.success(`已清空 ${res.deleted} 条记录`)
      else if (res.retried !== undefined) toast.success(`已重新入队 ${res.retried} 个任务`)
      else if (res.canceled !== undefined) toast.success(`已取消 ${res.canceled} 个任务`)
      await refresh()
    } catch (err) {
      toast.error(apiErrorMessage(err))
    } finally {
      setBatchBusy(false)
    }
  }

  // Filter and search
  const filteredTasks = useMemo(() => {
    let list = snapshot?.tasks ?? []
    if (filter !== 'all') {
      list = list.filter((t) => t.status === filter)
    }
    if (search.trim()) {
      const q = search.trim().toLowerCase()
      list = list.filter(
        (t) =>
          t.media_title.toLowerCase().includes(q) ||
          t.matched_title.toLowerCase().includes(q) ||
          t.library_name.toLowerCase().includes(q) ||
          t.media_path.toLowerCase().includes(q) ||
          (t.error && t.error.toLowerCase().includes(q)),
      )
    }
    return list
  }, [snapshot?.tasks, filter, search])

  const counts = snapshot?.counts
  const pendingCount = counts?.pending ?? 0
  const runningCount = counts?.running ?? 0
  const activeTaskCount = pendingCount + runningCount
  const doneCount = counts?.done ?? 0
  const failedCount = counts?.failed ?? 0
  const canceledCount = counts?.canceled ?? 0
  const finishedCount = doneCount + failedCount + canceledCount
  const allCurrentChecked =
    filteredTasks.length > 0 && filteredTasks.every((t) => selectedIds.has(t.id))

  const toggleSelectAll = () => toggleAllIds(filteredTasks.map((t) => t.id))

  return (
    <section className="space-y-6" aria-label="刮削队列">
      <div className="library-queue-description">
        <p>媒体元数据识别与海报、剧照下载进度（TMDb / Bangumi / TheTVDB）</p>
        <div role="status">
          {pollFailures >= 3 ? (
            <span className="library-queue-badge" data-tone="danger">
              <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-current" />
              连接失败，重试中
            </span>
          ) : (
            <span className="library-queue-badge" data-tone="success">
              <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-current" />
              实时同步
            </span>
          )}
        </div>
      </div>

      <QueueStatusFilters
        filters={FILTERS}
        selected={filter}
        count={(key) => counts && (key === 'all' ? activeTaskCount + finishedCount : counts[key] ?? 0)}
        onSelect={(key) => {
          setFilter(key)
          setPage(1)
        }}
      />

      <div className="library-queue-toolbar">
        <QueueSearch
          label="搜索刮削任务"
          placeholder="搜索媒体标题、匹配结果、媒体库或错误信息…"
          value={search}
          onChange={setSearch}
        />

        {selectedIds.size > 0 ? (
          <div className="library-queue-toolbar-actions rounded-xl bg-(--app-brand-soft) px-2 py-1.5 text-xs">
            <span className="font-semibold text-(--app-brand-text)">已选中 {selectedIds.size} 项</span>
            <button
              type="button"
              disabled={batchBusy}
              onClick={() => runSelectedBatch('retry')}
              className="library-queue-action" data-tone="progress"
            >
              <RefreshCw size={12} />
              重试选中
            </button>
            <button
              type="button"
              disabled={batchBusy}
              onClick={() => runSelectedBatch('cancel')}
              className="library-queue-action" data-tone="warning"
            >
              <Ban size={12} />
              取消选中
            </button>
            <button
              type="button"
              disabled={batchBusy}
              onClick={() => runSelectedBatch('delete')}
              className="library-queue-action" data-tone="danger"
            >
              <Trash2 size={12} />
              删除选中
            </button>
            <button
              type="button"
              onClick={() => setSelectedIds(new Set())}
              className="p-1 text-(--app-muted) hover:text-(--app-text)"
              title="清空选择"
            >
              <X size={13} />
            </button>
          </div>
        ) : (
          <div className="library-queue-toolbar-actions">
            {/* 当前状态专属快捷批量按钮 */}
            {filter === 'all' && (
              <>
                {failedCount > 0 && (
                  <button
                    type="button"
                    disabled={batchBusy}
                    onClick={() => runGlobalBatch(() => scraperAPI.retryFailed(), '确定重新入队所有失败任务？')}
                    className="library-queue-action" data-tone="progress"
                  >
                    <RefreshCw size={12} />
                    全部重试 ({failedCount})
                  </button>
                )}
                {activeTaskCount > 0 && (
                  <button
                    type="button"
                    disabled={batchBusy}
                    onClick={() => runGlobalBatch(() => scraperAPI.cancelPending(), '确定取消所有排队及进行中的刮削任务？')}
                    className="library-queue-action" data-tone="warning"
                  >
                    <Ban size={12} />
                    全部取消 ({activeTaskCount})
                  </button>
                )}
                {finishedCount > 0 && (
                  <button
                    type="button"
                    disabled={batchBusy}
                    onClick={() => runGlobalBatch(() => scraperAPI.clearFinished(), '确定清空所有已完成、失败及取消的历史记录？')}
                    className="library-queue-action" data-tone="danger"
                  >
                    <Trash2 size={12} />
                    全部删除 ({finishedCount})
                  </button>
                )}
              </>
            )}

            {(filter === 'pending' || filter === 'running') && (
              <button
                type="button"
                disabled={batchBusy || activeTaskCount === 0}
                onClick={() => runGlobalBatch(() => scraperAPI.cancelPending(), '确定取消所有排队及进行中的刮削任务？')}
                className="library-queue-action" data-tone="warning"
              >
                <Ban size={12} />
                全部取消{activeTaskCount > 0 ? ` (${activeTaskCount})` : ''}
              </button>
            )}

            {filter === 'done' && (
              <button
                type="button"
                disabled={batchBusy || doneCount === 0}
                onClick={() => runGlobalBatch(() => scraperAPI.clearDone(), '确定清空所有已匹配完成的记录？')}
                className="library-queue-action" data-tone="danger"
              >
                <Trash2 size={12} />
                全部删除{doneCount > 0 ? ` (${doneCount})` : ''}
              </button>
            )}

            {filter === 'failed' && (
              <>
                <button
                  type="button"
                  disabled={batchBusy || failedCount === 0}
                  onClick={() => runGlobalBatch(() => scraperAPI.retryFailed(), '确定重新入队所有失败任务？')}
                  className="library-queue-action" data-tone="progress"
                >
                  <RefreshCw size={12} />
                  全部重试{failedCount > 0 ? ` (${failedCount})` : ''}
                </button>
                <button
                  type="button"
                  disabled={batchBusy || failedCount === 0}
                  onClick={() => runGlobalBatch(() => scraperAPI.clearFailed(), '确定清空所有失败记录？')}
                  className="library-queue-action" data-tone="danger"
                >
                  <Trash2 size={12} />
                  全部删除{failedCount > 0 ? ` (${failedCount})` : ''}
                </button>
              </>
            )}

            {filter === 'canceled' && (
              <button
                type="button"
                disabled={batchBusy || canceledCount === 0}
                onClick={() => runGlobalBatch(() => scraperAPI.clearCanceled(), '确定清空所有已取消的任务记录？')}
                className="library-queue-action" data-tone="danger"
              >
                <Trash2 size={12} />
                全部删除{canceledCount > 0 ? ` (${canceledCount})` : ''}
              </button>
            )}

            {/* 下拉批量操作菜单：随时可做任意全局操作 */}
            <details className="relative inline-block">
              <summary className="library-queue-action cursor-pointer list-none [&::-webkit-details-marker]:hidden">
                <Trash2 size={12} />
                <span>批量清理</span>
              </summary>
              <div className="absolute right-0 top-full z-30 mt-1 min-w-44 rounded-xl border border-(--app-border) bg-(--app-panel-elevated) p-1.5 shadow-xl">
                {failedCount > 0 && (
                  <button
                    type="button"
                    disabled={batchBusy}
                    onClick={(e) => {
                      e.currentTarget.closest('details')?.removeAttribute('open')
                      runGlobalBatch(() => scraperAPI.retryFailed(), '确定重新入队所有失败任务？')
                    }}
                    className="library-queue-menu-item" data-tone="progress"
                  >
                    <RefreshCw size={13} />
                    <span>重试所有失败 ({failedCount})</span>
                  </button>
                )}
                {activeTaskCount > 0 && (
                  <button
                    type="button"
                    disabled={batchBusy}
                    onClick={(e) => {
                      e.currentTarget.closest('details')?.removeAttribute('open')
                      runGlobalBatch(() => scraperAPI.cancelPending(), '确定取消所有排队及进行中的刮削任务？')
                    }}
                    className="library-queue-menu-item" data-tone="warning"
                  >
                    <Ban size={13} />
                    <span>取消所有进行中 ({activeTaskCount})</span>
                  </button>
                )}
                <div className="my-1 border-t border-(--app-border)" />
                <button
                  type="button"
                  disabled={batchBusy}
                  onClick={(e) => {
                    e.currentTarget.closest('details')?.removeAttribute('open')
                    runGlobalBatch(() => scraperAPI.clearDone(), '确定清空所有已匹配完成的记录？')
                  }}
                  className="library-queue-menu-item"
                >
                  <CheckCircle2 size={13} className="text-emerald-500" />
                  <span>清空已完成记录 ({doneCount})</span>
                </button>
                <button
                  type="button"
                  disabled={batchBusy}
                  onClick={(e) => {
                    e.currentTarget.closest('details')?.removeAttribute('open')
                    runGlobalBatch(() => scraperAPI.clearFailed(), '确定清空所有失败的记录？')
                  }}
                  className="library-queue-menu-item" data-tone="danger"
                >
                  <AlertCircle size={13} />
                  <span>清空失败记录 ({failedCount})</span>
                </button>
                <button
                  type="button"
                  disabled={batchBusy}
                  onClick={(e) => {
                    e.currentTarget.closest('details')?.removeAttribute('open')
                    runGlobalBatch(() => scraperAPI.clearCanceled(), '确定清空所有已取消的任务记录？')
                  }}
                  className="library-queue-menu-item"
                >
                  <Ban size={13} className="text-amber-500" />
                  <span>清空已取消记录 ({canceledCount})</span>
                </button>
                <button
                  type="button"
                  disabled={batchBusy}
                  onClick={(e) => {
                    e.currentTarget.closest('details')?.removeAttribute('open')
                    runGlobalBatch(() => scraperAPI.clearFinished(), '确定清空所有已完成、失败及取消的历史记录？')
                  }}
                  className="library-queue-menu-item" data-tone="danger"
                >
                  <Trash2 size={13} />
                  <span>清空全部历史记录 ({finishedCount})</span>
                </button>
              </div>
            </details>
          </div>
        )}
      </div>

      <div className="glass-panel overflow-hidden p-0! shadow-xs">
        <div className="table-scroll">
          <table className="library-queue-table">
            <caption className="sr-only">刮削任务及匹配结果</caption>
            <thead className="border-b border-gray-200/80 bg-gray-50/50 text-[11px] font-bold tracking-wider text-sand-500">
              <tr>
                <th className="w-10 text-center" scope="col">
                  <input
                    type="checkbox"
                    checked={allCurrentChecked}
                    disabled={filteredTasks.length === 0}
                    onChange={toggleSelectAll}
                    className="h-3.5 w-3.5 cursor-pointer accent-(--app-brand-text)"
                    title="全选 / 反选本页"
                  />
                </th>
                <th scope="col">媒体文件</th>
                <th scope="col">所属媒体库</th>
                <th scope="col">刮削匹配结果</th>
                <th scope="col">识别源</th>
                <th scope="col">状态</th>
                <th scope="col">时间</th>
                <th scope="col" className="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              {!loading && filteredTasks.map((task) => {
                const status = taskStatusMeta(task.status)
                const filterStyle = FILTERS.find((item) => item.key === task.status)
                const StatusIcon = task.status === 'running' ? Loader2 : filterStyle?.icon
                const isSelected = selectedIds.has(task.id)

                return (
                  <tr key={task.id} className="library-queue-row" data-selected={isSelected}>
                    <td className="text-center">
                      <input
                        type="checkbox"
                        checked={isSelected}
                        onChange={() => toggleSelectRow(task.id)}
                        className="h-3.5 w-3.5 cursor-pointer accent-(--app-brand-text)"
                      />
                    </td>

                    {/* Media title & path */}
                    <td className="max-w-60">
                      <div className="flex items-center gap-2">
                        <span className="inline-flex h-3.5 w-3.5 shrink-0 items-center justify-center" aria-hidden="true">
                          {TYPE_ICONS[task.media_type] || <Film size={14} className="text-(--app-muted)" />}
                        </span>
                        <div className="min-w-0 flex-1">
                          <button
                            type="button"
                            onClick={() => setDetailTask(task)}
                            className="block max-w-full truncate text-left font-medium text-(--app-text) hover:text-(--app-brand-text) hover:underline"
                            title={task.media_title}
                          >
                            {task.media_title}
                          </button>
                          <span className="mt-0.5 block truncate font-mono text-[11px] text-(--app-muted)" title={task.media_path}>
                            {task.media_path}
                          </span>
                        </div>
                      </div>
                    </td>

                    {/* Library */}
                    <td className="whitespace-nowrap">
                      <span className="library-queue-badge">
                        {task.library_name || '媒体库'}
                      </span>
                    </td>

                    {/* Scraped matched result */}
                    <td className="max-w-60">
                      {task.matched_title ? (
                        <div className="flex items-center gap-2">
                          {task.poster_url ? (
                            <img
                              src={imageURL(task.poster_url, undefined, ARTWORK.posterTiny)}
                              alt=""
                              className="h-10 w-7 shrink-0 rounded-sm border border-(--app-border) object-cover"
                              onError={(e) => {
                                e.currentTarget.style.display = 'none'
                              }}
                            />
                          ) : (
                            <div className="flex h-10 w-7 shrink-0 items-center justify-center rounded-sm bg-(--app-panel-soft) text-(--app-muted)">
                              <ImageIcon size={12} />
                            </div>
                          )}
                          <div className="min-w-0">
                            <span className="block truncate text-xs font-medium text-(--app-text)">
                              {task.matched_title}
                            </span>
                            {task.matched_year > 0 && (
                              <span className="text-[11px] text-(--app-muted)">
                                {task.matched_year} 年
                              </span>
                            )}
                          </div>
                        </div>
                      ) : (
                        <span className="text-xs text-(--app-muted)">
                          {task.status === 'pending' || task.status === 'running'
                            ? '等待识别…'
                            : '未匹配到结果'}
                        </span>
                      )}
                    </td>

                    {/* Provider */}
                    <td className="whitespace-nowrap">
                      {task.provider ? (
                        <span className="library-queue-badge" data-tone="progress">
                          {PROVIDER_LABELS[task.provider] ?? task.provider}
                        </span>
                      ) : (
                        <span className="text-xs text-(--app-muted)">—</span>
                      )}
                    </td>

                    {/* Status & Error */}
                    <td>
                      <div className="flex flex-col gap-0.5">
                          <span className="library-queue-badge library-queue-status" data-tone={filterStyle?.tone}>
                          {StatusIcon && (
                            <StatusIcon size={12} className={task.status === 'running' ? 'animate-spin' : undefined} aria-hidden="true" />
                          )}
                          {task.status === 'done'
                            ? '已匹配'
                            : task.status === 'failed'
                              ? '未匹配'
                              : status.label}
                        </span>
                        {task.error && (
                          <span
                            onClick={() => setDetailTask(task)}
                            className="max-w-[180px] cursor-pointer truncate text-[11px] text-(--app-danger-text) hover:underline"
                            title={task.error}
                          >
                            {task.error}
                          </span>
                        )}
                      </div>
                    </td>

                    {/* Time */}
                    <td className="whitespace-nowrap text-xs text-(--app-muted) tabular-nums">
                      {formatTime(task.created_at)}
                    </td>

                    {/* Actions */}
                    <td className="whitespace-nowrap text-right">
                      <div className="flex items-center justify-end gap-1">
                        <button
                          type="button"
                          onClick={() => setDetailTask(task)}
                          className="library-queue-action" data-icon
                          title="查看详情"
                        >
                          <Eye size={13} />
                        </button>

                        {(task.status === 'pending' || task.status === 'running') && (
                          <button
                            type="button"
                            onClick={() => cancelTask(task)}
                            className="library-queue-action" data-compact data-tone="warning"
                            title="取消刮削"
                          >
                            取消
                          </button>
                        )}

                        {(task.status === 'failed' ||
                          task.status === 'canceled' ||
                          task.status === 'done') && (
                          <button
                            type="button"
                            onClick={() => retryTask(task)}
                            className="library-queue-action" data-compact data-tone="progress"
                            title="重新刮削"
                          >
                            重刮
                          </button>
                        )}

                        {(task.status === 'done' ||
                          task.status === 'failed' ||
                          task.status === 'canceled') && (
                          <button
                            type="button"
                            onClick={() => deleteTask(task)}
                            className="library-queue-action" data-icon data-tone="danger"
                            title="删除记录"
                          >
                            <Trash2 size={13} />
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
        {loading && <QueueTableMessage loading>正在读取刮削任务…</QueueTableMessage>}
        {!loading && filteredTasks.length === 0 && (
          <QueueTableMessage>
            {search
              ? '没有找到符合搜索条件的刮削任务'
              : filter === 'all'
                ? '刮削队列为空，暂无进行或排队中的任务'
                : `「${FILTERS.find((f) => f.key === filter)?.label}」状态下暂无任务`}
          </QueueTableMessage>
        )}
        {snapshot && (
          <QueuePagination total={snapshot.total ?? snapshot.tasks.length} page={page} pages={totalPages} disabled={loading} onPageChange={setPage} />
        )}
      </div>

      {/* 6. Task Detail Modal */}
      {detailTask && (
        <ScrapeDetailModal
          task={detailTask}
          onClose={() => setDetailTask(null)}
          onRetry={retryTask}
          onCancel={cancelTask}
          onDelete={deleteTask}
          onCopy={copyText}
        />
      )}
    </section>
  )
}
