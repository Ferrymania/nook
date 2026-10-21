import { Fragment, type ReactNode } from 'react'
import { Loader2, SlidersHorizontal } from 'lucide-react'
import { MEDIA_MANAGEMENT_ACTIONS, type MediaManagementActionKey } from './mediaManagementActions'

type MediaManagementPanelProps = {
  scope: string
  busy?: string
  headerActions?: ReactNode
  segmentAction: ReactNode
  manualMatchLabel?: string
  onSmartScrape: () => void
  onManualScrape: () => void
  onMetadataEdit: () => void
  onProbe: () => void
  onExportNFO: () => void
  onOrganize: () => void
  onDelete: () => void
}

export function MediaManagementPanel({
  scope, busy = '', headerActions, segmentAction, manualMatchLabel = '手动匹配',
  onSmartScrape, onManualScrape, onMetadataEdit, onProbe, onExportNFO, onOrganize, onDelete,
}: MediaManagementPanelProps) {
  const handlers: Record<Exclude<MediaManagementActionKey, 'segments'>, () => void> = {
    scrape: onSmartScrape, match: onManualScrape, metadata: onMetadataEdit,
    probe: onProbe, nfo: onExportNFO, organize: onOrganize, delete: onDelete,
  }

  return (
    <section className="media-management-panel" aria-label={`媒体管理 · ${scope}`}>
      <header className="media-management-header">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <h2 className="inline-flex items-center gap-2 text-[13px] font-semibold"><SlidersHorizontal size={14} aria-hidden="true" />媒体管理</h2>
          <span className="text-xs text-(--app-muted)">{scope}</span>
        </div>
        {headerActions}
      </header>
      <div className="media-management-actions">
        {MEDIA_MANAGEMENT_ACTIONS.map(action => {
          const { key, icon: Icon } = action
          if (key === 'segments') return <Fragment key={key}>{segmentAction}</Fragment>
          const label = key === 'match' ? manualMatchLabel : action.label
          return <button key={key} type="button" onClick={handlers[key]} disabled={!!busy} className={`media-management-action${'danger' in action ? ' media-management-delete' : ''}`} title={`${label} · ${scope}`}>
            {busy === key ? <Loader2 size={14} className="animate-spin" aria-hidden="true" /> : <Icon size={14} aria-hidden="true" />}
            <span>{busy === key && 'pending' in action ? action.pending : label}</span>
          </button>
        })}
      </div>
    </section>
  )
}
