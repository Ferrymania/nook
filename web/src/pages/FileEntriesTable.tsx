import { FileVideo, Folder, HardDrive } from 'lucide-react'

import type { FileEntry } from '../api/files'

type FileEntriesTableProps = {
  basePath: string
  entries: FileEntry[]
  recursive: boolean
  selectedPath?: string
  onEnter: (entry: FileEntry) => void
  onChoose: (entry: FileEntry) => void
}

export function FileEntriesTable({
  basePath,
  entries,
  recursive,
  selectedPath,
  onEnter,
  onChoose,
}: FileEntriesTableProps) {
  return (
    <div className="glass-panel table-scroll">
      <table className="min-w-[700px] w-full text-left text-sm">
        <thead className="text-xs uppercase tracking-wider text-sand-500">
          <tr>
            <th className="py-2">名称</th>
            <th>大小</th>
            <th>修改时间</th>
            <th className="text-right">选择</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((entry) => (
            <tr
              key={entry.path}
              className={'border-t border-gray-200 transition hover:bg-gray-50 ' + (selectedPath === entry.path ? 'bg-primary-400/5' : '')}
            >
              <td className="py-2 text-ink-600">
                <button
                  className="flex max-w-xl items-center gap-2 text-left"
                  onClick={() => (entry.is_dir ? onEnter(entry) : onChoose(entry))}
                  title={entry.path}
                >
                  {entry.is_dir ? <Folder size={16} className="text-brand-500" /> : <FileVideo size={16} className="text-ink-50" />}
                  <span className="truncate">{entryLabel(entry, basePath, recursive)}</span>
                </button>
              </td>
              <td className="text-ink-100">{entry.is_dir ? '—' : fmtBytes(entry.size)}</td>
              <td className="text-sand-500">{new Date(entry.modified * 1000).toLocaleString()}</td>
              <td className="text-right">
                <button
                  className="rounded-lg border border-gray-200 px-2 py-1 text-xs text-ink-100 hover:border-primary-400/40"
                  onClick={() => onChoose(entry)}
                >
                  <HardDrive size={12} className="mr-1 inline" /> 操作
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function entryLabel(entry: FileEntry, basePath: string, recursive: boolean): string {
  if (!recursive) return entry.name
  return entry.path.replace((basePath || '') + '\\', '').replace((basePath || '') + '/', '')
}

function fmtBytes(n: number): string {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(1)} ${u[i]}`
}
