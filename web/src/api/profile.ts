import { api } from './client'
import type { LibraryTagSet, User } from '../types'

export const profileAPI = {
  get: () => api.get<User>('/me').then((r) => r.data),

  update: (patch: {
    username?: string
    nickname?: string
    email?: string
    password?: string
  }) =>
    api.patch<User>('/me', patch).then((r) => r.data),

  avatarImage: (signal: AbortSignal) =>
    api.get<Blob>('/me/avatar', { signal, responseType: 'blob' }).then((r) => r.data),

  uploadAvatar: (file: File) =>
    api.post<User>('/me/avatar', file, { headers: { 'Content-Type': 'application/octet-stream' } }).then((r) => r.data),

  removeAvatar: () => api.delete<User>('/me/avatar').then((r) => r.data),

  getPinnedLibraries: () =>
    api.get<{ library_ids: string[] }>('/me/pinned-libraries').then((r) => r.data.library_ids ?? []),

  setPinnedLibraries: (libraryIds: string[]) =>
    api.put<{ library_ids: string[] }>('/me/pinned-libraries', { library_ids: libraryIds }).then((r) => r.data.library_ids ?? []),

  getLibraryTags: () =>
    api.get<{ tags: LibraryTagSet[] | null }>('/me/library-tags').then((r) => r.data.tags ?? []),

  setLibraryTags: (tags: LibraryTagSet[]) =>
    api.put<{ tags: LibraryTagSet[] | null }>('/me/library-tags', { tags }).then((r) => r.data.tags ?? []),

  adminUpdateRole: (id: string, role: 'admin' | 'user') =>
    api.patch<User>(`/admin/users/${id}/role`, { role }).then((r) => r.data),
}
