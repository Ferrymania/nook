import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Link } from 'react-router-dom'
import { ArrowDown, ArrowUp, Info, Loader2, Save, Settings2, ShieldAlert, X } from 'lucide-react'
import toast from 'react-hot-toast'

import { libraryAPI, type LibraryScrapeSettingsView } from '../api/library'
import { notifyEpisodeArtworkPreference } from '../hooks/useEpisodeArtworkPreference'
import type { Library } from '../types'

const API_SETTINGS_URL = '/settings?group=api-configs'
const PROBE_WARNING = '开启后会在刮削时读取视频技术信息。连续探测大量网盘媒体可能触发限流或风控，建议按需开启。默认关闭；单独手动探测不受影响。'

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
  const updateSettings = (patch: Partial<LibraryScrapeSettingsView['settings']>) => {
    setData(current => current ? { ...current, settings: { ...current.settings, ...patch } } : current)
  }
  const sourceOrder = data ? [...data.settings.providers, ...data.providers.map(p => p.id).filter(id => !data.settings.providers.includes(id))] : []
  const moveSource = (id: string, direction: number) => {
    if (!data) return
    const selected = [...data.settings.providers]
    const index = selected.indexOf(id)
    const other = index + direction
    if (index < 0 || other < 0 || other >= selected.length) return
    ;[selected[index], selected[other]] = [selected[other], selected[index]]
    updateSettings({ providers: selected })
  }
  const hasAvailableMetadata = data?.providers.some(p => p.id !== 'fanart' && p.available && data.settings.providers.includes(p.id))

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
            <div className="divide-y divide-(--app-border) overflow-hidden rounded-2xl border border-(--app-border)">
              <ScrapeSwitch label="扫描后自动刮削" description="扫描发现新增或变化的媒体后，自动补充影片资料。" checked={data.settings.auto_on_scan} onChange={value => updateSettings({ auto_on_scan: value })} />
              <ScrapeSwitch label="刮削时自动探测媒体信息" description="连续探测大量网盘媒体可能触发风控，建议按需开启。" title={PROBE_WARNING} warning checked={data.auto_probe} onChange={value => setData({ ...data, auto_probe: value })} />
            </div>
            <section>
              <h3 className="mb-2 text-sm font-semibold">片头／片尾时间点</h3>
              <div className="overflow-hidden rounded-2xl border border-(--app-border)">
                <ScrapeSwitch label="后台预取时间点" description="仅对当前媒体库生效。每 6 小时向 TheIntroDB 查询片头、片尾起止时间并保存到数据库。" checked={data.settings.segment_prewarm_enabled} onChange={value => updateSettings({ segment_prewarm_enabled: value })} />
              </div>
              <p className="mt-2 text-[11px] leading-relaxed text-(--app-muted)">默认关闭。关闭后，播放器仍可在播放时按需获取时间点。查询不读取视频文件，无需 API Key。</p>
            </section>
            <section>
              <h3 className="mb-2 text-sm font-semibold">全局图片偏好</h3>
              <div className="overflow-hidden rounded-2xl border border-(--app-border)">
                <ScrapeSwitch label="刮削时获取每集图片" description="对所有媒体库生效。关闭后仍获取每集简介、评分和时长，仅跳过单集剧照；已有图片不受影响。" checked={data.episode_images} onChange={value => setData({ ...data, episode_images: value })} />
              </div>
            </section>
            <section>
              <h3 className="mb-2 text-sm font-semibold">刮削源与优先级</h3>
              <div className="divide-y divide-(--app-border) overflow-hidden rounded-2xl border border-(--app-border)">
                {sourceOrder.map(id => {
                  const provider = data.providers.find(p => p.id === id)
                  if (!provider) return null
                  const index = data.settings.providers.indexOf(id)
                  const checked = index >= 0
                  return <div key={id} className="flex items-center gap-2 px-3 py-2">
                    <span className="w-3 text-[11px] text-(--app-muted)">{checked ? index + 1 : '–'}</span>
                    <label className="flex min-w-0 flex-1 items-center gap-2 text-xs">
                      <input type="checkbox" checked={checked} onChange={event => updateSettings({ providers: event.target.checked ? [...data.settings.providers, id] : data.settings.providers.filter(p => p !== id) })} className="accent-primary-500" />
                      <span>{provider.name}</span>
                      {id === 'fanart' && <span className="text-[10px] text-(--app-muted)">图片补充</span>}
                    </label>
                    <span className={`rounded-md px-1.5 py-0.5 text-[10px] ${provider.available ? 'bg-emerald-500/10 text-emerald-600' : 'bg-(--app-panel-soft) text-(--app-muted)'}`}>{provider.status}</span>
                    {!provider.available && <Link to={API_SETTINGS_URL} className="text-[10px] text-(--app-brand-text)" onClick={onClose}>去配置</Link>}
                    <button type="button" aria-label={`上移 ${provider.name}`} disabled={index <= 0} onClick={() => moveSource(id, -1)} className="p-1 text-(--app-muted) disabled:opacity-25"><ArrowUp size={13} /></button>
                    <button type="button" aria-label={`下移 ${provider.name}`} disabled={!checked || index === data.settings.providers.length - 1} onClick={() => moveSource(id, 1)} className="p-1 text-(--app-muted) disabled:opacity-25"><ArrowDown size={13} /></button>
                  </div>
                })}
              </div>
              <p className="mt-2 text-[11px] leading-relaxed text-(--app-muted)">按顺序尝试勾选的来源；不可用来源会跳过。勾选不会开启系统中已关闭的服务。</p>
              {!hasAvailableMetadata && <p role="status" className="mt-1 text-xs text-amber-600">所选元数据来源均不可用，请先到系统设置启用服务或配置 API。</p>}
            </section>
            <div className="grid items-end gap-2 sm:grid-cols-[190px_1fr] sm:gap-4">
              <label className="space-y-1.5 text-xs text-(--app-subtle)">TMDb 元数据语言<select value={data.settings.language} onChange={event => updateSettings({ language: event.target.value })} className="input-base w-full py-2.5! text-xs!"><option value="zh-CN">简体中文（zh-CN）</option><option value="zh-TW">繁体中文（zh-TW）</option><option value="en-US">English（en-US）</option><option value="ja-JP">日本語（ja-JP）</option></select></label>
              <p className="pb-1 text-[11px] leading-relaxed text-(--app-muted)">用于 TMDb 标题、简介和剧集资料；<br className="hidden sm:block" />其他来源使用其提供的语言。</p>
            </div>
            <section><h3 className="mb-2 text-sm font-semibold">刮削任务间隔</h3><div className="grid grid-cols-2 gap-3">
              <label className="space-y-1.5 text-xs text-(--app-subtle)">最小间隔（毫秒）<input type="number" required min={0} max={300000} value={data.settings.delay_min_ms} onChange={event => updateSettings({ delay_min_ms: Number(event.target.value) })} className="input-base w-full py-2.5! text-xs!" /></label>
              <label className="space-y-1.5 text-xs text-(--app-subtle)">最大间隔（毫秒）<input type="number" required min={data.settings.delay_min_ms} max={300000} value={data.settings.delay_max_ms} onChange={event => updateSettings({ delay_max_ms: Number(event.target.value) })} className="input-base w-full py-2.5! text-xs!" /></label>
            </div><p className="mt-2 text-[11px] text-(--app-muted)">两次媒体刮削开始之间随机等待 {data.settings.delay_min_ms}–{data.settings.delay_max_ms} 毫秒，手动与自动刮削共用。</p></section>
            <p className="flex items-center gap-2 rounded-xl bg-(--app-panel-soft) px-3 py-2.5 text-[11px] text-(--app-muted)"><Info size={13} className="shrink-0" /><span>API 凭据统一在 <Link to={API_SETTINGS_URL} onClick={onClose} className="text-(--app-brand-text)">系统设置 → 外部 API</Link> 中管理。</span></p>
          </>}
        </div>
        <footer className="flex shrink-0 items-center justify-between gap-3 border-t border-(--app-border) bg-(--app-panel-soft) px-5 py-4 sm:px-7">
          <span className="text-[10px] text-(--app-muted)">保存后用于后续任务</span><div className="flex gap-2"><button type="button" className="btn-outline px-4! py-2! text-xs" onClick={onClose} disabled={saving}>取消</button><button type="submit" className="btn-primary px-4! py-2! text-xs" disabled={!data || saving}>{saving ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}保存设置</button></div>
        </footer>
      </form>
    </div>, document.body,
  )
}

function ScrapeSwitch({ label, description, checked, onChange, title, warning }: {
  label: string; description: string; checked: boolean; onChange: (checked: boolean) => void; title?: string; warning?: boolean
}) {
  return <div className="flex items-center justify-between gap-4 px-4 py-3" title={title}>
    <div><p className="text-xs font-semibold">{label}</p><p className={`mt-1 flex items-center gap-1 text-[11px] leading-relaxed ${warning ? 'text-amber-600' : 'text-(--app-muted)'}`}>{warning && <ShieldAlert size={12} className="shrink-0" />}{description}</p></div>
    <button type="button" role="switch" aria-checked={checked} aria-label={label} onClick={() => onChange(!checked)} className={`flex h-5 w-9 shrink-0 items-center rounded-full p-0.5 transition-colors ${checked ? 'bg-primary-500' : 'bg-sand-300/70'}`}><span className={`h-4 w-4 rounded-full bg-white shadow-xs transition-transform ${checked ? 'translate-x-4' : ''}`} /></button>
  </div>
}
