import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowRight, Pencil } from 'lucide-react'

import { historyAPI } from '../api/history'
import { UserAvatar } from '../components/UserAvatar'
import { usePermission } from '../hooks/usePermission'
import type { User } from '../types'
import type { WatchSummary } from '../types/history'

const WATCH_LABELS = [['movies', '电影'], ['series', '剧集'], ['episodes', '单集']] as const
const formatCount = new Intl.NumberFormat('zh-CN')
const formatDate = new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: 'long' })

export function ProfileOverview({ user, onEdit }: { user: User | null; onEdit: () => void }) {
  const canViewHistory = usePermission('can_view_history')
  const [summary, setSummary] = useState<{ userID: string; data: WatchSummary }>()
  const userID = user?.id
  const displayName = user?.nickname?.trim() || user?.username || '个人账户'
  const joined = user?.created_at ? new Date(user.created_at) : undefined

  useEffect(() => {
    if (!canViewHistory || !userID) return
    const controller = new AbortController()
    historyAPI.summary(controller.signal).then((data) => {
      if (!controller.signal.aborted) setSummary({ userID, data })
    }).catch(() => undefined)
    return () => controller.abort()
  }, [canViewHistory, userID])

  return (
    <section className={`grid min-w-0 gap-5 rounded-2xl bg-(--app-panel) p-4 sm:p-6 ${canViewHistory ? 'lg:grid-cols-[minmax(0,1fr)_minmax(250px,0.5fr)] lg:items-center' : ''}`} aria-label="账号概览">
      <div className="flex min-w-0 items-center gap-4 sm:gap-6">
        <UserAvatar user={user} square className="size-20 text-2xl sm:size-28 sm:text-4xl" />
        <div className="min-w-0 space-y-2.5">
          <h2 className="wrap-break-word font-display text-2xl leading-tight font-semibold sm:text-3xl">{displayName}</h2>
          {user && <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-(--app-muted) sm:text-sm">
            {displayName !== user.username && <span className="wrap-break-word">@{user.username}</span>}
            <span>{user.role === 'admin' ? '管理员' : '普通用户'}</span>
            {joined && !Number.isNaN(joined.getTime()) && <time dateTime={user.created_at}>{formatDate.format(joined)}加入</time>}
          </div>}
          {user && <button type="button" className="btn-outline min-h-10" aria-haspopup="dialog" onClick={onEdit}>
            <Pencil size={15} aria-hidden="true" />编辑资料
          </button>}
        </div>
      </div>

      {canViewHistory && <div className="min-w-0 rounded-xl border border-(--app-border) bg-(--app-panel-elevated) px-4 py-3.5">
        <div className="mb-3 flex items-center justify-between gap-2">
          <h3 className="text-sm font-semibold">观看统计</h3>
          <Link to="/history/stats" aria-label="查看观看统计详情" className="inline-flex items-center gap-1 text-xs text-(--app-muted) hover:text-(--app-text)">
            详情 <ArrowRight size={13} aria-hidden="true" />
          </Link>
        </div>
        <dl className="grid grid-cols-3 divide-x divide-(--app-border)">
          {WATCH_LABELS.map(([key, label]) => {
            const count = summary?.userID === userID ? summary?.data[key] : undefined
            return <div key={key} className="flex min-w-0 flex-col px-1.5 text-center">
              <dt className="order-1 mt-1 text-xs text-(--app-muted)">{label}</dt>
              <dd className="font-display text-xl leading-tight font-semibold tabular-nums sm:text-2xl">{count == null ? '—' : formatCount.format(count)}</dd>
            </div>
          })}
        </dl>
      </div>}
    </section>
  )
}
