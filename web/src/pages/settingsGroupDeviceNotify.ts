import type { SettingGroup } from './settingsGroupTypes'

// 设备管控与通知设置。
export const deviceNotifySettingsGroup: SettingGroup = {
  key: 'device-notify',
  label: '设备与通知',
  items: [
    {
      key: 'telegram.enabled',
      label: 'Telegram 通知与同步控制',
      type: 'toggle',
      defaultValue: 'false',
    },
    {
      key: 'telegram.bot_token',
      label: 'Bot Token',
      type: 'text',
      placeholder: '123456:AA...',
    },
    {
      key: 'telegram.admin_chat_id',
      label: '管理员 Chat ID',
      type: 'text',
      placeholder: '例如 123456789',
      hint: '私聊发送 /start 获取 ID；群组仅接收通知。',
    },
    {
      key: 'device.antishare_enabled',
      label: '启用防共享',
      type: 'toggle',
      defaultValue: 'false',
      hint: '超出登录设备数或并发播放上限会禁用账号。',
    },
    {
      key: 'device.max_concurrent_play',
      label: '单账号并发播放设备上限',
      type: 'number',
      defaultValue: '3',
    },
    {
      key: 'device.max_logged_clients',
      label: '单账号登录设备上限',
      type: 'number',
      defaultValue: '3',
    },
    {
      key: 'device.play_window_seconds',
      label: '并发判定窗口（秒）',
      type: 'number',
      defaultValue: '90',
    },
    {
      key: 'device.client_active_days',
      label: '登录设备统计窗口（天）',
      type: 'number',
      defaultValue: '30',
    },
  ],
}
