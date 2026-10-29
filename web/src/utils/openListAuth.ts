export interface OpenListAuthFields {
  server: string
  token: string
  hasToken: boolean
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
  if (!fields.token.trim() && !fields.hasToken) return '请填写 Token'
}

export function openListAuthConfig(fields: OpenListAuthFields): Record<string, string> {
  return {
    server: fields.server.trim(),
    ...(fields.token.trim() ? { token: fields.token.trim() } : {}),
  }
}
