import type { Library } from '../types'
import { EpisodeArtworkToggle } from '../components/EpisodeArtworkToggle'
import { MediaSortDropdown } from '../components/MediaSortDropdown'
import { useState } from 'react'
import { Settings2 } from 'lucide-react'
import { LibraryScrapeSettingsDialog } from './LibraryScrapeSettingsDialog'
import type { SortField, SortOrder } from '../utils/mediaSort'
import { libraryDisplayPath } from './libraryDisplayModel'

type LibraryPageHeaderProps = {
  library: Library | null
  itemCount: number
  loadingAllText: string
  scanProgress: string
  isAdmin: boolean
  scrapeEpisodeArtwork: boolean
  onLibraryUpdated: (library: Library) => void
  scanning: boolean
  scraping: boolean
  repairing: boolean
  sortField: SortField
  sortOrder: SortOrder
  onSortChange: (field: SortField, order: SortOrder) => void
  onReshuffle: () => void
  onScrapeEpisodeArtworkChange: (checked: boolean) => void
  onScan: () => void
  onScrape: () => void
  onRepairRescrape: () => void
}

export function LibraryPageHeader({
  library,
  itemCount,
  loadingAllText,
  scanProgress,
  isAdmin,
  scrapeEpisodeArtwork,
  onLibraryUpdated,
  scanning,
  scraping,
  repairing,
  sortField,
  sortOrder,
  onSortChange,
  onReshuffle,
  onScrapeEpisodeArtworkChange,
  onScan,
  onScrape,
  onRepairRescrape,
}: LibraryPageHeaderProps) {
  const [settingsOpen, setSettingsOpen] = useState(false)
  const displayPath = library ? libraryDisplayPath(library.path) : ''

  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h1 className="font-display text-2xl font-bold text-ink-600 sm:text-3xl">
          {library?.name ?? '媒体库'}
          <span className="text-sand-500"> ({itemCount})</span>
        </h1>
        {library && <p className="text-xs text-ink-50 sm:text-sm" title={library.path}>{library.type} · {displayPath}</p>}
        {loadingAllText && <p className="mt-1 text-xs text-sand-500">{loadingAllText}</p>}
        {scanProgress && <p className="mt-1 text-xs text-brand-500">{scanProgress}</p>}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <MediaSortDropdown
          value={sortField}
          order={sortOrder}
          onChange={onSortChange}
          onReshuffle={onReshuffle}
        />
        {isAdmin && !library?.is_remote_emby && (
          <>
            <EpisodeArtworkToggle
              checked={scrapeEpisodeArtwork}
              onChange={onScrapeEpisodeArtworkChange}
              title="关闭后仍会获取主海报和每集文字元数据，只跳过每集图片"
              className="h-9 text-xs sm:text-sm"
            />
            <button type="button" onClick={() => setSettingsOpen(true)} disabled={!library} style={{ backgroundColor: 'var(--app-brand-soft)', borderColor: 'var(--app-brand-border)', color: 'var(--app-brand-text)' }} className="btn-outline !px-3 !py-1.5 text-xs sm:text-sm"><Settings2 size={16} />刮削设置</button>
            <button
              onClick={onScan}
              disabled={scanning}
              className="btn-outline !px-3 !py-1.5 text-xs sm:!px-4 sm:!py-2.5 sm:text-sm"
            >
              {scanning ? '扫描中…' : '扫描媒体库'}
            </button>
            <button
              onClick={onScrape}
              disabled={scraping}
              className="btn-outline !px-3 !py-1.5 text-xs sm:!px-4 sm:!py-2.5 sm:text-sm"
              title="对整个媒体库执行刮削元数据"
            >
              {scraping ? '刮削中…' : '整库刮削元数据'}
            </button>
            <button
              onClick={onRepairRescrape}
              disabled={repairing}
              className="btn-outline !px-3 !py-1.5 text-xs sm:!px-4 sm:!py-2.5 sm:text-sm"
              title="回填本库占位符外部 ID 并重刮，修正空 ID / 拆集问题"
            >
              {repairing ? '修复中…' : '修复+重刮整库'}
            </button>
          </>
        )}
      </div>
      {settingsOpen && library && <LibraryScrapeSettingsDialog key={library.id} library={library} onClose={() => setSettingsOpen(false)} onSaved={autoProbe => onLibraryUpdated({ ...library, auto_probe: autoProbe })} />}
    </div>
  )
}
