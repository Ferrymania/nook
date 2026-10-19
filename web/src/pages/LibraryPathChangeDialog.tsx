import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { ArrowDown, CheckCircle2, FolderSync, Loader2, X } from 'lucide-react'

import { libraryAPI, type LibraryPathPreview, type LibraryRootInput } from '../api/library'
import type { LibraryRoot } from '../types'
import { apiErrorMessage } from './adminLibraryPanelModel'

export function LibraryPathChangeDialog({ libraryID, root, draft, onClose, onSave }: {
  libraryID: string
  root: LibraryRoot
  draft: LibraryRootInput
  onClose: () => void
  onSave: (preview: LibraryPathPreview) => Promise<void>
}) {
  const [mode, setMode] = useState<'migrate' | 'replace'>('migrate')
  const [preview, setPreview] = useState<LibraryPathPreview | null>(null)
  const [busy, setBusy] = useState<'checking' | 'saving' | null>(null)
  const [error, setError] = useState('')
  const [acknowledged, setAcknowledged] = useState(false)
  const busyRef = useRef(false)
  const dialogRef = useRef<HTMLDivElement>(null)
  const firstOptionRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const previousFocus = document.activeElement as HTMLElement | null
    const overflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    firstOptionRef.current?.focus()
    return () => { document.body.style.overflow = overflow; previousFocus?.focus() }
  }, [])

  const close = () => { if (!busyRef.current) onClose() }
  const check = async () => {
    if (busyRef.current) return
    busyRef.current = true
    setBusy('checking')
    setError('')
    setPreview(null)
    try {
      setPreview(await libraryAPI.previewRootPath(libraryID, root.id, { ...draft, change_mode: mode }))
    } catch (err: unknown) {
      setError(apiErrorMessage(err, '检查目录失败，请重试'))
    } finally {
      busyRef.current = false
      setBusy(null)
    }
  }
  const save = async () => {
    if (!preview?.can_apply || busyRef.current || (mode === 'replace' && !acknowledged)) return
    busyRef.current = true
    setBusy('saving')
    setError('')
    try {
      await onSave(preview)
    } catch (err: unknown) {
      setError(apiErrorMessage(err, '保存失败，请重新检查后重试'))
      setPreview(null)
    } finally {
      busyRef.current = false
      setBusy(null)
    }
  }

  return createPortal(
    <div className="fixed inset-0 z-[110] flex items-center justify-center bg-black/35 p-3 backdrop-blur-sm sm:p-6" onClick={event => { event.stopPropagation(); close() }}>
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="path-change-title"
        aria-describedby="path-change-description"
        aria-busy={busy !== null}
        className="flex max-h-[94dvh] w-full max-w-2xl flex-col overflow-hidden rounded-3xl border border-[var(--app-border)] bg-[var(--app-panel)] text-[var(--app-text)] shadow-2xl"
        onClick={event => event.stopPropagation()}
        onKeyDown={event => {
          if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close() }
          if (event.key !== 'Tab') return
          const targets = Array.from(dialogRef.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled)') ?? [])
          const first = targets[0], last = targets[targets.length - 1]
          if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
          if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
          if (!targets.length) event.preventDefault()
        }}
      >
        <header className="flex items-center justify-between gap-4 border-b border-[var(--app-border)] px-5 py-4 sm:px-6">
          <div className="flex items-center gap-3">
            <span className="rounded-xl bg-[var(--app-brand-soft)] p-2.5 text-[var(--app-brand-text)]"><FolderSync size={21} /></span>
            <h2 id="path-change-title" className="font-display text-xl font-semibold">更改媒体库路径</h2>
          </div>
          <button type="button" aria-label="关闭路径更改" disabled={busy !== null} onClick={close} className="rounded-lg p-1 text-[var(--app-muted)] hover:bg-[var(--app-panel-soft)] disabled:opacity-50"><X size={20} /></button>
        </header>
        <div className="space-y-4 overflow-y-auto px-5 py-4 sm:px-6">
          <p id="path-change-description" className="text-sm leading-6 text-[var(--app-muted)]">仅更新 Nook 的媒体记录，不移动、复制或删除实际文件。请提前准备好新目录或 Docker 挂载。</p>
          <div className="grid items-center gap-3 rounded-2xl border border-[var(--app-border)] bg-[var(--app-panel-soft)] p-4 text-sm sm:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]">
            <div><p className="mb-1 text-xs text-[var(--app-muted)]">原目录</p><p className="break-all font-mono">{root.path}</p></div>
            <ArrowDown size={16} className="text-[var(--app-muted)] sm:-rotate-90" aria-hidden="true" />
            <div><p className="mb-1 text-xs text-[var(--app-muted)]">新目录</p><p className="break-all font-mono">{preview?.new_path ?? draft.path}</p></div>
          </div>
          <fieldset disabled={busy !== null} className="space-y-2">
            <legend className="mb-2 text-sm font-semibold">这次更改是为了什么？</legend>
            {([
              { value: 'migrate', title: '迁移原有内容', description: '同一批文件换了位置。保留媒体资料、收藏和观看进度。' },
              { value: 'replace', title: '改用新目录', description: '换成另一批内容。清除该来源的旧索引、收藏和进度，再扫描新目录。' },
            ] as const).map(option => (
              <label key={option.value} className={`flex cursor-pointer items-start gap-3 rounded-xl border p-4 transition ${mode === option.value ? 'border-brand-400 bg-[var(--app-brand-soft)]' : 'border-[var(--app-border)] hover:bg-[var(--app-panel-soft)]'}`}>
                <input
                  ref={option.value === 'migrate' ? firstOptionRef : undefined}
                  type="radio" name="library-path-mode" value={option.value} checked={mode === option.value}
                  className="mt-1 h-4 w-4 shrink-0 accent-brand-500"
                  onChange={() => { setMode(option.value); setPreview(null); setError(''); setAcknowledged(false) }}
                />
                <span><span className="block text-sm font-semibold">{option.title}</span><span className="mt-1 block text-xs leading-5 text-[var(--app-muted)]">{option.description}</span></span>
              </label>
            ))}
          </fieldset>
          {mode === 'migrate' && <p className="text-xs leading-5 text-[var(--app-muted)]">按相对路径、文件大小核对；STRM 核对播放地址。发现缺失或冲突时不会保存，其他来源目录不受影响。</p>}
          {preview && (
            <div aria-live="polite" className="space-y-3">
              <div className={`rounded-xl border p-4 text-sm ${preview.can_apply ? 'border-emerald-500/30 bg-emerald-500/5' : 'border-amber-500/30 bg-amber-500/5'}`}>
                <p className="flex items-center gap-2 font-semibold">{preview.can_apply && <CheckCircle2 size={17} className="text-emerald-500" />}{preview.can_apply ? '检查完成，可确认保存' : '检查未通过，请先处理以下问题'}</p>
                <p className="mt-2 leading-6 text-[var(--app-muted)]">
                  {mode === 'migrate' ? `原有 ${preview.total} 条 · 匹配 ${preview.matched} 条 · 缺失 ${preview.missing} 条 · 冲突 ${preview.conflicts} 项` : `将清除该来源的 ${preview.total} 条旧媒体索引及其关联记录。实际文件会保留。`}
                </p>
              </div>
              {preview.issues.length > 0 && (
                <ul className="max-h-44 space-y-2 overflow-y-auto rounded-xl border border-[var(--app-border)] p-3 text-xs">
                  {preview.issues.map((issue, index) => <li key={index} className="break-all"><p className="font-mono">{issue.path}</p><p className="mt-1 text-[var(--app-muted)]">{issue.reason}</p></li>)}
                  {preview.missing + preview.conflicts > preview.issues.length && <li className="text-[var(--app-muted)]">仅展示前 {preview.issues.length} 项，请处理后重新检查。</li>}
                </ul>
              )}
              {mode === 'replace' && preview.can_apply && (
                <label className="flex items-start gap-2 text-sm leading-6">
                  <input type="checkbox" className="mt-1 h-4 w-4 shrink-0 accent-brand-500" checked={acknowledged} disabled={busy !== null} onChange={event => setAcknowledged(event.target.checked)} />
                  我确认清除该来源的旧索引、收藏和观看进度，保留磁盘上的文件。
                </label>
              )}
            </div>
          )}
          {error && <p role="alert" className="text-sm text-red-500">{error}</p>}
        </div>
        <footer className="flex flex-wrap justify-end gap-2 border-t border-[var(--app-border)] px-5 py-4 sm:px-6">
          <button type="button" onClick={close} disabled={busy !== null} className="btn-outline disabled:opacity-50">取消</button>
          <button type="button" onClick={check} disabled={busy !== null} className="btn-outline disabled:opacity-50">
            {busy === 'checking' && <Loader2 size={16} className="animate-spin" />}{busy === 'checking' ? '正在检查…' : preview ? '重新检查' : '检查新目录'}
          </button>
          <button type="button" onClick={save} disabled={busy !== null || !preview?.can_apply || (mode === 'replace' && !acknowledged)} className="btn-primary disabled:opacity-40">
            {busy === 'saving' && <Loader2 size={16} className="animate-spin" />}{busy === 'saving' ? '保存中…' : '确认保存'}
          </button>
        </footer>
      </div>
    </div>, document.body,
  )
}
