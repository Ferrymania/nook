import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Download, ListChecks, Sparkles, Upload } from 'lucide-react'
import clsx from 'clsx'

import type { ScrapeQueueCounts } from '../types/scraper'
import type { StrmQueueCounts } from '../types/strm'
import { ScraperQueuePage } from './ScraperQueuePage'
import { StrmQueuePanel } from './StrmQueuePage'

type QueueType = 'all' | 'scrape' | 'download' | 'upload'

const QUEUE_TABS: { type: QueueType; label: string; icon: typeof ListChecks }[] = [
  { type: 'all', label: '全部', icon: ListChecks },
  { type: 'scrape', label: '刮削队列', icon: Sparkles },
  { type: 'download', label: '下载队列', icon: Download },
  { type: 'upload', label: '上传队列', icon: Upload },
]

function normalizeType(raw: string | null): QueueType {
  if (raw === 'scrape' || raw === 'download' || raw === 'upload' || raw === 'all') {
    return raw
  }
  return 'all'
}

/**
 * 任务队列：整合刮削 / 下载 / 上传 三类队列的统一入口，按类型 Tab 区分。
 * 单个类型视图直接复用原队列组件；「全部」纵向堆叠三个队列。
 */
export function TaskQueuePage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const type = normalizeType(searchParams.get('type'))

  const [scrapeCounts, setScrapeCounts] = useState<ScrapeQueueCounts | null>(null)
  const [downloadCounts, setDownloadCounts] = useState<StrmQueueCounts | null>(null)
  const [uploadCounts, setUploadCounts] = useState<StrmQueueCounts | null>(null)

  const countBadge = (counts: ScrapeQueueCounts | StrmQueueCounts | null) => {
    if (!counts) return null
    const total = Object.values(counts).reduce((a, b) => a + (b || 0), 0)
    return <span className="tab-count">{total}</span>
  }

  const switchType = (next: QueueType) => {
    setSearchParams(next === 'all' ? {} : { type: next }, { replace: true })
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
      {type === 'all' && (
        <div className="space-y-10">
          <section>
            <h2 className="mb-2 flex items-center gap-2 font-display text-xl font-bold text-ink-600">
              <Sparkles size={18} className="text-brand-500" />
              刮削队列
            </h2>
            <ScraperQueuePage embedded onCountsChange={setScrapeCounts} />
          </section>
          <section>
            <h2 className="mb-2 flex items-center gap-2 font-display text-xl font-bold text-ink-600">
              <Download size={18} className="text-brand-500" />
              下载队列
            </h2>
            <StrmQueuePanel kind="download" embedded onCountsChange={setDownloadCounts} />
          </section>
          <section>
            <h2 className="mb-2 flex items-center gap-2 font-display text-xl font-bold text-ink-600">
              <Upload size={18} className="text-brand-500" />
              上传队列
            </h2>
            <StrmQueuePanel kind="upload" embedded onCountsChange={setUploadCounts} />
          </section>
        </div>
      )}
    </div>
  )
}
