import type { StrmAccountConfigPreview } from '../types/strm'

export type OpenListAuthMode = 'token' | 'password'

export interface OpenListAuthFields {
  mode: OpenListAuthMode
  server: string
  username: string
  password: string
  token: string
  hasPassword: boolean
  hasToken: boolean
}

export function defaultOpenListAuthMode(preview?: StrmAccountConfigPreview): OpenListAuthMode {
  return preview?.has_token || !preview?.has_password ? 'token' : 'password'
}

export function openListAuthError(fields: OpenListAuthFields): string | undefined {
  if (!fields.server.trim()) return '请填写服务器地址'
  try {
    const address = fields.server.trim()
    const url = new URL(address)
    if (!/^https?:\/\//i.test(address) || !url.hostname) throw new Error('invalid server URL')
  } catch {
    return '请填写包含 http:// 或 https:// 的完整服务器地址'
  }
  if (fields.mode === 'token') {
    if (!fields.token.trim() && !fields.hasToken) return '请填写 Token'
  } else {
    if (!fields.username.trim()) return '请填写用户名'
    if (!fields.password && !fields.hasPassword) return '请填写密码'
  }
}

export function openListAuthConfig(fields: OpenListAuthFields): Record<string, string> {
  if (fields.mode === 'token') {
    return {
      server: fields.server.trim(),
      username: '',
      password: '',
      ...(fields.token.trim() ? { token: fields.token.trim() } : {}),
    }
  }
  return {
    server: fields.server.trim(),
    token: '',
    username: fields.username.trim(),
    ...(fields.password ? { password: fields.password } : {}),
  }
}
