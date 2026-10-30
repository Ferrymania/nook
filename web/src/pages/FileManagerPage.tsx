import { useCallback, useEffect, useRef, useState } from 'react'
import { Loader2, Plus } from 'lucide-react'

import { filesAPI, type FileEntry, type FileListing } from '../api/files'
import { FileBrowserRoots } from './FileBrowserRoots'
import { FileEntriesTable } from './FileEntriesTable'
import { FileManagerToolbar } from './FileManagerToolbar'
import { FileOperationDialog, type FileOperation } from './FileOperationDialog'
import type { FileEntryAction } from './FileEntryMenu'
import { useFileOperations } from './useFileOperations'
import './file-manager.css'

export function FileManagerPage() {
  const [path, setPath] = useState('')
  const [data, setData] = useState<FileListing | null>(null)
  const [loadedPath, setLoadedPath] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [recursive, setRecursive] = useState(false)
  const [action, setAction] = useState<FileOperation | null>(null)
  const requestID = useRef(0)
  const currentDir = data?.path || path

  const refresh = useCallback(() => {
    const request = ++requestID.current
    setLoading(true)
    setError('')
    filesAPI.list(path, recursive ? 5000 : 1000, recursive)
      .then(result => { if (request === requestID.current) { setData(result); setLoadedPath(path) } })
      .catch((err: unknown) => {
        if (request !== requestID.current) return
        setError((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '加载失败')
      })
      .finally(() => { if (request === requestID.current) setLoading(false) })
  }, [path, recursive])

  useEffect(() => {
    refresh()
    const requests = requestID
    return () => { requests.current++ }
  }, [refresh])

  const operations = useFileOperations({ currentDir, path, refresh })
  const navigate = (next: string) => { if (next !== path) setLoading(true); setAction(null); setPath(next) }
  const closeDialog = () => { setAction(null); operations.clearSelection() }
  const operate = (entry: FileEntry, next: FileEntryAction) => {
    if (next === 'delete') { void operations.deleteSelected(entry); return }
    operations.choose(entry)
    setAction(next)
  }
  const entries = data?.entries ?? []
  const visible = data && loadedPath === path

  return <div className="file-manager">
    <header className="file-page-heading">
      <h1>文件管理</h1>
      {visible && data?.path && <button type="button" className="file-button file-button-primary" disabled={loading || Boolean(error) || Boolean(operations.busy)}
        onClick={() => { operations.setFolderName(''); operations.setError(''); setAction('mkdir') }}><Plus size={17} />新建目录</button>}
    </header>

    <section className="file-browser" aria-label="本地文件浏览">
      <FileManagerToolbar currentPath={path} parent={data?.parent} roots={data?.roots} recursive={recursive}
        loading={loading || Boolean(operations.busy)} onNavigate={navigate} onRefresh={refresh} onRecursiveChange={setRecursive} />

      {error ? <div className="file-list-state" role="alert"><span>{error}</span><button type="button" className="file-button" onClick={refresh}>重试</button></div>
        : !visible ? <div className="file-list-state" role="status"><Loader2 size={18} className="animate-spin" />加载中…</div>
          : !data.path ? <FileBrowserRoots roots={data.roots ?? []} onOpen={navigate} />
            : <div aria-busy={loading}>
              {entries.length > 0 ? <FileEntriesTable basePath={data.path} entries={entries} recursive={recursive}
                disabled={loading || Boolean(operations.busy)} selectedPath={operations.selected?.path}
                onEnter={entry => navigate(entry.path)} onAction={operate} />
                : <p className="file-list-state">此目录为空</p>}
              <footer className="file-list-footer" role="status">
                {loading ? '正在刷新…' : `${entries.length} 项`}
                {!loading && entries.length >= (recursive ? 5000 : 1000) && <span>已达显示上限，请进入子目录查看</span>}
              </footer>
            </div>}
    </section>
    {action && <FileOperationDialog action={action} currentDir={currentDir} operations={operations} onClose={closeDialog} />}
  </div>
}
