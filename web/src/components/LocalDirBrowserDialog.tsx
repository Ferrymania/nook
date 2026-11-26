import { useEffect, useId, useState, type FormEvent } from 'react'
import { ChevronRight, Folder, FolderPlus, Loader2, X } from 'lucide-react'
import toast from 'react-hot-toast'

import { strmAPI } from '../api/strm'

function apiErrorMessage(err: unknown): string {
  if (typeof err === 'object' && err !== null && 'response' in err) {
    const res = (err as { response?: { data?: { error?: string; message?: string } } }).response
    if (res?.data?.error) return res.data.error
    if (res?.data?.message) return res.data.message
  }
  if (err instanceof Error) return err.message
  return '请求失败'
}

export function LocalDirBrowserDialog({
  initialDir,
  title = '选择本地目录',
  allowCreate = false,
  onSelect,
  onClose,
}: {
  initialDir?: string
  title?: string
  allowCreate?: boolean
  onSelect: (path: string) => void
  onClose: () => void
}) {
  const [list, setList] = useState<{
    roots: boolean
    parent?: string
    current?: string
    children: { name: string; path: string }[]
  } | null>(null)
  const [loading, setLoading] = useState(true)
  const [folderName, setFolderName] = useState<string | null>(null)
  const [folderError, setFolderError] = useState('')
  const [creating, setCreating] = useState(false)
  const titleId = useId()
  const folderInputId = useId()

  const load = async (target: string) => {
    setLoading(true)
    try {
      const data = await strmAPI.listLocalDirs(target)
      setList(data)
    } catch (err) {
      toast.error(apiErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load(initialDir ?? '').catch(() => undefined)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const selectable = list && !list.roots && list.current
  const showingCreateForm = folderName !== null
  const busy = loading || creating

  const createFolder = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const name = folderName?.trim()
    if (!allowCreate || !selectable || !name || busy) return

    setCreating(true)
    setFolderError('')
    try {
      const folder = await strmAPI.createOutputFolder(selectable, name)
      setFolderName(null)
      await load(folder.path)
    } catch (err) {
      setFolderError(apiErrorMessage(err))
    } finally {
      setCreating(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-110 flex items-center justify-center bg-black/35 p-4 backdrop-blur-xs"
      onClick={creating ? undefined : onClose}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className="flex max-h-[80vh] w-full max-w-2xl flex-col overflow-hidden rounded-3xl border border-white/70 bg-white shadow-2xl"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-gray-100 px-6 py-4">
          <h3 id={titleId} className="font-display text-lg font-bold text-ink-600">{title}</h3>
          <button
            type="button"
            onClick={onClose}
            disabled={creating}
            className="rounded-xl p-1.5 text-ink-50 transition hover:bg-gray-100 hover:text-ink-600"
            title="关闭"
          >
            <X size={20} />
          </button>
        </div>
        <div className="flex items-center gap-2 border-b border-gray-100 px-6 py-2.5 text-xs text-sand-500">
          {list?.roots ? (
            <span className="text-ink-50">选择盘符 / 根目录</span>
          ) : (
            <>
              <button
                type="button"
                className="hover:text-brand-500"
                disabled={busy || showingCreateForm}
                onClick={() => load('')}
              >
                盘符 / 根
              </button>
              <ChevronRight size={12} />
              <span className="truncate text-ink-50" title={list?.current}>
                {list?.current}
              </span>
            </>
          )}
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-3 sm:min-h-[180px]">
          {showingCreateForm && (
            <form onSubmit={createFolder} className="mb-3 space-y-2 rounded-xl border border-gray-200 p-3">
              <label htmlFor={folderInputId} className="text-sm font-medium text-ink-600">文件夹名称</label>
              <div className="flex flex-wrap items-center gap-2">
                <input
                  id={folderInputId}
                  className="input-base min-w-32 flex-1"
                  value={folderName}
                  onChange={(event) => {
                    setFolderName(event.target.value)
                    setFolderError('')
                  }}
                  disabled={creating}
                  aria-invalid={Boolean(folderError)}
                  aria-describedby={folderError ? `${folderInputId}-error` : undefined}
                  autoFocus
                  required
                />
                <button type="button" className="btn-outline" disabled={creating} onClick={() => setFolderName(null)}>取消</button>
                <button type="submit" className="neon-button" disabled={busy || !folderName.trim()}>
                  {creating ? '创建中…' : '创建'}
                </button>
              </div>
              {folderError && <p id={`${folderInputId}-error`} role="alert" className="text-sm text-red-600">{folderError}</p>}
            </form>
          )}
          {loading ? (
            <div className="flex justify-center py-10 text-ink-50">
              <Loader2 className="animate-spin" />
            </div>
          ) : list && (
            <div className="space-y-1">
              {(list.parent ?? '') !== '' && (
                <button
                  type="button"
                  disabled={busy || showingCreateForm}
                  className="flex w-full items-center gap-3 rounded-xl px-3 py-2 text-left text-sm text-ink-50 transition hover:bg-gray-50"
                  onClick={() => load(list.parent ?? '')}
                >
                  <FolderPlus size={16} className="text-sand-400" />
                  <span>..（上级目录）</span>
                </button>
              )}
              {list.children.length === 0 && (
                <p className="py-8 text-center text-sm text-sand-500">该目录下没有子目录</p>
              )}
              {list.children.map((entry) => (
                <button
                  key={entry.path}
                  type="button"
                  disabled={busy || showingCreateForm}
                  className="flex w-full items-center gap-3 rounded-xl px-3 py-2 text-left text-sm transition hover:bg-gray-50"
                  onClick={() => load(entry.path)}
                >
                  <Folder size={16} className="text-brand-400" />
                  <span className="flex-1 truncate text-ink-600">{entry.name}</span>
                </button>
              ))}
            </div>
          )}
        </div>
        <div className="flex flex-wrap justify-end gap-2 border-t border-gray-100 px-6 py-3">
          {allowCreate && selectable && !showingCreateForm && (
            <button type="button" className="btn-outline" disabled={busy} onClick={() => {
              setFolderError('')
              setFolderName('')
            }}>
              <FolderPlus size={16} aria-hidden="true" />
              新建文件夹
            </button>
          )}
          <button
            type="button"
            className="neon-button"
            disabled={!selectable || busy || showingCreateForm}
            onClick={() => {
              if (list?.current) onSelect(list.current)
            }}
          >
            选择当前目录
          </button>
        </div>
      </div>
    </div>
  )
}
