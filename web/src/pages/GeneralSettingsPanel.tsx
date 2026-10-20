import { useId } from 'react'
import { Database, LockKeyhole } from 'lucide-react'

import { FFToolsPanel } from '../components/FFToolsPanel'
import { SettingsSection, SettingsSwitch } from '../components/SettingsSection'
import { PlaybackSettingsPanel } from './PlaybackSettingsPanel'
import { SettingRow } from './SettingsRow'
import { generalSettingsGroup } from './settingsGroupGeneral'
import './GeneralSettingsPanel.css'

const concurrency = generalSettingsGroup.items.find((item) => item.key === 'ffprobe.max_concurrent')!
const cacheSettings = generalSettingsGroup.items.filter((item) => item.key.startsWith('cache.'))
const certificatePaths = generalSettingsGroup.items.filter((item) => ['https.cert_path', 'https.key_path'].includes(item.key))
const certificateContents = generalSettingsGroup.items.filter((item) => ['https.cert', 'https.key'].includes(item.key))

type Props = {
  values: Record<string, string>
  savedValues: Record<string, string>
  dirty: Set<string>
  saving: boolean
  statusRevision: number
  onChange: (key: string, value: string) => void
  onBusyChange: (busy: boolean) => void
}

export function GeneralSettingsPanel(props: Props) {
  const { values, saving, onChange } = props
  return (
    <>
      <PlaybackSettingsPanel values={values} saving={saving} onChange={onChange} />
      <FFToolsPanel {...props}>
        <SettingRow def={concurrency} value={values[concurrency.key] ?? concurrency.defaultValue ?? ''}
          onChange={(value) => onChange(concurrency.key, value)} disabled={saving}
          showKey={false} showHint={false} layout="stacked" />
      </FFToolsPanel>
      <SettingsSection title="缓存管理" description="控制内存与图片缓存的占用" icon={Database}>
        <div className="settings-fields">
          {cacheSettings.map((setting) => (
            <SettingRow key={setting.key} def={{ ...setting, label: setting.label.replace(/ \((MB|小时)\)$/, '') }}
              value={values[setting.key] ?? setting.defaultValue ?? ''} onChange={(value) => onChange(setting.key, value)}
              showKey={false} showHint={false} layout="stacked" disabled={saving}
              unit={setting.key.endsWith('_hours') ? '小时' : 'MB'} />
          ))}
        </div>
        <details className="settings-disclosure mt-4">
          <summary>使用建议与清理规则</summary>
          <div className="space-y-3 pt-3">
            {cacheSettings.map((setting) => <p key={setting.key} className="settings-help-text !mt-0"><span className="font-medium">{setting.label}：</span>{setting.hint}</p>)}
          </div>
        </details>
      </SettingsSection>
      <HTTPSSettingsPanel values={values} saving={saving} onChange={onChange} />
    </>
  )
}

function HTTPSSettingsPanel({ values, saving, onChange }: Pick<Props, 'values' | 'saving' | 'onChange'>) {
  const descriptionID = useId()
  const enabled = ['true', '1', 'on'].includes(values['https.enabled'] ?? '')
  const hasPaths = certificatePaths.some((setting) => values[setting.key]?.trim())
  const hasContents = certificateContents.some((setting) => values[setting.key]?.trim())
  return (
    <SettingsSection title="HTTPS" description="为访问连接启用加密" icon={LockKeyhole}
      actions={<SettingsSwitch label="启用 HTTPS" checked={enabled} disabled={saving} describedBy={enabled ? descriptionID : undefined} onChange={(checked) => onChange('https.enabled', String(checked))} />}>
      {enabled && (
        <div className="settings-https-fields">
          <p id={descriptionID} className="settings-help-text !mt-0">填写证书与私钥，文件路径或 PEM 内容任选一种；同时填写时优先使用对应文件。保存后切换为 HTTPS 访问，可能需要重新登录。</p>
          <details className="settings-disclosure" open={hasPaths || !hasContents || undefined}>
            <summary>证书文件路径</summary>
            <div className="settings-fields mt-3">
              {certificatePaths.map((setting) => <SettingRow key={setting.key} def={setting} value={values[setting.key] ?? ''}
                onChange={(value) => onChange(setting.key, value)} showKey={false} showHint={false} layout="stacked" disabled={saving} />)}
            </div>
          </details>
          <details className="settings-disclosure" open={hasContents || undefined}>
            <summary>粘贴 PEM 内容</summary>
            <div className="settings-fields mt-3">
              {certificateContents.map((setting) => <SettingRow key={setting.key} def={setting} value={values[setting.key] ?? ''}
                onChange={(value) => onChange(setting.key, value)} showKey={false} showHint={false} layout="stacked" disabled={saving} />)}
            </div>
          </details>
        </div>
      )}
    </SettingsSection>
  )
}
