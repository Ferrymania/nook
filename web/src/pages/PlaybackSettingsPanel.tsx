import { useId } from 'react'
import { MonitorPlay } from 'lucide-react'

import { SettingRow } from './SettingsRow'
import { generalSettingsGroup } from './settingsGroupGeneral'

const directOnlySetting = generalSettingsGroup.items.find((item) => item.key === 'playback.direct_only')!
const transcodeSettings = generalSettingsGroup.items.filter((item) => item.key.startsWith('transcode.'))
const parameterKeys = [
  'transcode.hw_enabled',
  'transcode.hw_accel',
  'transcode.max_jobs',
  'transcode.realtime',
  'transcode.threads',
  'transcode.idle_timeout_seconds',
]

type Props = {
  values: Record<string, string>
  onChange: (key: string, value: string) => void
}

export function PlaybackSettingsPanel({ values, onChange }: Props) {
  const modeDescriptionID = useId()
  const valueOf = (key: string) => values[key] ?? generalSettingsGroup.items.find((item) => item.key === key)?.defaultValue ?? ''
  const isOn = (key: string) => ['true', '1', 'on'].includes(valueOf(key))
  // Older installations may have disabled transcoding with the separate switch.
  // Treat either legacy restriction as client decoding, then keep both keys in
  // sync whenever the user changes the single visible playback switch.
  const transcodeEnabled = ['true', '1', 'on'].includes(values['transcode.enabled'] ?? 'true')
  const directOnly = isOn(directOnlySetting.key) || !transcodeEnabled
  const setDirectOnly = (enabled: boolean) => {
    onChange(directOnlySetting.key, enabled ? 'true' : 'false')
    onChange('transcode.enabled', enabled ? 'false' : 'true')
  }

  return (
    <section aria-label="播放与转码" className="space-y-5">
      <div className={`rounded-2xl border p-4 sm:p-5 ${directOnly ? 'border-brand-500/30 bg-brand-500/5' : 'border-[var(--app-border)] bg-[var(--app-panel)]'}`}>
        <div className="flex items-start justify-between gap-4">
          <div className="flex min-w-0 items-start gap-3">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-brand-500/10 text-brand-500">
              <MonitorPlay size={21} />
            </div>
            <div>
              <h2 className="mb-1 text-base font-bold text-[var(--app-text)]">{directOnlySetting.label}</h2>
              <p className="text-sm text-[var(--app-muted)]">{directOnly ? '由客户端解码，服务器不转码' : '优先直放，允许服务器按需转码'}</p>
            </div>
          </div>
          <div className="flex shrink-0 flex-col items-center gap-1.5">
            <button
              type="button"
              role="switch"
              aria-label="客户端直连解码"
              aria-checked={directOnly}
              aria-describedby={modeDescriptionID}
              onClick={() => setDirectOnly(!directOnly)}
              className={`relative h-7 w-12 rounded-full transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500 ${directOnly ? 'bg-brand-500' : 'bg-slate-400'}`}
            >
              <span className={`absolute left-1 top-1 h-5 w-5 rounded-full bg-white shadow-sm transition-transform ${directOnly ? 'translate-x-5' : ''}`} />
            </button>
            <span className="text-xs font-medium text-[var(--app-muted)]">{directOnly ? '已开启' : '已关闭'}</span>
          </div>
        </div>
        <p id={modeDescriptionID} className="mt-3 text-xs leading-relaxed text-[var(--app-muted)]">{directOnlySetting.hint} 更改后需点击页面底部“保存”生效。</p>
      </div>

      <div className="space-y-4 rounded-2xl border border-[var(--app-border)] p-4 sm:p-5">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-sm font-bold text-[var(--app-text)]">转码设置</h3>
          <span className={`rounded-full px-2.5 py-1 text-xs font-medium ${directOnly ? 'bg-[var(--app-hover)] text-[var(--app-muted)]' : 'bg-brand-500/10 text-brand-500'}`}>
            {directOnly ? '已停用' : '按需使用'}
          </span>
        </div>
        <div className="space-y-4">
          {directOnly && <p className="text-xs text-[var(--app-muted)]">关闭“客户端直连解码”后可调整以下参数；原配置会保留。</p>}
          {parameterKeys.map((key) => {
            const setting = transcodeSettings.find((item) => item.key === key)!
            return (
              <SettingRow
                key={key}
                def={setting}
                value={valueOf(key)}
                onChange={(value) => onChange(key, value)}
                disabled={directOnly || (key === 'transcode.hw_accel' && !isOn('transcode.hw_enabled'))}
                showKey={false}
              />
            )
          })}
        </div>
      </div>
    </section>
  )
}
