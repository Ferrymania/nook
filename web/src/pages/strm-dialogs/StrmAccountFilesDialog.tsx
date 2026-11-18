import { useEffect, useState, type FormEvent } from 'react'
import { ChevronRight, File, Folder, FolderPlus, Loader2, RefreshCw, Trash2 } from 'lucide-react'
import toast from 'react-hot-toast'
import { strmAPI, type StrmRemoteEntry } from '../../api/strm'
import { confirmAction } from '../../components/confirmAction'
import type { StrmAccount } from '../../types/strm'
import { DialogShell } from '../StrmDialogs'
import { apiErrorMessage, formatBytes } from '../StrmManagePage'

type Directory = { id: string; name: string }

export function StrmAccountFilesDialog({ account, onClose }: { account: StrmAccount; onClose: () => void }) {
  const [path, setPath] = useState<Directory[]>([{ id: '0', name: '根目录' }])
  const [busy, setBusy] = useState(false)
  const directory = path[path.length - 1]

  return <DialogShell title="文件管理" subtitle={account.name} wide onClose={() => { if (!busy) onClose() }}>
    <nav aria-label="网盘目录" className="mb-4 flex flex-wrap items-center gap-1 text-sm text-sand-500">
      {path.map((dir, index) => <span key={dir.id} className="inline-flex min-w-0 max-w-full items-center gap-1">
        {index > 0 && <ChevronRight size={14} className="shrink-0" />}
        {index === path.length - 1
          ? <span aria-current="page" className="wrap-anywhere font-semibold text-ink-600">{dir.name}</span>
          : <button type="button" disabled={busy} onClick={() => setPath(path.slice(0, index + 1))}
              className="truncate hover:text-brand-500 hover:underline">{dir.name}</button>}
      </span>)}
    </nav>
    <DirectoryContents key={directory.id} account={account} directory={directory} busy={busy} setBusy={setBusy}
      enter={(dir) => setPath([...path, dir])} />
  </DialogShell>
}

function DirectoryContents({ account, directory, enter, busy, setBusy }: {
  account: StrmAccount
  directory: Directory
  enter: (directory: Directory) => void
  busy: boolean
  setBusy: (value: boolean) => void
}) {
  const [page, setPage] = useState(1)
  const [revision, setRevision] = useState(0)
  const [listing, setListing] = useState<{ entries: StrmRemoteEntry[]; has_more: boolean } | null>(null)
  const [error, setError] = useState('')
  const [folderName, setFolderName] = useState<string | null>(null)
  const [pendingDeletes, setPendingDeletes] = useState<string[]>([])
  useEffect(() => {
    const controller = new AbortController()
    strmAPI.listAccountFiles(account.id, directory.id, page, controller.signal)
      .then((result) => {
        if (controller.signal.aborted) return
        if (page > 1 && result.entries.length === 0) setPage(page - 1)
        else setListing(result)
      })
      .catch((err: unknown) => { if (!controller.signal.aborted) setError(apiErrorMessage(err)) })
    return () => controller.abort()
  }, [account.id, directory.id, page, revision])

  const reload = (nextPage = page) => {
    setError('')
    setListing(null)
    setPage(nextPage)
    setRevision((value) => value + 1)
  }

  const create = async (event: FormEvent) => {
    event.preventDefault()
    const name = folderName?.trim()
    if (!name || busy) return
    setBusy(true)
    try {
      await strmAPI.createAccountFolder(account.id, directory.id, name)
      setFolderName(null)
      toast.success(`已创建文件夹「${name}」`)
      reload(1)
    } catch (err) {
      toast.error(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const remove = async (entry: StrmRemoteEntry) => {
    if (busy) return
    setBusy(true)
    try {
      const message = entry.is_dir
        ? `将「${account.name}」中的文件夹「${entry.name}」及其全部内容移入 115 回收站？`
        : `将「${account.name}」中的文件「${entry.name}」移入 115 回收站？`
      if (!await confirmAction({ title: '删除网盘文件', message, confirmText: '移入回收站' })) return
      await strmAPI.deleteAccountFile(account.id, directory.id, entry.id)
      setPendingDeletes((ids) => [...ids, entry.id])
      toast.success('115 已受理删除请求')
      reload()
    } catch (err) {
      toast.error(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return <div className="space-y-3">
    <div className="flex flex-wrap items-center justify-between gap-2">
      <button type="button" className="neon-button" disabled={busy || !listing || folderName !== null}
        onClick={() => setFolderName('')}><FolderPlus size={16} />新建文件夹</button>
      <button type="button" className="neon-button" disabled={busy || (!listing && !error)}
        onClick={() => reload()}><RefreshCw size={15} />刷新</button>
    </div>
    {folderName !== null && <form onSubmit={create} className="flex flex-wrap items-center gap-2">
      <input autoFocus required aria-label="文件夹名称" placeholder="文件夹名称" className="input-base min-w-0 flex-1"
        value={folderName} disabled={busy} onChange={(e) => setFolderName(e.target.value)} />
      <button type="submit" className="neon-button" disabled={busy || !folderName.trim()}>创建</button>
      <button type="button" className="neon-button" disabled={busy} onClick={() => setFolderName(null)}>取消</button>
    </form>}
    {error ? <p role="alert" className="py-6 text-center text-sm text-rose-600">{error}</p>
      : !listing ? <p role="status" className="flex items-center justify-center gap-2 py-8 text-sm text-sand-500">
          <Loader2 size={16} className="animate-spin" />正在读取目录…
        </p>
      : listing.entries.length === 0 ? <p className="py-8 text-center text-sm text-sand-500">此目录为空</p>
      : <ul className="divide-y divide-gray-100">
          {listing.entries.map((entry) => {
            const pending = pendingDeletes.includes(entry.id)
            return <li key={entry.id} className="flex items-center gap-3 py-1">
              {entry.is_dir
                ? <button type="button" disabled={busy || pending} onClick={() => enter(entry)}
                    className="flex min-w-0 flex-1 items-center gap-2 py-2 text-left text-sm hover:text-brand-500">
                    <Folder size={17} className="shrink-0 text-brand-500" /><span className="wrap-anywhere">{entry.name}</span>
                  </button>
                : <span className="flex min-w-0 flex-1 items-center gap-2 py-2 text-sm">
                    <File size={17} className="shrink-0 text-sand-500" /><span className="wrap-anywhere">{entry.name}</span>
                  </span>}
              {!entry.is_dir && <span className="shrink-0 text-xs text-sand-500">{entry.size ? formatBytes(entry.size) : '0 B'}</span>}
              {pending ? <span className="text-xs text-sand-500">删除中</span>
                : <button type="button" aria-label={`删除「${entry.name}」`} title="删除" disabled={busy}
                    className="shrink-0 rounded-lg p-2 text-sand-500 hover:bg-rose-50 hover:text-rose-600"
                    onClick={() => void remove(entry)}><Trash2 size={16} /></button>}
            </li>
          })}
        </ul>}
    {listing && (page > 1 || listing.has_more) && <nav aria-label="目录分页" className="flex items-center justify-end gap-3 text-sm">
      <button type="button" className="neon-button" disabled={busy || page === 1} onClick={() => reload(page - 1)}>上一页</button>
      <span>第 {page} 页</span>
      <button type="button" className="neon-button" disabled={busy || !listing.has_more} onClick={() => reload(page + 1)}>下一页</button>
    </nav>}
  </div>
}
