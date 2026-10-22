import { ArrowDown, ArrowUp, Info, ShieldAlert } from 'lucide-react'
import type { LibraryScrapeSettingsView } from '../api/library'

const API_SETTINGS_URL = '/settings?group=api-configs'
const PROBE_WARNING = '开启后会在刮削时读取视频技术信息。连续探测大量网盘媒体可能触发限流或风控，建议按需开启。默认关闭；单独手动探测不受影响。'

export function LibraryScrapeSettingsFields({ data, onChange, showEpisodeImages = false, onConfigure }: {
  data: LibraryScrapeSettingsView
  onChange: (value: LibraryScrapeSettingsView) => void
  showEpisodeImages?: boolean
  onConfigure?: () => void
}) {
  const updateSettings = (patch: Partial<LibraryScrapeSettingsView['settings']>) => onChange({ ...data, settings: { ...data.settings, ...patch } })
  const sourceOrder = [...data.settings.providers, ...data.providers.map(p => p.id).filter(id => !data.settings.providers.includes(id))]
  const moveSource = (id: string, direction: number) => {
    const selected = [...data.settings.providers]
    const index = selected.indexOf(id), other = index + direction
    if (index < 0 || other < 0 || other >= selected.length) return
    ;[selected[index], selected[other]] = [selected[other], selected[index]]
    updateSettings({ providers: selected })
  }
  const hasAvailableMetadata = data.providers.some(p => p.id !== 'fanart' && p.available && data.settings.providers.includes(p.id))
  return <div className="space-y-5">
            <div className="divide-y divide-(--app-border) overflow-hidden rounded-2xl border border-(--app-border)">
              <ScrapeSwitch label="扫描后自动刮削" description="扫描发现新增或变化的媒体后，自动补充影片资料。" checked={data.settings.auto_on_scan} onChange={value => updateSettings({ auto_on_scan: value })} />
              <ScrapeSwitch label="刮削时自动探测媒体信息" description="连续探测大量网盘媒体可能触发风控，建议按需开启。" title={PROBE_WARNING} warning checked={data.auto_probe} onChange={value => onChange({ ...data, auto_probe: value })} />
            </div>
            <section>
              <h3 className="mb-2 text-sm font-semibold">片头／片尾时间点</h3>
              <div className="overflow-hidden rounded-2xl border border-(--app-border)">
                <ScrapeSwitch label="后台预取时间点" description="仅对当前媒体库生效。每 6 小时向 TheIntroDB 查询片头、片尾起止时间并保存到数据库。" checked={data.settings.segment_prewarm_enabled} onChange={value => updateSettings({ segment_prewarm_enabled: value })} />
              </div>
              <p className="mt-2 text-[11px] leading-relaxed text-(--app-muted)">默认关闭。关闭后，播放器仍可在播放时按需获取时间点。查询不读取视频文件，无需 API Key。</p>
            </section>
            {showEpisodeImages && <section>
              <h3 className="mb-2 text-sm font-semibold">全局图片偏好</h3>
              <div className="overflow-hidden rounded-2xl border border-(--app-border)">
                <ScrapeSwitch label="刮削时获取每集图片" description="对所有媒体库生效。关闭后仍获取每集简介、评分和时长，仅跳过单集剧照；已有图片不受影响。" checked={data.episode_images} onChange={value => onChange({ ...data, episode_images: value })} />
              </div>
            </section>}
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
                    {!provider.available && <a href={API_SETTINGS_URL} className="text-[10px] text-(--app-brand-text)" onClick={onConfigure}>去配置</a>}
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
            <p className="flex items-center gap-2 rounded-xl bg-(--app-panel-soft) px-3 py-2.5 text-[11px] text-(--app-muted)"><Info size={13} className="shrink-0" /><span>API 凭据统一在 <a href={API_SETTINGS_URL} onClick={onConfigure} className="text-(--app-brand-text)">系统设置 → 外部 API</a> 中管理。</span></p>
  </div>
}

function ScrapeSwitch({ label, description, checked, onChange, title, warning }: {
  label: string; description: string; checked: boolean; onChange: (checked: boolean) => void; title?: string; warning?: boolean
}) {
  return <div className="flex items-center justify-between gap-4 px-4 py-3" title={title}>
    <div><p className="text-xs font-semibold">{label}</p><p className={`mt-1 flex items-center gap-1 text-[11px] leading-relaxed ${warning ? 'text-amber-600' : 'text-(--app-muted)'}`}>{warning && <ShieldAlert size={12} className="shrink-0" />}{description}</p></div>
    <button type="button" role="switch" aria-checked={checked} aria-label={label} onClick={() => onChange(!checked)} className={`flex h-5 w-9 shrink-0 items-center rounded-full p-0.5 transition-colors ${checked ? 'bg-primary-500' : 'bg-sand-300/70'}`}><span className={`h-4 w-4 rounded-full bg-white shadow-xs transition-transform ${checked ? 'translate-x-4' : ''}`} /></button>
  </div>
}
