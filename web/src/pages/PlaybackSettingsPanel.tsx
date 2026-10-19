import { useId } from 'react'
import { LockKeyhole, MonitorPlay } from 'lucide-react'

import { SettingRow } from './SettingsRow'
import { generalSettingsGroup } from './settingsGroupGeneral'

const directOnlySetting = generalSettingsGroup.items.find((item) => item.key === 'playback.direct_only')!
const transcodeSettings = generalSettingsGroup.items.filter((item) => item.key.startsWith('transcode.'))
const enabledSetting = transcodeSettings.find((item) => item.key === 'transcode.enabled')!
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
  const directOnly = isOn(directOnlySetting.key)
  const transcodeEnabled = isOn(enabledSetting.key)
  const parametersDisabled = directOnly || !transcodeEnabled

  return (
    <section aria-label="播放与转码" className="space-y-5">
      <div className={`rounded-2xl border p-4 sm:p-5 ${directOnly ? 'border-brand-500/30 bg-brand-500/5' : 'border-[var(--app-border)] bg-[var(--app-panel)]'}`}>
        <div className="flex items-start justify-between gap-4">
          <div className="flex min-w-0 items-start gap-3">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-brand-500/10 text-brand-500">
              <MonitorPlay size={21} />
            </div>
            <div>
              <div className="mb-1 flex flex-wrap items-center gap-2">
                <h2 className="text-base font-bold text-[var(--app-text)]">{directOnlySetting.label}</h2>
                <span className="inline-flex items-center gap-1 rounded-full border border-brand-500/25 px-2 py-0.5 text-[11px] font-semibold text-brand-500">
                  <LockKeyhole size={11} />全局限制
                </span>
              </div>
              <p className="text-sm font-medium text-[var(--app-text)]">禁止服务端转码，交给客户端解码</p>
            </div>
          </div>
          <div className="flex shrink-0 flex-col items-center gap-1.5">
            <button
              type="button"
              role="switch"
              aria-label="仅直放模式（禁止服务端转码）"
              aria-checked={directOnly}
              aria-describedby={modeDescriptionID}
              onClick={() => onChange(directOnlySetting.key, directOnly ? 'false' : 'true')}
              className={`relative h-7 w-12 rounded-full transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500 ${directOnly ? 'bg-brand-500' : 'bg-slate-400'}`}
            >
              <span className={`absolute left-1 top-1 h-5 w-5 rounded-full bg-white shadow-sm transition-transform ${directOnly ? 'translate-x-5' : ''}`} />
            </button>
            <span className="text-xs font-medium text-[var(--app-muted)]">{directOnly ? '已开启' : '已关闭'}</span>
          </div>
        </div>
        <p id={modeDescriptionID} className="mt-3 text-xs leading-relaxed text-[var(--app-muted)]">{directOnlySetting.hint} 更改后需点击页面底部“保存”生效。</p>
        {directOnly && (
          <div className="mt-4 flex items-start gap-2 rounded-xl border border-brand-500/20 bg-[var(--app-panel)] p-3 text-sm text-[var(--app-text)]">
            <LockKeyhole size={16} className="mt-0.5 shrink-0 text-brand-500" />
            <p><strong>下方所有转码设置暂不生效。</strong>原配置会保留，关闭仅直放模式后可继续调整。</p>
          </div>
        )}
      </div>

      <div className="space-y-4 rounded-2xl border border-[var(--app-border)] p-4 sm:p-5">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-sm font-bold text-[var(--app-text)]">服务端转码</h3>
          <span className={`rounded-full px-2.5 py-1 text-xs font-medium ${directOnly ? 'bg-[var(--app-hover)] text-[var(--app-muted)]' : transcodeEnabled ? 'bg-brand-500/10 text-brand-500' : 'bg-[var(--app-hover)] text-[var(--app-muted)]'}`}>
            {directOnly ? '已被仅直放模式禁用' : transcodeEnabled ? '按需启用' : '已关闭'}
          </span>
        </div>
        <SettingRow
          def={enabledSetting}
          value={valueOf(enabledSetting.key)}
          onChange={(value) => onChange(enabledSetting.key, value)}
          disabled={directOnly}
          showKey={false}
        />
        <div className="space-y-4 border-t border-[var(--app-border)] pt-4">
          {parametersDisabled && !directOnly && (
            <p className="text-xs text-[var(--app-muted)]">转码已关闭，开启上方“启用服务端转码”后才能调整以下参数。</p>
          )}
          {parameterKeys.map((key) => {
            const setting = transcodeSettings.find((item) => item.key === key)!
            return (
              <SettingRow
                key={key}
                def={setting}
                value={valueOf(key)}
                onChange={(value) => onChange(key, value)}
                disabled={parametersDisabled || (key === 'transcode.hw_accel' && !isOn('transcode.hw_enabled'))}
                showKey={false}
              />
            )
          })}
        </div>
      </div>
    </section>
  )
}
