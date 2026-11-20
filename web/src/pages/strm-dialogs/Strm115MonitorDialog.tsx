import { useEffect, useState } from 'react'

import { strmAPI } from '../../api/strm'
import type { Strm115RequestHour, Strm115RequestStats, StrmAccount } from '../../types/strm'
import { DialogShell } from '../StrmDialogs'

const countFormat = new Intl.NumberFormat('zh-CN')
const hourFormat = new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' })
const hourDetailFormat = new Intl.DateTimeFormat('zh-CN', {
  month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
})

export function Strm115MonitorDialog({ account, onClose }: { account: StrmAccount; onClose: () => void }) {
  const [stats, setStats] = useState<Strm115RequestStats | null>(null)
  const [error, setError] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    let timer: number

    const refresh = async () => {
      if (!document.hidden) {
        try {
          const next = await strmAPI.get115RequestStats(account.id, controller.signal)
          if (!controller.signal.aborted) {
            setStats(next)
            setError(false)
          }
        } catch {
          if (!controller.signal.aborted) setError(true)
        }
      }
      if (!controller.signal.aborted) timer = window.setTimeout(refresh, 3000)
    }

    void refresh()
    return () => {
      controller.abort()
      window.clearTimeout(timer)
    }
  }, [account.id])

  return (
    <DialogShell title="115 接口监控" subtitle={`${account.name} · 本次运行的开放平台请求`} onClose={onClose} wide>
      <div className="space-y-5">
        {error && <p role="status" className="text-sm text-rose-600">获取统计失败，正在重试{stats && '。当前显示上次结果'}</p>}
        {!stats && !error && <p role="status" className="py-8 text-center text-sm text-ink-50">正在加载请求统计…</p>}
        {stats && <>
          <dl className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            {[
              ['QPS · 最近 1 秒', countFormat.format(stats.requests_last_second)],
              ['QPM · 最近 1 分钟', countFormat.format(stats.requests_last_minute)],
              ['QPH · 最近 1 小时', countFormat.format(stats.requests_last_hour)],
              ['平均响应 · 最近 1 小时', stats.average_response_ms === null ? '—' : `${countFormat.format(Math.round(stats.average_response_ms))} ms`],
            ].map(([label, value]) => (
              <div key={label} className="min-w-0 rounded-xl border border-(--app-border) bg-(--app-panel-soft) px-3 py-2.5">
                <dt className="text-xs text-sand-500">{label}</dt>
                <dd className="mt-1 text-xl font-semibold break-all tabular-nums text-ink-600">{value}</dd>
              </div>
            ))}
          </dl>
          <RequestHistory hours={stats.hourly} />
        </>}
      </div>
    </DialogShell>
  )
}

function RequestHistory({ hours }: { hours: Strm115RequestHour[] }) {
  const [selectedStart, setSelectedStart] = useState<number | null>(null)
  const last = hours[hours.length - 1]
  const first = hours[0]
  if (!last || !first) return null
  const selected = hours.find(hour => hour.start_at === selectedStart) ?? last
  const maxRequests = Math.max(1, ...hours.map(hour => hour.requests))

  return (
    <section className="space-y-2">
      <h4 className="text-sm font-semibold text-ink-600">最近 24 个小时段</h4>
      <p className="text-xs tabular-nums text-sand-500">
        {hourDetailFormat.format(selected.start_at * 1000)} · {countFormat.format(selected.requests)} 次请求 · {countFormat.format(selected.throttled)} 次触发限流
      </p>
      <div className="grid h-28 grid-cols-[repeat(24,minmax(0,1fr))] gap-0.5 sm:gap-1">
        {hours.map(hour => (
          <button key={hour.start_at} type="button"
            aria-label={`${hourDetailFormat.format(hour.start_at * 1000)}，${hour.requests} 次请求，${hour.throttled} 次触发限流`}
            aria-pressed={selected.start_at === hour.start_at}
            className="flex min-w-0 cursor-pointer flex-col justify-end rounded bg-(--app-panel-soft) focus-visible:outline-2 aria-pressed:ring-1 aria-pressed:ring-brand-500"
            onClick={() => setSelectedStart(hour.start_at)}>
            <span aria-hidden="true" className="min-h-0.5 w-full overflow-hidden rounded bg-brand-500"
              style={{ height: `${hour.requests / maxRequests * 100}%` }}>
              {hour.throttled > 0 && <span className="block bg-amber-500" style={{ height: `${hour.throttled / hour.requests * 100}%` }} />}
            </span>
          </button>
        ))}
      </div>
      <div className="flex justify-between text-xs tabular-nums text-sand-500">
        <span>{hourFormat.format(first.start_at * 1000)}</span>
        <span>{hourFormat.format(last.start_at * 1000)}</span>
      </div>
    </section>
  )
}
