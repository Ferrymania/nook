import { Database, FileText, FolderInput, ListVideo, Pencil, Search, Sparkles, Trash2 } from 'lucide-react'

// 卡片菜单与详情页共用名称、图标和顺序。
export const MEDIA_MANAGEMENT_ACTIONS = [
  { key: 'scrape', label: '智能刮削', pending: '正在刮削…', icon: Sparkles },
  { key: 'match', label: '手动匹配', icon: Search },
  { key: 'metadata', label: '编辑元数据', icon: Pencil },
  { key: 'probe', label: '探测媒体轨', pending: '正在探测…', icon: Database },
  { key: 'nfo', label: '导出 NFO', pending: '正在导出…', icon: FileText },
  { key: 'organize', label: '整理入库', pending: '正在整理…', icon: FolderInput },
  { key: 'segments', label: '查询片头片尾', icon: ListVideo },
  { key: 'delete', label: '删除', pending: '正在删除…', icon: Trash2, danger: true },
] as const

export type MediaManagementActionKey = typeof MEDIA_MANAGEMENT_ACTIONS[number]['key']
