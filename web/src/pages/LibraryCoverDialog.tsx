import { type FormEvent, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Image, Loader2, Save, X } from 'lucide-react'

import type { Library } from '../types'
import { apiErrorMessage } from './adminLibraryPanelModel'

export function LibraryCoverDialog({ library, onClose, onSave }: {
  library: Library
  onClose: () => void
  onSave: (library: Library, coverURL: string) => Promise<void>
}) {
  const [coverURL, setCoverURL] = useState(library.cover_url ?? '')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const savingRef = useRef(false)
  const formRef = useRef<HTMLFormElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const previousFocus = document.activeElement as HTMLElement | null
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    inputRef.current?.focus()
    return () => {
      document.body.style.overflow = previousOverflow
      previousFocus?.focus()
    }
  }, [])

  const close = () => { if (!savingRef.current) onClose() }
  const save = async (event: FormEvent) => {
    event.preventDefault()
    if (savingRef.current) return
    savingRef.current = true
    setSaving(true)
    setError('')
    try {
      await onSave(library, coverURL)
    } catch (err: unknown) {
      setError(apiErrorMessage(err, '保存封面失败，请重试'))
    } finally {
      savingRef.current = false
      setSaving(false)
    }
  }

  return createPortal(
    <div
      className="fixed inset-0 z-[110] flex items-center justify-center bg-black/35 p-3 backdrop-blur-sm sm:p-6"
      onClick={event => { event.stopPropagation(); close() }}
    >
      <form
        ref={formRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="library-cover-title"
        aria-describedby="library-cover-description"
        className="flex max-h-[94dvh] w-full max-w-lg flex-col overflow-hidden rounded-3xl border border-[var(--app-border)] bg-[var(--app-panel)] text-[var(--app-text)] shadow-2xl"
        onClick={event => event.stopPropagation()}
        onSubmit={save}
        onKeyDown={event => {
          if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close() }
          if (event.key !== 'Tab') return
          const targets = Array.from(formRef.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled)') ?? [])
          const first = targets[0], last = targets[targets.length - 1]
          if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
          if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
          if (!targets.length) event.preventDefault()
        }}
      >
        <header className="flex items-start justify-between gap-4 border-b border-[var(--app-border)] px-5 py-5 sm:px-6">
          <div className="flex items-start gap-3">
            <span className="rounded-xl bg-[var(--app-brand-soft)] p-2.5 text-[var(--app-brand-text)]"><Image size={20} /></span>
            <div>
              <h2 id="library-cover-title" className="font-display text-xl font-semibold">自定义封面</h2>
              <p className="mt-1 text-sm text-[var(--app-muted)]">{library.name}</p>
            </div>
          </div>
          <button type="button" aria-label="关闭封面编辑" onClick={close} disabled={saving} className="rounded-lg p-1 text-[var(--app-muted)] hover:bg-[var(--app-panel-soft)] disabled:opacity-50"><X size={20} /></button>
        </header>
        <div className="space-y-4 overflow-y-auto px-5 py-5 sm:px-6">
          <p id="library-cover-description" className="text-sm leading-6 text-[var(--app-muted)]">设置媒体库入口的封面，建议使用 16:9 横向图片。留空保存后，恢复自动选取库内图片。</p>
          <div>
            <label htmlFor="library-cover-url" className="mb-2 block text-sm font-medium">封面图片 URL</label>
            <input
              ref={inputRef}
              id="library-cover-url"
              type="text"
              inputMode="url"
              autoComplete="off"
              spellCheck={false}
              value={coverURL}
              onChange={event => setCoverURL(event.target.value)}
              disabled={saving}
              placeholder="https://example.com/cover.jpg"
              aria-invalid={Boolean(error)}
              aria-describedby={error ? 'library-cover-error' : undefined}
              className="w-full rounded-xl border border-[var(--app-border)] bg-[var(--app-panel-soft)] px-3.5 py-3 text-sm outline-none transition focus:border-primary-400 focus:ring-2 focus:ring-primary-400/20 disabled:opacity-60"
            />
          </div>
          {error && <p id="library-cover-error" role="alert" className="text-sm text-red-500">{error}</p>}
        </div>
        <footer className="flex justify-end gap-2 border-t border-[var(--app-border)] px-5 py-4 sm:px-6">
          <button type="button" onClick={close} disabled={saving} className="rounded-xl border border-[var(--app-border)] px-4 py-2.5 text-sm font-semibold text-[var(--app-subtle)] hover:bg-[var(--app-panel-soft)] disabled:opacity-50">取消</button>
          <button type="submit" disabled={saving} className="inline-flex items-center gap-2 rounded-xl bg-brand-500 px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-brand-600 disabled:opacity-50">
            {saving ? <Loader2 size={16} className="animate-spin" /> : <Save size={16} />}{saving ? '保存中…' : '保存封面'}
          </button>
        </footer>
      </form>
    </div>,
    document.body,
  )
}
