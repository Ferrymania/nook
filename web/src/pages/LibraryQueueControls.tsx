import { Inbox, Loader2, Search, X, type LucideIcon } from 'lucide-react'

type QueueStatusTone = 'neutral' | 'muted' | 'progress' | 'success' | 'warning' | 'danger'

export type QueueFilter<Key extends string> = {
  key: Key
  label: string
  icon: LucideIcon
  tone: QueueStatusTone
}

export function QueueStatusFilters<Key extends string>({ filters, selected, count, onSelect }: {
  filters: readonly QueueFilter<Key>[]
  selected: Key
  count: (key: Key) => number | undefined
  onSelect: (key: Key) => void
}) {
  return (
    <div className="library-queue-filters" role="group" aria-label="按状态筛选">
      {filters.map(({ key, label, icon: Icon, tone }) => (
        <button
          key={key}
          type="button"
          className={`library-queue-filter flex min-w-0 flex-col justify-between rounded-2xl border p-3.5 text-left transition-all duration-200 select-none ${
            selected === key
              ? 'border-brand-500 bg-primary-400/10 shadow-xs ring-2 ring-brand-500/20'
              : 'border-gray-200 bg-white/80 hover:border-gray-300 hover:bg-white'
          }`}
          aria-pressed={selected === key}
          onClick={() => onSelect(key)}
        >
          <span className="flex items-center justify-between gap-2 text-xs text-sand-500">
            <span className="font-semibold">{label}</span>
            <Icon size={14} className="library-queue-tone shrink-0" data-tone={tone} aria-hidden="true" />
          </span>
          <span className="mt-2 flex items-baseline gap-1">
            <span className="library-queue-tone font-display text-2xl font-black tabular-nums" data-tone={tone}>{count(key) ?? '—'}</span>
            <span className="text-[10px] font-medium text-sand-400">项</span>
          </span>
        </button>
      ))}
    </div>
  )
}

export function QueueSearch({ value, label, placeholder, maxLength, onChange }: {
  value: string
  label: string
  placeholder: string
  maxLength?: number
  onChange: (value: string) => void
}) {
  return (
    <div className="library-queue-search relative min-w-0">
      <Search size={15} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" aria-hidden="true" />
      <input
        type="text"
        aria-label={label}
        placeholder={placeholder}
        value={value}
        maxLength={maxLength}
        onChange={(event) => onChange(event.target.value)}
        className="h-9 w-full rounded-xl border border-gray-200 bg-white pr-8 pl-9 text-xs text-ink-600 outline-hidden transition placeholder:text-gray-400 focus:border-brand-500 focus:ring-2 focus:ring-brand-100/60"
      />
      {value && (
        <button
          type="button"
          aria-label="清空搜索"
          className="absolute right-2.5 top-1/2 -translate-y-1/2 rounded-sm p-0.5 text-gray-400 hover:text-ink-600"
          onClick={() => onChange('')}
        >
          <X size={13} aria-hidden="true" />
        </button>
      )}
    </div>
  )
}

export function QueueTableMessage({ loading, children }: { loading?: boolean; children: string }) {
  const Icon = loading ? Loader2 : Inbox
  return (
    <div className="flex flex-col items-center gap-2 border-t border-(--app-border) px-4 py-8 text-center text-sm text-(--app-muted)" role={loading ? 'status' : undefined}>
      <Icon size={22} className={loading ? 'animate-spin' : undefined} aria-hidden="true" />
      <span>{children}</span>
    </div>
  )
}

export function QueuePagination({ total, page, pages, disabled, onPageChange }: {
  total: number
  page: number
  pages: number
  disabled?: boolean
  onPageChange: (page: number) => void
}) {
  return (
    <nav className="library-queue-pagination border-t border-gray-200/80 bg-gray-50/40 text-xs text-sand-500" aria-label="队列分页">
      <span className="tabular-nums">共 {total} 条 · 第 {page} / {pages} 页</span>
      <div className="flex gap-2">
        <button type="button" className="library-queue-action" disabled={disabled || page <= 1} onClick={() => onPageChange(page - 1)}>上一页</button>
        <button type="button" className="library-queue-action" disabled={disabled || page >= pages} onClick={() => onPageChange(page + 1)}>下一页</button>
      </div>
    </nav>
  )
}
