import { adultSettingsGroup } from './settingsGroupAccess'
import { apiConfigsSettingsGroup } from './settingsGroupAPIConfigs'
import { deviceNotifySettingsGroup } from './settingsGroupDeviceNotify'
import { generalSettingsGroup } from './settingsGroupGeneral'
import { recognitionWordsSettingsGroup } from './settingsGroupRecognitionWords'
import type { SettingGroup } from './settingsGroupTypes'

export type { SettingGroup } from './settingsGroupTypes'

export const databaseSettingsGroup: SettingGroup = {
  key: 'database',
  label: '数据库',
  description: '配置 PostgreSQL 连接并查看数据库运行状态',
  items: [],
}

export const aboutSettingsGroup: SettingGroup = {
  key: 'about',
  label: '关于',
  description: '项目信息、版本信息与使用指南',
  items: [],
}

export const GROUPS: SettingGroup[] = [
  generalSettingsGroup,
  deviceNotifySettingsGroup,
  databaseSettingsGroup,
  apiConfigsSettingsGroup,
  recognitionWordsSettingsGroup,
  adultSettingsGroup,
  aboutSettingsGroup,
]

export const ALL_KEYS = new Set([
  ...GROUPS.flatMap((group) => group.items.map((item) => item.key)),
  // Keep the legacy value for compatibility with the single client-decoding switch.
  'transcode.enabled',
])
