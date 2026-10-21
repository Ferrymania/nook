import { useEffect, useState } from 'react'
import { Activity } from 'lucide-react'

import { statsAPI, type TopUserEntry } from '../api/stats'
import type { User } from '../types'

// ActiveUsersStrip 展示观看记录最多的几个账号及最近上报的设备活动。
//
// 只做展示与定位，不做筛选：管理员关心的是「谁在用」，点一下滚动到该用户
// 行即可继续原来的操作。非管理员拿到 403 时静默隐藏，避免出现红色报错条。
export function ActiveUsersStrip({ users }: { users: User[] }) {
  const [top, setTop] = useState<TopUserEntry[]>([])

  useEffect(() => {
    let cancelled = false
    statsAPI
      .topUsers(5)
      .then((rows) => {
        if (!cancelled) setTop(rows)
      })
      .catch(() => {
        if (!cancelled) setTop([])
      })
    return () => {
      cancelled = true
    }
  }, [])

  if (top.length === 0) return null

  const knownIDs = new Set(users.map((u) => u.id))

  const focusUser = (userID: string) => {
    const row = document.getElementById(`admin-user-${userID}`)
    if (row) {
      row.scrollIntoView({ behavior: 'smooth', block: 'center' })
    }
  }

  return (
    <section className="glass-panel space-y-3">
      <div className="flex flex-wrap items-center gap-3">
        <span className="flex items-center gap-1.5 text-xs font-semibold text-ink-100">
          <Activity size={14} className="text-brand-500" />
          活跃用户 Top {top.length}
        </span>
        <span className="text-xs text-sand-500">按观看记录数排序</span>
      </div>
      <div className="flex flex-wrap gap-2">
        {top.map((entry) => {
          // 已删除的账号仍会出现在历史统计里，此时不可点击。
          const exists = knownIDs.has(entry.user_id)
          const device = [entry.device_name, entry.client]
            .filter((value, index, values) => value && values.indexOf(value) === index)
            .join(' · ')
          return (
            <button
              key={entry.user_id}
              type="button"
              disabled={!exists}
              onClick={() => focusUser(entry.user_id)}
              title={exists ? '定位到该用户' : '该账号已不存在'}
              className="min-w-56 max-w-full rounded-xl border border-sand-200 bg-white px-3 py-2.5 text-left text-xs text-ink-600 transition-colors enabled:hover:border-brand-300 enabled:hover:text-brand-600 disabled:opacity-50"
            >
              <span className="flex flex-wrap items-center gap-x-2 gap-y-1 font-semibold">
                {entry.username || entry.user_id}
                <span className="text-[11px] font-normal text-sand-500">
                  {entry.plays} 条观看记录
                </span>
              </span>
              <span className="mt-1.5 block text-[11px] text-sand-500">
                最近活跃：{formatActivityTime(entry.last_active_at)}
              </span>
              <span className="mt-1 block wrap-break-word text-[11px] text-sand-500">
                最近设备：{device || '未上报'}
              </span>
            </button>
          )
        })}
      </div>
    </section>
  )
}

function formatActivityTime(value?: string | null) {
  if (!value) return '未记录'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '未记录'
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}
