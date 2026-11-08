import toast from 'react-hot-toast'

import { mediaAPI, type RemoteDeleteItem } from '../api/library'
import { confirmAction, confirmActionResult } from '../components/confirmAction'
import type { Media } from '../types'

type DeleteChoice =
  | { mode: 'local'; deleteFiles: boolean }
  | { mode: 'remote'; approved: Record<string, string> }

export async function confirmMediaDelete(items: Media[], title: string, label: string): Promise<DeleteChoice | null> {
  const result = await confirmActionResult({
    title,
    message: `确定从媒体库删除「${label}」？\n默认仅移除索引记录，磁盘文件会保留。`,
    confirmText: '删除',
    checkboxLabel: '同时删除本地文件（含同名 NFO）',
    alternateText: items.some(canPreviewRemoteDelete) ? '删除 115 原片…' : undefined,
  })
  if (result.confirmed) return { mode: 'local', deleteFiles: result.checked }
  const approved = result.alternate ? await confirmRemoteMediaDelete(items) : null
  return approved ? { mode: 'remote', approved } : null
}

export function canPreviewRemoteDelete(media: Media): boolean {
  if (!media.path?.toLowerCase().endsWith('.strm')) return false
  if (!media.strm_url) return true
  try {
    const path = new URL(media.strm_url, window.location.origin).pathname.toLowerCase()
    return path.startsWith('/api/strm/play/cloud115/video.') || path === '/api/cloud/play/cloud115'
  } catch {
    return false
  }
}

export async function confirmRemoteMediaDelete(items: Media[]): Promise<Record<string, string> | null> {
  const loading = toast.loading('正在核对 115 原片…')
  let preview: RemoteDeleteItem[]
  try {
    preview = await mediaAPI.previewRemoteDelete(items.map((item) => item.id))
  } finally {
    toast.dismiss(loading)
  }
  const approved = Object.fromEntries(preview.flatMap((item) =>
    item.status === 'ready' && item.fingerprint ? [[item.id, item.fingerprint]] : [],
  ))
  const count = Object.keys(approved).length
  const confirmed = await confirmAction({
    title: count ? '删除 115 原片' : '没有可删除的 115 原片',
    message: count
      ? `${count} 个 115 视频将移入回收站，随后清理对应的本地 STRM、未被其他版本共用的同名 NFO 和媒体条目。${
        preview.length > count ? `其余 ${preview.length - count} 个条目将保留。` : ''
      }`
      : '所有条目都将保留。',
    confirmText: count ? '移入 115 回收站' : '关闭',
    danger: count > 0,
    details: renderRemoteDeleteList(preview),
  })
  return confirmed && count ? approved : null
}

export async function deleteConfirmedRemoteMedia(approved: Record<string, string>): Promise<boolean> {
  const loading = toast.loading('正在删除 115 原片并核对结果…')
  let results: RemoteDeleteItem[]
  try {
    results = await mediaAPI.deleteRemote(approved)
  } finally {
    toast.dismiss(loading)
  }
  const remaining = results.filter((item) => item.status !== 'deleted')
  const deleted = results.length - remaining.length
  if (remaining.length) {
    await confirmAction({
      title: '115 删除结果',
      message: `已完成 ${deleted} 项，另有 ${remaining.length} 项需要查看。`,
      confirmText: '关闭',
      danger: false,
      details: renderRemoteDeleteList(remaining),
    })
  } else {
    toast.success(`已删除 ${deleted} 个 115 原片及对应本地条目`)
  }
  return deleted === results.length
}

function renderRemoteDeleteList(items: RemoteDeleteItem[]) {
  return (
    <ul className="mt-3 max-h-52 divide-y divide-sand-200 overflow-y-auto rounded-lg border border-sand-200 text-sm">
      {items.map((item) => (
        <li key={item.id} className="space-y-1 break-words px-3 py-2">
          <p className="font-medium text-ink-600">{item.title || item.id}</p>
          {item.account && <p>{item.account} · {item.remote_path}</p>}
          {item.message && <p className="text-ink-50">{item.message}</p>}
        </li>
      ))}
    </ul>
  )
}
