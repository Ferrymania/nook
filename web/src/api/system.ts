import { api } from './client'

export interface ReleaseCheckStatus {
  current_version: string
  latest_version?: string
  status: 'update_available' | 'up_to_date' | 'ahead' | 'unknown' | 'unavailable'
  message: string
  release_url: string
  release_notes?: string
  published_at?: string
  checked_at: string
}

// systemAPI wraps the read-only metadata and release checks shown in About.
export const systemAPI = {
  release: (refresh = false, signal?: AbortSignal) =>
    api.get<ReleaseCheckStatus>('/admin/system/release', {
      params: refresh ? { refresh: '1' } : undefined,
      signal,
    }).then((r) => r.data),

  info: () =>
    api
      .get<{
        name: string
        version: string
        go: string
        os: string
        arch: string
        data_dir: string
        cache_dir: string
        direct_play_only?: boolean
      }>('/system/info')
      .then((r) => r.data),
}
