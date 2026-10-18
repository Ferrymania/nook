import type { Library } from '../types'
import { LibraryScrapeMenu } from '../components/LibraryScrapeMenu'
import { MediaSortDropdown } from '../components/MediaSortDropdown'
import { useState } from 'react'
import { Loader2, ScanLine, Settings2 } from 'lucide-react'
import { LibraryScrapeSettingsDialog } from './LibraryScrapeSettingsDialog'
import type { SortField, SortOrder } from '../utils/mediaSort'
import { libraryDisplayPath } from './libraryDisplayModel'

type LibraryPageHeaderProps = {
  library: Library | null
  itemCount: number
  loadingAllText: string
  scanProgress: string
  isAdmin: boolean
  onLibraryUpdated: (library: Library) => void
  scanLabel: string
  scanning: boolean
  scraping: boolean
  repairing: boolean
  sortField: SortField
  sortOrder: SortOrder
  onSortChange: (field: SortField, order: SortOrder) => void
  onReshuffle: () => void
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
  onLibraryUpdated,
  scanLabel,
  scanning,
  scraping,
  repairing,
  sortField,
  sortOrder,
  onSortChange,
  onReshuffle,
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
        {scanProgress && <p role="status" className="mt-1 text-xs text-[var(--app-muted)]">{scanProgress}</p>}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <MediaSortDropdown
          value={sortField}
          order={sortOrder}
          onChange={onSortChange}
          onReshuffle={onReshuffle}
          buttonClassName="library-toolbar-button"
        />
        {isAdmin && !library?.is_remote_emby && (
          <>
            <button type="button" onClick={onScan} disabled={scanning} aria-busy={scanning} className="library-toolbar-button" title="立即扫描当前媒体库">
              {scanning ? <Loader2 size={16} className="animate-spin" /> : <ScanLine size={16} />}
              {scanLabel}
            </button>
            <LibraryScrapeMenu scraping={scraping} repairing={repairing} onScrape={onScrape} onRepair={onRepairRescrape} />
            <button type="button" onClick={() => setSettingsOpen(true)} disabled={!library} aria-haspopup="dialog" className="library-toolbar-button" title="打开刮削设置"><Settings2 size={16} />刮削设置…</button>
          </>
        )}
      </div>
      {settingsOpen && library && <LibraryScrapeSettingsDialog key={library.id} library={library} onClose={() => setSettingsOpen(false)} onSaved={autoProbe => onLibraryUpdated({ ...library, auto_probe: autoProbe })} />}
    </div>
  )
}
