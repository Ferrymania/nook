import { api } from './client'

// 管理员 Telegram 通知通道测试。
export const telegramAPI = {
  test: () => api.post<{ success: boolean; error?: string }>('/admin/telegram/test').then((r) => r.data),
}
