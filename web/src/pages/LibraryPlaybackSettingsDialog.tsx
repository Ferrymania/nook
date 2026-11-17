import { useEffect, useId, useRef, useState, type FormEvent, type Ref } from 'react'
import { createPortal } from 'react-dom'
import { Loader2, MonitorPlay, Save, X } from 'lucide-react'

import type { Library } from '../types'
import { apiErrorMessage } from './adminLibraryPanelModel'
import { playbackSettingsError, type LibraryPlaybackSettings } from '../utils/libraryPlaybackSettings'

function PlaybackNumberField({ label, value, onChange, min, max, unit, help, inputRef }: {
  label: string
  value: string
  onChange: (value: string) => void
  min: number
  max: number
  unit: string
  help: string
  inputRef?: Ref<HTMLInputElement>
}) {
  const id = useId()
  return <div>
    <label htmlFor={id} className="mb-2 block text-sm font-medium">{label}</label>
    <div className="flex max-w-40 items-center gap-2">
      <input ref={inputRef} id={id} type="number" required min={min} max={max} step={1} value={value}
        aria-describedby={`${id}-help`} className="input-base min-w-0 w-full"
        onInvalid={event => event.currentTarget.setCustomValidity(`请输入 ${min}–${max} 的整数${unit === '%' ? '百分比' : '秒数'}`)}
        onChange={event => { event.currentTarget.setCustomValidity(''); onChange(event.target.value) }} />
      <span className="shrink-0 text-sm text-(--app-muted)">{unit}</span>
    </div>
    <p id={`${id}-help`} className="mt-2 text-xs leading-6 text-(--app-muted)">{help}</p>
  </div>
}

export function LibraryPlaybackSettingsFields({ percent, minimum, duration, onPercentChange, onMinimumChange, onDurationChange, inputRef }: {
  percent: string
  minimum: string
  duration: string
  onPercentChange: (value: string) => void
  onMinimumChange: (value: string) => void
  onDurationChange: (value: string) => void
  inputRef?: Ref<HTMLInputElement>
}) {
  return <div className="space-y-5">
    <p className="text-xs leading-6 text-(--app-muted)">仅对本库生效，适用于网页和 Emby 兼容客户端。</p>
    <PlaybackNumberField label="最小续播百分比" value={minimum} onChange={onMinimumChange} min={0} max={99} unit="%" inputRef={inputRef}
      help="播放进度低于此值时不续播，也不进入“继续观看”。默认 5%；设为 0 关闭比例限制。" />
    <PlaybackNumberField label="播放完成阈值" value={percent} onChange={onPercentChange} min={1} max={100} unit="%"
      help="播放达到此百分比时标记为已观看。默认 90%，须大于最小续播百分比。" />
    <PlaybackNumberField label="最短可续播视频时长" value={duration} onChange={onDurationChange} min={0} max={2147483647} unit="秒"
      help="按视频总时长判断。短于此时长的视频不续播，仍可标记已观看。默认 180 秒；设为 0 关闭时长限制。" />
  </div>
}

export function LibraryPlaybackSettingsDialog({ library, onClose, onSave }: {
  library: Library
  onClose: () => void
  onSave: (library: Library, settings: LibraryPlaybackSettings) => Promise<void>
}) {
  const [percent, setPercent] = useState(String(library.played_threshold_pct ?? 90))
  const [minimum, setMinimum] = useState(String(library.min_resume_pct ?? 5))
  const [duration, setDuration] = useState(String(library.min_resume_duration_seconds ?? 180))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const savingRef = useRef(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const formRef = useRef<HTMLFormElement>(null)
  const titleID = useId()

  useEffect(() => {
    const previousFocus = document.activeElement as HTMLElement | null
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    inputRef.current?.focus()
    return () => { document.body.style.overflow = previousOverflow; previousFocus?.focus() }
  }, [])

  const close = () => { if (!savingRef.current) onClose() }
  const save = async (event: FormEvent) => {
    event.preventDefault()
    if (savingRef.current) return
    const validation = playbackSettingsError(percent, minimum, duration)
    if (validation) { setError(validation); return }
    savingRef.current = true
    setSaving(true)
    setError('')
    try { await onSave(library, { played_threshold_pct: Number(percent), min_resume_pct: Number(minimum), min_resume_duration_seconds: Number(duration) }) }
    catch (err: unknown) { setError(apiErrorMessage(err, '保存播放设置失败，请重试')) }
    finally { savingRef.current = false; setSaving(false) }
  }

  return createPortal(
    <div className="fixed inset-0 z-110 flex items-center justify-center bg-black/35 p-3 backdrop-blur-xs sm:p-6" onClick={event => { event.stopPropagation(); close() }}>
      <form ref={formRef} role="dialog" aria-modal="true" aria-labelledby={titleID}
        className="flex max-h-[94dvh] w-full max-w-lg flex-col overflow-hidden rounded-3xl border border-(--app-border) bg-(--app-panel) text-(--app-text) shadow-2xl"
        onClick={event => event.stopPropagation()} onSubmit={save}
        onKeyDown={event => {
          if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close() }
          if (event.key !== 'Tab') return
          const targets = Array.from(formRef.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled)') ?? [])
          const first = targets[0], last = targets[targets.length - 1]
          if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
          if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
          if (!targets.length) event.preventDefault()
        }}>
        <header className="flex items-start justify-between gap-4 border-b border-(--app-border) px-5 py-5 sm:px-6">
          <div className="flex items-start gap-3">
            <span className="rounded-xl bg-(--app-brand-soft) p-2.5 text-(--app-brand-text)"><MonitorPlay size={20} /></span>
            <div><h2 id={titleID} className="font-display text-xl font-semibold">播放设置</h2><p className="mt-1 text-sm text-(--app-muted)">{library.name}</p></div>
          </div>
          <button type="button" aria-label="关闭播放设置" onClick={close} disabled={saving} className="rounded-lg p-1 text-(--app-muted) hover:bg-(--app-panel-soft)"><X size={20} /></button>
        </header>
        <fieldset disabled={saving} className="space-y-4 overflow-y-auto px-5 py-5 sm:px-6">
          <LibraryPlaybackSettingsFields percent={percent} minimum={minimum} duration={duration} onPercentChange={setPercent} onMinimumChange={setMinimum} onDurationChange={setDuration} inputRef={inputRef} />
          {error && <p role="alert" className="text-sm text-red-500">{error}</p>}
        </fieldset>
        <footer className="flex items-center justify-between gap-3 border-t border-(--app-border) px-5 py-4 sm:px-6">
          <p className="text-xs text-(--app-muted)">保存后用于本库播放</p>
          <div className="flex gap-2">
            <button type="button" onClick={close} disabled={saving} className="btn-outline px-4! py-2! text-xs">取消</button>
            <button type="submit" disabled={saving} className="btn-primary px-4! py-2! text-xs">{saving ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}保存设置</button>
          </div>
        </footer>
      </form>
    </div>, document.body,
  )
}
