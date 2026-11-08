import { useState, type ReactNode } from 'react'
import { AlertTriangle } from 'lucide-react'

export type ConfirmOptions = {
  title?: string
  message: string
  confirmText?: string
  cancelText?: string
  danger?: boolean
  checkboxLabel?: string
  checkboxDefaultChecked?: boolean
  alternateCheckbox?: {
    label: string
    loadPreview: () => Promise<{ details: ReactNode; canConfirm: boolean }>
  }
  details?: ReactNode
}

export type ConfirmResult = {
  confirmed: boolean
  checked: boolean
  alternate?: boolean
}

export function ConfirmDialog({
  options,
  onClose,
}: {
  options: ConfirmOptions
  onClose: (value: ConfirmResult) => void
}) {
  const danger = options.danger ?? true
  const hasCheckbox = Boolean(options.checkboxLabel)
  const [checked, setChecked] = useState(Boolean(options.checkboxDefaultChecked))
  const [alternate, setAlternate] = useState(false)
  const [preview, setPreview] = useState<{ details: ReactNode; canConfirm: boolean }>()
  const [loadingPreview, setLoadingPreview] = useState(false)
  const [previewFailed, setPreviewFailed] = useState(false)

  const toggleAlternate = (selected: boolean) => {
    setAlternate(selected)
    if (!selected || !options.alternateCheckbox || preview || loadingPreview) return
    setLoadingPreview(true)
    setPreviewFailed(false)
    void options.alternateCheckbox.loadPreview()
      .then(setPreview)
      .catch(() => setPreviewFailed(true))
      .finally(() => setLoadingPreview(false))
  }
  return (
    <div
      className="fixed inset-0 z-100 flex items-center justify-center bg-black/35 p-4 backdrop-blur-xs"
      onClick={() => onClose({ confirmed: false, checked })}
    >
      <div
        role="dialog"
        aria-modal="true"
        className="flex max-h-[calc(100dvh-2rem)] w-full max-w-md flex-col overflow-hidden rounded-3xl border border-white/70 bg-white shadow-2xl"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex min-h-0 gap-4 overflow-y-auto p-5">
          <div
            className={
              'flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl ' +
              (danger ? 'bg-red-50 text-red-500' : 'bg-primary-400/10 text-brand-500')
            }
          >
            <AlertTriangle size={22} />
          </div>
          <div className="min-w-0 flex-1">
            <h3 className="font-display text-lg font-bold text-ink-600">
              {options.title || '确认操作'}
            </h3>
            <p className="mt-2 whitespace-pre-line text-sm leading-6 text-ink-50">{options.message}</p>
            {options.details}
            {hasCheckbox ? (
              <label className="mt-4 flex cursor-pointer items-start gap-2.5 text-sm text-ink-100 has-disabled:cursor-default has-disabled:opacity-60">
                <input
                  type="checkbox"
                  className="mt-0.5 size-4 shrink-0 rounded-sm border-gray-300 text-red-500 focus:ring-red-400"
                  checked={checked || alternate}
                  disabled={alternate}
                  onChange={(event) => setChecked(event.target.checked)}
                />
                <span>{options.checkboxLabel}</span>
              </label>
            ) : null}
            {options.alternateCheckbox && (
              <label className="mt-2 flex cursor-pointer items-start gap-2.5 text-sm text-ink-100">
                <input
                  type="checkbox"
                  className="mt-0.5 size-4 shrink-0 rounded-sm border-gray-300 text-red-500 focus:ring-red-400"
                  checked={alternate}
                  onChange={(event) => toggleAlternate(event.target.checked)}
                />
                <span>{options.alternateCheckbox.label}</span>
              </label>
            )}
            {alternate && (
              <div className="mt-3" aria-live="polite">
                {loadingPreview && <p className="text-sm text-ink-50">正在核对网盘原片…</p>}
                {previewFailed && <p className="text-sm text-red-700">清单加载失败，请取消勾选后重试。</p>}
                {preview?.details}
              </div>
            )}
          </div>
        </div>
        <div className="flex shrink-0 justify-end gap-2 border-t border-gray-100 bg-gray-50/80 px-5 py-4">
          <button
            type="button"
            onClick={() => onClose({ confirmed: false, checked })}
            className="rounded-xl border border-gray-200 bg-white px-4 py-2 text-sm font-semibold text-ink-100 hover:bg-gray-50"
          >
            {options.cancelText || '取消'}
          </button>
          <button
            type="button"
            disabled={alternate && !preview?.canConfirm}
            onClick={() => onClose({ confirmed: true, checked, alternate })}
            className={
              'rounded-xl px-4 py-2 text-sm font-semibold text-white shadow-xs transition disabled:cursor-not-allowed disabled:opacity-50 ' +
              (danger ? 'bg-red-500 hover:bg-red-600' : 'bg-brand-500 hover:bg-brand-600')
            }
          >
            {options.confirmText || '确认'}
          </button>
        </div>
      </div>
    </div>
  )
}
