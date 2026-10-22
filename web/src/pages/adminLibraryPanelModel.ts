import type { Library, LibraryRoot } from '../types'
import type { LibraryRootInput, LibraryScrapeSettingsView } from '../api/library'

export type RootDraft = LibraryRootInput

export const emptyRootDraft = (): RootDraft => ({ name: '', path: '', enabled: true })

export const libraryTypeLabels: Record<string, string> = { movie: '电影', tv: '电视剧', variety: '综艺', anime: '动漫', music: '音乐' }

export function duplicateLibrary(libraries: Library[], name: string, type: string) {
  return libraries.find(library => library.name.trim().toLowerCase() === name.trim().toLowerCase() && library.type === type)
}

export function rootDraftError(roots: RootDraft[], existing: LibraryRoot[] = []) {
  const paths = roots.map(root => root.path.trim().replace(/[/\\]+$/, '') || '/')
  if (!roots.length || roots.some(root => !root.path.trim())) return '请填写每个目录的路径'
  if (new Set(paths).size !== paths.length) return '新增目录中存在重复路径'
  if (paths.some(path => existing.some(root => (root.path.trim().replace(/[/\\]+$/, '') || '/') === path))) return '该媒体库已经包含此目录'
  return ''
}

export function scrapeSettingsError(data: LibraryScrapeSettingsView) {
  if (!data.settings.providers.some(id => id !== 'fanart')) return '请至少选择一个元数据来源'
  const { delay_min_ms: min, delay_max_ms: max } = data.settings
  if (!Number.isInteger(min) || !Number.isInteger(max) || min < 0 || max > 300000 || max < min) return '刮削间隔须为 0–300000 毫秒，最大间隔不能小于最小间隔'
  return ''
}

export const rootDraftKey = (libraryID: string, rootID: string) => `${libraryID}:${rootID}`

export function displayLibraryRootPath(path: string) {
  if (!path.toLowerCase().startsWith('cloud://')) return path
  return decodePercentEscapes(path)
}

export function displayLibraryRootName(name: string | undefined, path: string) {
  const value = name?.trim() || '默认路径'
  if (!path.toLowerCase().startsWith('cloud://') && !/%[0-9a-f]{2}/i.test(value)) return value
  return decodePercentEscapes(value)
}

function decodePercentEscapes(value: string) {
  return value.replace(/%[0-9a-f]{2}/gi, (token) => {
    try {
      return decodeURIComponent(token)
    } catch {
      return token
    }
  })
}

export function createRootPayload(roots: RootDraft[]) {
  return roots
    .map((root, index) => ({
      ...root,
      path: root.path.trim(),
      name: root.name?.trim(),
      sort_order: index,
    }))
    .filter((root) => root.path)
}

export function fallbackLibraryRoot(library: Library): LibraryRoot {
  return {
    id: '',
    library_id: library.id,
    path: library.path,
    enabled: library.enabled,
    name: '',
    sort_order: 0,
    created_at: library.created_at,
    updated_at: library.updated_at,
  }
}

export function apiErrorMessage(err: unknown, fallback: string) {
  return (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? fallback
}
