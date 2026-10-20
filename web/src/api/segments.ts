import { api } from './client'

export type SegmentTaskStatus = 'running' | 'found' | 'missing' | 'failed'
export type SegmentTaskCounts = Partial<Record<SegmentTaskStatus, number>>
export interface SegmentTask {
  id: string
  media_id: string
  media_title: string
  media_path: string
  library_id: string
  library_name: string
  season_num: number
  episode_num: number
  provider: string
  trigger: string
  status: SegmentTaskStatus
  error: string
  segments: { kind: string; start_ms: number; end_ms: number }[] | null
  created_at: string
  started_at?: string
  finished_at?: string
}
export interface SegmentQueueSnapshot {
  tasks: SegmentTask[]
  counts: SegmentTaskCounts
  total: number
  page: number
  page_size: number
}

export const segmentQueueAPI = {
  list: (status: string, q: string, page: number, signal?: AbortSignal) =>
    api.get<SegmentQueueSnapshot>('/admin/segments/queue', { params: { status, q, page, page_size: 50 }, signal }).then(r => r.data),
  retry: (id: string) => api.post(`/admin/segments/queue/${encodeURIComponent(id)}/retry`).then(r => r.data),
  remove: (id: string) => api.delete(`/admin/segments/queue/${encodeURIComponent(id)}`).then(r => r.data),
  clearFinished: () => api.post<{ deleted: number }>('/admin/segments/queue/clear-finished').then(r => r.data),
}
