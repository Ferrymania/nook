import { useEffect, useId, useRef, type FormEvent } from 'react'
import { createPortal } from 'react-dom'
import { Loader2, X } from 'lucide-react'

import type { useFileOperations } from './useFileOperations'

export type FileOperation = 'mkdir' | 'rename' | 'transfer'

export function FileOperationDialog({ action, currentDir, operations, onClose }: {
  action: FileOperation
  currentDir: string
  operations: ReturnType<typeof useFileOperations>
  onClose: () => void
}) {
  const dialog = useRef<HTMLDialogElement>(null)
  const heading = useId()
  const busy = Boolean(operations.busy)
  const titles = { mkdir: '新建目录', rename: '重命名', transfer: '转移文件' }
  const modes = { copy: '复制', move: '移动', hardlink: '创建硬链接', symlink: '创建软链接' }
  const valid = action === 'mkdir' ? operations.folderName.trim()
    : action === 'rename' ? operations.renameTo.trim() : operations.destPath.trim()

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    dialog.current?.showModal()
    dialog.current?.querySelector<HTMLElement>('input, select')?.focus()
    return () => { previous?.focus({ preventScroll: true }) }
  }, [])

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (busy || !valid) return
    const success = await (action === 'mkdir' ? operations.createFolder()
      : action === 'rename' ? operations.renameSelected() : operations.transferSelected())
    if (success) onClose()
    else dialog.current?.querySelector<HTMLElement>('input, select')?.focus()
  }

  return createPortal(
    <dialog ref={dialog} aria-labelledby={heading} className="file-operation-dialog"
      onCancel={event => { event.preventDefault(); if (!busy) onClose() }}>
      <form onSubmit={submit}>
        <header className="file-dialog-heading">
          <h2 id={heading}>{titles[action]}</h2>
          <button type="button" className="file-icon-button" aria-label="关闭" disabled={busy} onClick={onClose}><X size={18} /></button>
        </header>
        <div className="file-dialog-fields">
          <p className="file-dialog-path" title={action === 'mkdir' ? currentDir : operations.selected?.path}>
            {action === 'mkdir' ? currentDir : operations.selected?.path}
          </p>
          {action === 'mkdir' ? <label className="file-field">目录名称
            <input autoFocus required className="input-base" placeholder="输入目录名称" disabled={busy}
              value={operations.folderName} onChange={event => operations.setFolderName(event.target.value)} />
          </label> : action === 'rename' ? <label className="file-field">名称
            <input autoFocus required className="input-base" disabled={busy} value={operations.renameTo}
              onFocus={event => event.target.select()} onChange={event => operations.setRenameTo(event.target.value)} />
          </label> : <>
            <label className="file-field">操作方式
              <select autoFocus className="input-base" disabled={busy} value={operations.transferMode}
                onChange={event => operations.setTransferMode(event.target.value)}>
                {Object.entries(modes).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
              </select>
            </label>
            <label className="file-field">目标目录
              <input required className="input-base" placeholder="例如 /media/电影" disabled={busy}
                value={operations.destPath} onChange={event => operations.setDestPath(event.target.value)} />
            </label>
          </>}
          {operations.error && <p role="alert" className="text-sm text-(--app-danger-text)">{operations.error}</p>}
        </div>
        <footer className="file-dialog-footer">
          <button type="button" className="file-button" disabled={busy} onClick={onClose}>取消</button>
          <button type="submit" className="file-button file-button-primary" disabled={busy || !valid}>
            {busy && <Loader2 size={16} className="animate-spin" />}
            {action === 'mkdir' ? '创建' : action === 'rename' ? '保存' : modes[operations.transferMode as keyof typeof modes]}
          </button>
        </footer>
      </form>
    </dialog>, document.body,
  )
}
