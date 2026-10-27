import { api } from './client'

export interface DiagnosticsStatus {
  enabled: boolean
  expires_at?: string
  capturing: boolean
}

export type DiagnosticsProfile = 'heap' | 'allocs' | 'goroutine' | 'profile'

const BASE = '/admin/diagnostics/pprof'

export const diagnosticsAPI = {
  status: (signal?: AbortSignal) =>
    api.get<DiagnosticsStatus>(`${BASE}/status`, { signal }).then((r) => r.data),
  enable: () => api.post<DiagnosticsStatus>(`${BASE}/enable`).then((r) => r.data),
  disable: () => api.post<DiagnosticsStatus>(`${BASE}/disable`).then((r) => r.data),
  capture: async (profile: DiagnosticsProfile, signal: AbortSignal) => {
    const result = await api.get<Blob>(`${BASE}/${profile}`, {
      params: profile === 'profile' ? { seconds: 10 } : undefined,
      responseType: 'blob',
      timeout: 60_000,
      signal,
    })
    const disposition = String(result.headers['content-disposition'] || '')
    const filename = /filename="([a-zA-Z0-9_.-]+)"/.exec(disposition)?.[1] || `nook-${profile}.pprof`
    return { blob: result.data, filename }
  },
}

export async function diagnosticsError(error: unknown): Promise<string> {
  let data = (error as { response?: { data?: unknown } })?.response?.data
  if (data instanceof Blob) {
    const text = await data.text()
    try { data = JSON.parse(text) } catch { data = undefined }
  }
  const message = (data as { error?: string } | undefined)?.error
  return message || '运行诊断请求失败，请检查连接后重试'
}
