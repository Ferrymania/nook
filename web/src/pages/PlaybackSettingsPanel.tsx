import { useId } from 'react'
import { MonitorPlay } from 'lucide-react'

import { SettingsSection, SettingsSwitch } from '../components/SettingsSection'
import { SettingRow } from './SettingsRow'
import { generalSettingsGroup } from './settingsGroupGeneral'

const directOnlySetting = generalSettingsGroup.items.find((item) => item.key === 'playback.direct_only')!
const parameterKeys = [
  'transcode.hw_enabled', 'transcode.hw_accel', 'transcode.max_jobs',
  'transcode.threads', 'transcode.idle_timeout_seconds', 'transcode.realtime',
]

type Props = {
  values: Record<string, string>
  saving?: boolean
  onChange: (key: string, value: string) => void
}

export function PlaybackSettingsPanel({ values, saving = false, onChange }: Props) {
  const descriptionID = useId()
  const valueOf = (key: string) => values[key] ?? generalSettingsGroup.items.find((item) => item.key === key)?.defaultValue ?? ''
  const isOn = (key: string) => ['true', '1', 'on'].includes(valueOf(key))
  // Preserve the legacy restriction and synchronize both keys with the visible switch.
  const transcodeEnabled = ['true', '1', 'on'].includes(values['transcode.enabled'] ?? 'true')
  const directOnly = isOn(directOnlySetting.key) || !transcodeEnabled
  const setDirectOnly = (enabled: boolean) => {
    onChange(directOnlySetting.key, enabled ? 'true' : 'false')
    onChange('transcode.enabled', enabled ? 'false' : 'true')
  }

  return (
    <SettingsSection title="播放方式" description="选择由客户端解码，或允许服务器按需转码" icon={MonitorPlay}>
      <div className="settings-mode-row">
        <div>
          <p className="font-medium">客户端直连解码</p>
          <p id={descriptionID} className="settings-section-description">
            {directOnly ? '由客户端解码，服务器不转码' : '优先直放，允许服务器按需转码'}
          </p>
        </div>
        <SettingsSwitch label="客户端直连解码" checked={directOnly} disabled={saving} describedBy={descriptionID} onChange={setDirectOnly} />
      </div>
      <details className="settings-disclosure settings-disclosure-divided" open={!directOnly || undefined}>
        <summary>转码参数<span className="settings-state">{directOnly ? '暂不生效' : '按需使用'}</span></summary>
        <div className="settings-fields mt-4">
          {parameterKeys.map((key) => {
            const setting = generalSettingsGroup.items.find((item) => item.key === key)!
            const disabled = saving || directOnly
            if (setting.type === 'toggle') return (
              <div key={key} className={`settings-mode-row ${disabled ? 'opacity-60' : ''}`}>
                <span>{setting.label}</span>
                <SettingsSwitch label={setting.label} checked={isOn(key)} disabled={disabled} onChange={(checked) => onChange(key, String(checked))} />
              </div>
            )
            return (
              <SettingRow key={key} def={setting} value={valueOf(key)} onChange={(value) => onChange(key, value)}
                disabled={disabled || (key === 'transcode.hw_accel' && !isOn('transcode.hw_enabled'))}
                showKey={false} showHint={false} layout="stacked" unit={key === 'transcode.idle_timeout_seconds' ? '秒' : undefined} />
            )
          })}
        </div>
        <p className="settings-help-text">NAS 建议并发 1、软件线程 1–2；硬件编码器需先开启硬件加速。按播放速度转码可降低 CPU 峰值，空闲超时后自动结束转码。</p>
        <p className="settings-help-text">{directOnlySetting.hint}</p>
      </details>
    </SettingsSection>
  )
}
