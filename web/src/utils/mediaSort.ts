export type SortField =
  | 'title'
  | 'release_date'
  | 'year'
  | 'created_at'
  | 'updated_at'
  | 'rating'
  | 'imdb_rating'
  | 'last_played'
  | 'duration'

export type SortOrder = 'asc' | 'desc'

export type SortOption = {
  id: SortField
  label: string
  defaultOrder: SortOrder
}

export const SORT_OPTIONS: SortOption[] = [
  { id: 'title', label: '标题', defaultOrder: 'asc' },
  { id: 'release_date', label: '发行日期', defaultOrder: 'desc' },
  { id: 'year', label: '年份', defaultOrder: 'desc' },
  { id: 'created_at', label: '加入日期', defaultOrder: 'desc' },
  { id: 'updated_at', label: '最后一集添加日期', defaultOrder: 'desc' },
  { id: 'rating', label: '影评人评分', defaultOrder: 'desc' },
  { id: 'imdb_rating', label: 'IMDB评分', defaultOrder: 'desc' },
  { id: 'last_played', label: '播放日期', defaultOrder: 'desc' },
  { id: 'duration', label: '播放时长', defaultOrder: 'desc' },
]

export function getSortOption(field: string | null): SortOption {
  return SORT_OPTIONS.find((opt) => opt.id === field) ?? SORT_OPTIONS[0]
}

export function libraryMediaSortField(field: SortField, isSeriesLibrary: boolean): SortField {
  return field === 'updated_at' && !isSeriesLibrary ? 'created_at' : field
}
