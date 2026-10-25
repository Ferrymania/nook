import { api } from './client'
import type { AccessLog, Setting, User } from '../types'

export interface DatabaseStatus {
  type: 'postgres'
  dsn?: string
  open_conns: number
  in_use: number
  idle: number
  max_open_conns: number
  table_counts?: Record<string, number>
}

export interface PostgresTestResult {
  success: boolean
  latency_ms?: number
  version?: string
  message?: string
  error?: string
}


export interface DatabaseConnectionPayload {
  type?: string
  dsn?: string
  host?: string
  port?: number
  user?: string
  password?: string
  dbname?: string
  sslmode?: string
}

export interface UserListResponse {
  users: User[]
  max_users: number
  current_users: number
}

export interface UserLimitResponse {
  max_users: number
  current_users: number
}

export interface CurrentPlaybackSession {
  id: string
  user_id: string
  username: string
  device_name: string
  client: string
  media_id: string
  title: string
  series_name?: string
  season_num: number
  episode_num: number
  position_ms: number
  duration_ms: number
  is_paused: boolean
  last_report_at: string
}

export const adminAPI = {
  playbackSessions: (signal?: AbortSignal) =>
    api.get<{ items: CurrentPlaybackSession[] }>('/admin/playback-sessions', { signal, timeout: 8000 })
      .then((r) => r.data.items),
  listUsers: () => api.get<UserListResponse>('/admin/users').then((r) => r.data),

  getUserLimit: () => api.get<UserLimitResponse>('/admin/users/limit').then((r) => r.data),

  updateUserLimit: (maxUsers: number) =>
    api.put<UserLimitResponse>('/admin/users/limit', { max_users: maxUsers }).then((r) => r.data),

  createUser: (payload: { username: string; password: string }) =>
    api.post<User>('/admin/users', payload).then((r) => r.data),

  updateUser: (id: string, payload: { username: string }) =>
    api.patch<User>(`/admin/users/${id}`, payload).then((r) => r.data),

  resetUserPassword: (id: string, password: string) =>
    api.patch(`/admin/users/${id}/password`, { password }).then((r) => r.data),

  setUserStatus: (id: string, isActive: boolean) =>
    api.patch<User>(`/admin/users/${id}/status`, { is_active: isActive }).then((r) => r.data),

  updateUserLibraries: (id: string, allowedLibraryIDs: string[] | null) =>
    api.patch<User>(`/admin/users/${id}/libraries`, { allowed_library_ids: allowedLibraryIDs }).then((r) => r.data),

  deleteUser: (id: string) => api.delete(`/admin/users/${id}`).then((r) => r.data),

  listSettings: () => api.get<Setting[]>('/admin/settings').then((r) => r.data),

  updateSetting: (key: string, value: string) =>
    api.put('/admin/settings', { key, value }).then((r) => r.data),

  recentLogs: () => api.get<AccessLog[]>('/admin/logs').then((r) => r.data),

	  getDatabaseStatus: () =>
	    api.get<DatabaseStatus>('/admin/database/status').then((r) => r.data),

	  testDatabaseConnection: (payload: DatabaseConnectionPayload) =>
	    api.post<PostgresTestResult>('/admin/database/test', payload).then((r) => r.data),



	  saveDatabaseConfig: (payload: DatabaseConnectionPayload) =>
	    api.post<{ message: string; type: string }>('/admin/database/save-config', payload).then((r) => r.data),
	}
