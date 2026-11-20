import { apiConfigsSettingsGroup } from './settingsGroupAPIConfigs'
import { deviceNotifySettingsGroup } from './settingsGroupDeviceNotify'
import { generalSettingsGroup } from './settingsGroupGeneral'
import type { SettingGroup } from './settingsGroupTypes'

export type { SettingGroup } from './settingsGroupTypes'

export const aboutSettingsGroup: SettingGroup = {
  key: 'about',
  label: '关于',
  description: '项目信息、版本更新与运行状态',
  items: [],
}

export const GROUPS: SettingGroup[] = [
  generalSettingsGroup,
  deviceNotifySettingsGroup,
  apiConfigsSettingsGroup,
  aboutSettingsGroup,
]

export const ALL_KEYS = new Set([
  ...GROUPS.flatMap((group) => group.items.map((item) => item.key)),
  // Keep the legacy value for compatibility with the single client-decoding switch.
  'transcode.enabled',
])
