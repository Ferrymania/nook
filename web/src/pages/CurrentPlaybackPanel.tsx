import { useEffect, useState } from 'react'
import { Loader2, MonitorPlay, Pause, Play } from 'lucide-react'

import { adminAPI, type CurrentPlaybackSession } from '../api/admin'

export function CurrentPlaybackPanel() {
  const [sessions, setSessions] = useState<CurrentPlaybackSession[] | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let disposed = false
    let request: AbortController | null = null
    let timer: number | undefined
    const refresh = async () => {
      if (disposed || document.hidden || request) return
      const controller = new AbortController()
      request = controller
      try {
        const rows = await adminAPI.playbackSessions(controller.signal)
        if (!disposed && !controller.signal.aborted) {
          setSessions(rows)
          setFailed(false)
        }
      } catch {
        if (!disposed && !controller.signal.aborted) setFailed(true)
      } finally {
        if (request === controller) request = null
      }
    }
    const resume = () => {
      window.clearInterval(timer)
      if (document.hidden) {
        request?.abort()
        request = null
        return
      }
      void refresh()
      timer = window.setInterval(() => void refresh(), 10000)
    }
    resume()
    document.addEventListener('visibilitychange', resume)
    return () => {
      disposed = true
      window.clearInterval(timer)
      request?.abort()
      document.removeEventListener('visibilitychange', resume)
    }
  }, [])

  return (
    <section className="glass-panel space-y-3" aria-label="当前播放">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="flex items-center gap-1.5 text-xs font-semibold text-ink-100">
          <MonitorPlay size={14} className="text-brand-500" />
          当前播放{!failed && sessions !== null && `（${sessions.length}）`}
        </h2>
        <span className="text-[11px] text-sand-500">页面可见时每 10 秒刷新</span>
      </div>
      {failed ? (
        <div role="status" className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-5 text-xs text-amber-700">
          播放状态获取失败，正在播放情况暂时未知，将自动重试。
        </div>
      ) : sessions === null ? (
        <div role="status" className="flex items-center justify-center gap-2 rounded-xl bg-sand-50/60 py-6 text-xs text-sand-500">
          <Loader2 size={14} className="animate-spin" />正在加载播放状态…
        </div>
      ) : sessions.length === 0 ? (
        <div role="status" className="rounded-xl border border-dashed border-sand-200 bg-sand-50/40 px-4 py-6 text-center text-xs text-sand-500">
          暂无用户正在播放
        </div>
      ) : (
        <ul className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {sessions.map((session) => (
            <li key={session.id} className="min-w-0 space-y-2 rounded-xl border border-sand-200 bg-white p-3">
              <div className="flex items-start justify-between gap-2 text-xs">
                <div className="min-w-0">
                  <p className="break-words font-semibold text-ink-600">{session.username || '未知用户'}</p>
                  <p className="mt-1 break-words text-[11px] text-sand-500">
                    {[session.device_name, session.client].filter((v, i, all) => v && all.indexOf(v) === i).join(' · ') || '未知设备'}
                  </p>
                </div>
                <span className={`inline-flex shrink-0 items-center gap-1 rounded-md px-1.5 py-0.5 text-[10px] font-semibold ${session.is_paused ? 'bg-amber-50 text-amber-700' : 'bg-emerald-50 text-emerald-700'}`}>
                  {session.is_paused ? <Pause size={10} /> : <Play size={10} />}
                  {session.is_paused ? '已暂停' : '正在播放'}
                </span>
              </div>
              <p className="break-words text-xs font-medium text-ink-600">{session.series_name || session.title}</p>
              {session.episode_num > 0 && (
                <p className="text-[11px] text-sand-500">第 {session.season_num} 季 · 第 {session.episode_num} 集</p>
              )}
              <p className="text-[11px] tabular-nums text-sand-500">
                {formatPlaybackTime(session.position_ms)} / {session.duration_ms > 0 ? formatPlaybackTime(session.duration_ms) : '时长未知'}
              </p>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

function formatPlaybackTime(ms: number) {
  const total = Math.max(0, Math.floor(ms / 1000))
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const seconds = String(total % 60).padStart(2, '0')
  return hours > 0 ? `${hours}:${String(minutes).padStart(2, '0')}:${seconds}` : `${minutes}:${seconds}`
}
