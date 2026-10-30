import { File, FileImage, FileText, FileVideo, Folder } from 'lucide-react'

import type { FileEntry } from '../api/files'
import { FileEntryMenu, type FileEntryAction } from './FileEntryMenu'

type FileEntriesTableProps = {
  basePath: string
  entries: FileEntry[]
  recursive: boolean
  disabled: boolean
  selectedPath?: string
  onEnter: (entry: FileEntry) => void
  onAction: (entry: FileEntry, action: FileEntryAction) => void
}

export function FileEntriesTable({ basePath, entries, recursive, disabled, selectedPath, onEnter, onAction }: FileEntriesTableProps) {
  return <table className="file-entries" aria-label="文件列表">
    <thead><tr><th scope="col">名称</th><th scope="col" className="file-size">大小</th><th scope="col" className="file-modified">修改时间</th><th scope="col"><span className="sr-only">操作</span></th></tr></thead>
    <tbody>
      {entries.map(entry => {
        const relative = recursive ? entry.path.slice(basePath.replace(/[\\/]$/, '').length + 1) : entry.name
        const Icon = fileIcon(entry)
        const label = <span className="file-entry-label"><span>{entry.name}</span>
          {recursive && relative !== entry.name && <small>{relative.slice(0, -(entry.name.length + 1))}</small>}
        </span>
        return <tr key={entry.path} className={selectedPath === entry.path ? 'file-entry-selected' : undefined}>
          <td className="file-name">
            {entry.is_dir ? <button type="button" disabled={disabled} onClick={() => onEnter(entry)} title={entry.path}>
              <Icon size={19} className="file-folder-icon" />{label}
            </button> : <div title={entry.path}><Icon size={19} />{label}</div>}
          </td>
          <td className="file-size">{entry.is_dir ? <span className="file-folder-size">—</span> : fmtBytes(entry.size)}</td>
          <td className="file-modified"><time dateTime={new Date(entry.modified * 1000).toISOString()}>{formatDate(entry.modified)}</time></td>
          <td className="file-row-actions"><FileEntryMenu entry={entry} disabled={disabled} onAction={onAction} /></td>
        </tr>
      })}
    </tbody>
  </table>
}

function fileIcon(entry: FileEntry) {
  if (entry.is_dir) return Folder
  const ext = (entry.ext || entry.name.split('.').pop() || '').toLowerCase()
  if (['strm', 'mkv', 'mp4', 'avi', 'mov', 'ts', 'm2ts', 'webm'].includes(ext)) return FileVideo
  if (['jpg', 'jpeg', 'png', 'webp', 'gif'].includes(ext)) return FileImage
  if (['nfo', 'txt', 'srt', 'ass', 'vtt', 'json'].includes(ext)) return FileText
  return File
}

function formatDate(seconds: number) {
  return new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(new Date(seconds * 1000))
}

function fmtBytes(n: number): string {
  if (!n) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(n) / Math.log(1024)), units.length - 1)
  return `${(n / 1024 ** index).toFixed(1)} ${units[index]}`
}
