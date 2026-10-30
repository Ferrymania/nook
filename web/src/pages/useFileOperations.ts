import { useEffect, useState } from 'react'
import toast from 'react-hot-toast'

import { filesAPI, type FileEntry } from '../api/files'
import { confirmAction } from '../components/confirmAction'

type UseFileOperationsOptions = {
  currentDir: string
  path: string
  refresh: () => void
}

export function useFileOperations({ currentDir, path, refresh }: UseFileOperationsOptions) {
  const [selected, setSelected] = useState<FileEntry | null>(null)
  const [folderName, setFolderName] = useState('')
  const [renameTo, setRenameTo] = useState('')
  const [destPath, setDestPath] = useState('')
  const [transferMode, setTransferMode] = useState('copy')
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    setSelected(null)
    setRenameTo('')
  }, [path])

  const choose = (entry: FileEntry) => {
    setSelected(entry)
    setRenameTo(entry.name)
    setDestPath('')
    setError('')
  }

  const createFolder = async () => {
    if (busy || !currentDir || !folderName.trim()) return false
    setBusy('mkdir')
    setError('')
    try {
      const res = await filesAPI.createFolder(currentDir, folderName.trim())
      toast.success(`已创建目录：${res.path}`)
      setFolderName('')
      refresh()
      return true
    } catch (err: unknown) {
      setError((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '创建失败')
      return false
    } finally {
      setBusy('')
    }
  }

  const renameSelected = async () => {
    if (busy || !selected || !renameTo.trim()) return false
    setBusy('rename')
    setError('')
    try {
      const res = await filesAPI.rename(selected.path, renameTo.trim())
      toast.success(`已重命名：${res.path}`)
      setSelected(null)
      refresh()
      return true
    } catch (err: unknown) {
      setError((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '重命名失败')
      return false
    } finally {
      setBusy('')
    }
  }

  const deleteSelected = async (entry: FileEntry) => {
    if (busy) return
    const ok = await confirmAction({
      title: '确认删除文件',
      message: `将删除：${entry.path}。此操作不可恢复，请确认不是媒体库根目录。`,
      confirmText: '确认删除',
      danger: true,
    })
    if (!ok) return
    setBusy('delete')
    try {
      await filesAPI.remove(entry.path)
      toast.success('已删除')
      setSelected(null)
      refresh()
    } catch (err: unknown) {
      toast.error((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '删除失败')
    } finally {
      setBusy('')
    }
  }

  const transferSelected = async () => {
    if (busy || !selected || selected.is_dir || !destPath.trim()) return false
    setBusy('transfer')
    setError('')
    try {
      const res = await filesAPI.transfer(selected.path, destPath.trim(), transferMode)
      toast.success(`已完成转移：${res.path}`)
      if (transferMode === 'move') setSelected(null)
      refresh()
      return true
    } catch (err: unknown) {
      setError((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '转移失败')
      return false
    } finally {
      setBusy('')
    }
  }

  return {
    selected,
    folderName,
    renameTo,
    destPath,
    transferMode,
    busy,
    error,
    setError,
    clearSelection: () => setSelected(null),
    setFolderName,
    setRenameTo,
    setDestPath,
    setTransferMode,
    choose,
    createFolder,
    renameSelected,
    deleteSelected,
    transferSelected,
  }
}
