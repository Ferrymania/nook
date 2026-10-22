import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Loader2, Save, Settings2, X } from 'lucide-react'
import toast from 'react-hot-toast'

import { libraryAPI, type LibraryScrapeSettingsView } from '../api/library'
import { notifyEpisodeArtworkPreference } from '../hooks/useEpisodeArtworkPreference'
import type { Library } from '../types'

import { LibraryScrapeSettingsFields } from './LibraryScrapeSettingsFields'

export function LibraryScrapeSettingsDialog({ library, onClose, onSaved }: {
  library: Library
  onClose: () => void
  onSaved: (autoProbe: boolean) => void
}) {
  const [data, setData] = useState<LibraryScrapeSettingsView | null>(null)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const originalEpisodeImages = useRef<boolean | null>(null)
  const formRef = useRef<HTMLFormElement>(null)
  const closeRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    let active = true
    libraryAPI.scrapeSettings(library.id)
      .then(value => { if (active) { originalEpisodeImages.current = value.episode_images; setData(value) } })
      .catch(() => { if (active) setError('加载刮削设置失败，请关闭后重试') })
    return () => { active = false }
  }, [library.id])

  useEffect(() => {
    const previousFocus = document.activeElement as HTMLElement | null
    closeRef.current?.focus()
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => { document.body.style.overflow = previousOverflow; previousFocus?.focus() }
  }, [])

  const save = async () => {
    if (!data || saving) return
    if (!data.settings.providers.some(id => id !== 'fanart')) { toast.error('请至少选择一个元数据来源'); return }
    if (data.settings.delay_min_ms > data.settings.delay_max_ms) { toast.error('最大间隔不能小于最小间隔'); return }
    setSaving(true)
    try {
      const saved = await libraryAPI.saveScrapeSettings(library.id, {
        settings: data.settings,
        auto_probe: data.auto_probe,
        ...(data.episode_images !== originalEpisodeImages.current ? { episode_images: data.episode_images } : {}),
      })
      notifyEpisodeArtworkPreference(saved.episode_images)
      onSaved(saved.auto_probe)
      toast.success('本库刮削设置已保存')
      onClose()
    } catch (err: unknown) {
      toast.error((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '保存刮削设置失败')
    } finally { setSaving(false) }
  }

  return createPortal(
    <div className="fixed inset-0 z-110 flex items-center justify-center bg-black/35 p-3 backdrop-blur-xs sm:p-6" onClick={() => { if (!saving) onClose() }}>
      <form
        ref={formRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="library-scrape-settings-title"
        className="flex max-h-[94dvh] w-full max-w-[650px] flex-col overflow-hidden rounded-3xl border border-(--app-border) bg-(--app-panel) text-(--app-text) shadow-2xl"
        onClick={event => event.stopPropagation()}
        onSubmit={event => { event.preventDefault(); void save() }}
        onKeyDown={event => {
          if (event.key === 'Escape' && !saving) { event.preventDefault(); onClose() }
          if (event.key !== 'Tab') return
          const targets = Array.from(formRef.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled), a[href]') ?? [])
          const first = targets[0], last = targets[targets.length - 1]
          if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
          if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
        }}
      >
        <header className="flex shrink-0 items-start justify-between gap-4 border-b border-(--app-border) px-5 py-5 sm:px-7">
          <div className="flex items-start gap-3">
            <span className="rounded-xl bg-(--app-brand-soft) p-2.5 text-(--app-brand-text)"><Settings2 size={20} /></span>
            <div><h2 id="library-scrape-settings-title" className="font-display text-xl font-semibold">刮削设置</h2><p className="mt-1 text-xs text-(--app-muted)"><span className="font-medium text-(--app-subtle)">{library.name}</span> · 库级设置仅对当前媒体库生效</p></div>
          </div>
          <button ref={closeRef} type="button" aria-label="关闭" onClick={onClose} disabled={saving} className="rounded-lg p-1 text-(--app-muted) hover:bg-(--app-panel-soft)"><X size={20} /></button>
        </header>
        <div className="min-h-0 space-y-5 overflow-y-auto px-5 py-5 sm:px-7">
          {error && <p role="alert" className="text-sm text-red-500">{error}</p>}
          {!data && !error && <div className="flex justify-center py-12"><Loader2 className="animate-spin text-(--app-muted)" /></div>}
          {data && <>
            <LibraryScrapeSettingsFields data={data} onChange={setData} showEpisodeImages onConfigure={onClose} />
          </>}
        </div>
        <footer className="flex shrink-0 items-center justify-between gap-3 border-t border-(--app-border) bg-(--app-panel-soft) px-5 py-4 sm:px-7">
          <span className="text-[10px] text-(--app-muted)">保存后用于后续任务</span><div className="flex gap-2"><button type="button" className="btn-outline px-4! py-2! text-xs" onClick={onClose} disabled={saving}>取消</button><button type="submit" className="btn-primary px-4! py-2! text-xs" disabled={!data || saving}>{saving ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}保存设置</button></div>
        </footer>
      </form>
    </div>, document.body,
  )
}
