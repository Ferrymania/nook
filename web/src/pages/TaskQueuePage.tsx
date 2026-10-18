import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Download, Sparkles, Upload } from 'lucide-react'
import clsx from 'clsx'

import type { ScrapeQueueCounts } from '../types/scraper'
import type { StrmQueueCounts } from '../types/strm'
import { ScraperQueuePage } from './ScraperQueuePage'
import { StrmQueuePanel } from './StrmQueuePage'

type QueueType = 'scrape' | 'download' | 'upload'
const QUEUE_TYPE_STORAGE_KEY = 'nook_task_queue_type'

const QUEUE_TABS: { type: QueueType; label: string; icon: typeof Sparkles }[] = [
  { type: 'scrape', label: '刮削队列', icon: Sparkles },
  { type: 'download', label: '下载队列', icon: Download },
  { type: 'upload', label: '上传队列', icon: Upload },
]

function isQueueType(raw: string | null): raw is QueueType {
  return raw === 'scrape' || raw === 'download' || raw === 'upload'
}

function preferredQueueType(): QueueType {
  try {
    const saved = localStorage.getItem(QUEUE_TYPE_STORAGE_KEY)
    if (isQueueType(saved)) return saved
  } catch {
    // Storage may be disabled; explicit links and the default still work.
  }
  return 'scrape'
}

/**
 * 任务队列：整合刮削 / 下载 / 上传 三类队列的统一入口，按类型 Tab 区分。
 * 只挂载当前队列，复用其统计和轮询；有效链接优先，否则恢复上次类型。
 */
export function TaskQueuePage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const rawType = searchParams.get('type')
  const type = isQueueType(rawType) ? rawType : preferredQueueType()

  useEffect(() => {
    try {
      localStorage.setItem(QUEUE_TYPE_STORAGE_KEY, type)
    } catch {
      // Remembering the selection is optional.
    }
    if (rawType !== type) {
      setSearchParams((previous) => {
        const next = new URLSearchParams(previous)
        next.set('type', type)
        return next
      }, { replace: true })
    }
  }, [rawType, type, setSearchParams])

  const [scrapeCounts, setScrapeCounts] = useState<ScrapeQueueCounts | null>(null)
  const [downloadCounts, setDownloadCounts] = useState<StrmQueueCounts | null>(null)
  const [uploadCounts, setUploadCounts] = useState<StrmQueueCounts | null>(null)

  const countBadge = (counts: ScrapeQueueCounts | StrmQueueCounts | null) => {
    if (!counts) return null
    const total = Object.values(counts).reduce((a, b) => a + (b || 0), 0)
    return <span className="tab-count">{total}</span>
  }

  const switchType = (next: QueueType) => {
    setSearchParams((previous) => {
      const params = new URLSearchParams(previous)
      params.set('type', next)
      return params
    }, { replace: true })
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="font-display text-3xl font-bold text-ink-600">任务队列</h1>
          <p className="mt-0.5 text-sm text-ink-50">刮削 / 下载 / 上传三类任务统一管理，按类型切换查看</p>
        </div>
      </div>

      {/* 类型 Tab */}
      <div className="flex flex-wrap gap-2 border-b border-sand-200 pb-3">
        {QUEUE_TABS.map((tab) => {
          const active = type === tab.type
          const Icon = tab.icon
          const badge =
            tab.type === 'scrape'
              ? countBadge(scrapeCounts)
              : tab.type === 'download'
                ? countBadge(downloadCounts)
                : tab.type === 'upload'
                  ? countBadge(uploadCounts)
                  : null
          return (
            <button
              key={tab.type}
              onClick={() => switchType(tab.type)}
              className={clsx(
                'inline-flex items-center gap-1.5 rounded-full px-4 py-2 text-sm font-semibold transition',
                active ? 'bg-brand-500 text-white shadow-sm' : 'text-ink-100 hover:bg-brand-50 hover:text-brand-600',
              )}
            >
              <Icon size={14} />
              {tab.label}
              {badge}
            </button>
          )
        })}
      </div>

      {/* 内容 */}
      {type === 'scrape' && <ScraperQueuePage embedded onCountsChange={setScrapeCounts} />}
      {type === 'download' && <StrmQueuePanel kind="download" embedded onCountsChange={setDownloadCounts} />}
      {type === 'upload' && <StrmQueuePanel kind="upload" embedded onCountsChange={setUploadCounts} />}
    </div>
  )
}
