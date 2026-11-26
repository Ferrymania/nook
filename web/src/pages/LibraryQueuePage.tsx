import { lazy, Suspense } from 'react'
import { ArrowLeft, ListVideo, Sparkles } from 'lucide-react'
import { Link, useLocation } from 'react-router-dom'

import { resolveHeaderBack } from '../components/layoutNavigation'
import { libraryQueueLocation, type LibraryQueueTab } from '../utils/libraryQueueNavigation'
import './library-queue.css'

const ScraperQueuePanel = lazy(() => import('./ScraperQueuePage').then((m) => ({ default: m.ScraperQueuePanel })))
const SegmentQueuePanel = lazy(() => import('./SegmentQueuePage').then((m) => ({ default: m.SegmentQueuePanel })))

const tabs = [
  { value: 'scrape', label: '刮削', icon: Sparkles },
  { value: 'segments', label: '片头片尾', icon: ListVideo },
] as const

export function LibraryQueuePage() {
  const { pathname, search, state } = useLocation()
  const activeTab: LibraryQueueTab = new URLSearchParams(search).get('tab') === 'segments' ? 'segments' : 'scrape'
  const back = resolveHeaderBack(pathname, search, state?.segmentReturnTo)

  return (
    <div className="library-queue space-y-4">
      <header className="flex items-center justify-between gap-3">
        <h1 className="font-display text-2xl font-bold text-(--app-text) sm:text-3xl">任务队列</h1>
        {back && (
          <Link to={back.to} className="btn-outline hidden lg:inline-flex">
            <ArrowLeft size={16} aria-hidden="true" />
            {back.label}
          </Link>
        )}
      </header>

      <nav aria-label="队列类型" className="flex gap-5 border-b border-(--app-border)">
        {tabs.map(({ value, label, icon: Icon }) => (
          <Link
            key={value}
            to={libraryQueueLocation(value, search)}
            state={state}
            aria-current={activeTab === value ? 'page' : undefined}
            className={`-mb-px inline-flex min-h-11 items-center gap-2 border-b-2 py-2 text-sm font-medium transition-colors ${
              activeTab === value
                ? 'border-(--app-text) text-(--app-text)'
                : 'border-transparent text-(--app-muted) hover:text-(--app-text)'
            }`}
          >
            <Icon size={16} aria-hidden="true" />
            {label}
          </Link>
        ))}
      </nav>

      <Suspense fallback={<p role="status" className="py-8 text-sm text-(--app-muted)">正在加载队列…</p>}>
        {activeTab === 'segments' ? <SegmentQueuePanel /> : <ScraperQueuePanel />}
      </Suspense>
    </div>
  )
}
