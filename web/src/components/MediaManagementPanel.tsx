import type { ReactNode } from 'react'
import { Database, FileText, FolderInput, Loader2, Pencil, Search, SlidersHorizontal, Sparkles, Trash2 } from 'lucide-react'

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
  const actions = [
    { key: 'scrape', label: '智能刮削', pending: '正在刮削…', icon: Sparkles, onClick: onSmartScrape },
    { key: 'match', label: manualMatchLabel, icon: Search, onClick: onManualScrape },
    { key: 'metadata', label: '编辑元数据', icon: Pencil, onClick: onMetadataEdit },
    { key: 'probe', label: '探测媒体轨', pending: '正在探测…', icon: Database, onClick: onProbe },
    { key: 'nfo', label: '导出 NFO', pending: '正在导出…', icon: FileText, onClick: onExportNFO },
    { key: 'organize', label: '整理入库', pending: '正在整理…', icon: FolderInput, onClick: onOrganize },
  ]

  return (
    <section className="media-management-panel" aria-label={`媒体管理 · ${scope}`}>
      <header className="media-management-header">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <h2 className="inline-flex items-center gap-2 text-[13px] font-semibold"><SlidersHorizontal size={14} aria-hidden="true" />媒体管理</h2>
          <span className="text-xs text-[var(--app-muted)]">{scope}</span>
        </div>
        {headerActions}
      </header>
      <div className="media-management-actions">
        {actions.map(({ key, label, pending, icon: Icon, onClick }) => (
          <button key={key} type="button" onClick={onClick} disabled={!!busy} className="media-management-action" title={`${label} · ${scope}`}>
            {busy === key ? <Loader2 size={14} className="animate-spin" aria-hidden="true" /> : <Icon size={14} aria-hidden="true" />}
            <span>{busy === key ? pending || label : label}</span>
          </button>
        ))}
        {segmentAction}
        <button type="button" onClick={onDelete} disabled={!!busy} className="media-management-action media-management-delete" title={`删除 · ${scope}`}>
          {busy === 'delete' ? <Loader2 size={14} className="animate-spin" aria-hidden="true" /> : <Trash2 size={14} aria-hidden="true" />}
          <span>{busy === 'delete' ? '正在删除…' : '删除'}</span>
        </button>
      </div>
    </section>
  )
}
