import { FormEvent, useEffect, useState } from 'react'
import toast from 'react-hot-toast'

import { libraryAPI, type LibraryRootInput, type LibraryPathPreview, type LibraryScrapeSettingsView } from '../api/library'
import type { Library, LibraryRoot } from '../types'
import { invalidateLibraries } from '../utils/libraryCache'
import { confirmAction } from '../components/confirmAction'
import { apiErrorMessage, createRootPayload, duplicateLibrary, rootDraftError, scrapeSettingsError, displayLibraryRootName, displayLibraryRootPath, emptyRootDraft, rootDraftKey, type RootDraft } from './adminLibraryPanelModel'

export function useAdminLibraryPanel() {
  const { libs, refresh } = useAdminLibraryList()
  const createForm = useCreateLibraryForm(refresh, libs)
  const editableRoots = useEditableRootDrafts()
  const rootActions = useEditableLibraryRootActions(refresh, editableRoots)
  const libraryActions = useLibraryActions(refresh)

  return { libs, createForm, editableRoots, rootActions, libraryActions }
}

function useAdminLibraryList() {
  const [libs, setLibs] = useState<Library[]>([])
  const refresh = () => {
    // 后台任何库变更都会走到这里；顺带清掉前台会话缓存，
    // 避免返回首页/媒体库页后 30 秒 TTL 内还显示旧列表。
    invalidateLibraries()
    return libraryAPI
      .list({ includeHidden: true })
      .then((libs) => setLibs(libs.filter((l) => !l.is_remote_emby)))  // 远程挂载库只读，不在后台管理列表内
  }

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  return { libs, refresh }
}

function useCreateLibraryForm(refresh: () => Promise<void>, libs: Library[]) {
  const [mode, setMode] = useState<'create' | 'append' | null>(null)
  const [targetLibrary, setTargetLibrary] = useState<Library | null>(null)
  const [name, setName] = useState('')
  const [roots, setRoots] = useState<RootDraft[]>([emptyRootDraft()])
  const [type, setType] = useState('movie')
  const [coverURL, setCoverURL] = useState('')
  const [createPerSubfolder, setCreatePerSubfolder] = useState(false)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const [scrape, setScrape] = useState<LibraryScrapeSettingsView | null>(null)
  const [loadingScrape, setLoadingScrape] = useState(false)
  const [scrapeError, setScrapeError] = useState('')

  const loadScrapeDefaults = async () => {
    setLoadingScrape(true)
    setScrapeError('')
    try { setScrape(await libraryAPI.scrapeDefaults()) }
    catch { setScrapeError('无法加载刮削默认设置，请重试') }
    finally { setLoadingScrape(false) }
  }

  const open = (library?: Library) => {
    if (creating) return
    setMode(library ? 'append' : 'create')
    setTargetLibrary(library ?? null)
    setName('')
    setRoots([emptyRootDraft()])
    setType('movie')
    setCoverURL('')
    setCreatePerSubfolder(false)
    setError('')
    if (!library) { setScrape(null); void loadScrapeDefaults() }
  }

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault()
    if (creating || !mode) return
    const existingRoots = mode === 'append' ? (libs.find(l => l.id === targetLibrary?.id)?.roots ?? []) : []
    const validation = rootDraftError(roots, existingRoots)
    if (validation) { setError(validation); return }
    if (mode === 'create') {
      if (!createPerSubfolder && !name.trim()) { setError('请填写媒体库名称'); return }
      if (!createPerSubfolder && duplicateLibrary(libs, name, type)) { setError('同名同类型媒体库已存在，请使用该库的“添加目录”'); return }
      if (!scrape) { setError('请先加载刮削设置'); return }
      const scrapeValidation = scrapeSettingsError(scrape)
      if (scrapeValidation) { setError(scrapeValidation); return }
    }
    setCreating(true)
    setError('')
    let added = 0
    try {
      if (mode === 'append' && targetLibrary) {
        for (const root of createRootPayload(roots)) {
          await libraryAPI.addRoot(targetLibrary.id, root)
          added++
        }
        toast.success(`已向「${targetLibrary.name}」添加 ${added} 个目录`)
      } else {
        const options = { create_only: true, scrape_settings: scrape!.settings, auto_probe: scrape!.auto_probe }
        if (createPerSubfolder) {
          const { libraries } = await libraryAPI.createPerSubfolder(roots[0].path.trim(), type, coverURL.trim(), options)
          toast.success(`已按目录创建 ${libraries.length} 个媒体库`)
        } else {
          const saved = await libraryAPI.createWithRoots(name.trim(), type, createRootPayload(roots), coverURL.trim(), options)
          toast.success(`已创建媒体库「${saved.name}」`)
        }
      }
      setMode(null)
      setRoots([emptyRootDraft()])
    } catch (err: unknown) {
      if (added) setRoots(previous => previous.slice(added))
      setError(`${added ? `已添加 ${added} 个目录，其余目录保存失败：` : ''}${apiErrorMessage(err, mode === 'append' ? '添加目录失败' : '创建失败')}`)
    } finally {
      setCreating(false)
      await refresh().catch(() => toast.error('媒体库列表刷新失败，请重试'))
    }
  }

  const updateRoot = (index: number, patch: Partial<RootDraft>) => {
    setRoots(previous => previous.map((root, i) => i === index ? { ...root, ...patch } : root))
  }

  return {
    mode, targetLibrary, name, type, coverURL, roots, createPerSubfolder, creating, error,
    scrape, setScrape, loadingScrape, scrapeError, loadScrapeDefaults,
    setName, setType, setCoverURL, setCreatePerSubfolder, updateRoot,
    openCreate: () => open(), openAppend: (library: Library) => open(library),
    close: () => { if (!creating) setMode(null) },
    addRoot: () => setRoots(previous => [...previous, emptyRootDraft()]),
    removeRoot: (index: number) => setRoots(previous => previous.length <= 1 ? previous : previous.filter((_, i) => i !== index)),
    handleCreate,
  }
}

function useEditableRootDrafts() {
  const [rootDrafts, setRootDrafts] = useState<Record<string, RootDraft>>({})

  const editableRootDraft = (libraryID: string, root: LibraryRoot): RootDraft => {
    const key = rootDraftKey(libraryID, root.id)
    return rootDrafts[key] ?? {
      name: displayLibraryRootName(root.name, root.path),
      path: displayLibraryRootPath(root.path),
      enabled: root.enabled,
      sort_order: root.sort_order,
    }
  }

  const setEditableRootDraft = (libraryID: string, root: LibraryRoot, patch: Partial<RootDraft>) => {
    const key = rootDraftKey(libraryID, root.id)
    setRootDrafts((prev) => ({ ...prev, [key]: { ...editableRootDraft(libraryID, root), ...patch } }))
  }

  const clearEditableRootDraft = (libraryID: string, rootID: string) => {
    setRootDrafts((prev) => {
      const next = { ...prev }
      delete next[rootDraftKey(libraryID, rootID)]
      return next
    })
  }

  return { editableRootDraft, setEditableRootDraft, clearEditableRootDraft }
}

type EditableRootDrafts = ReturnType<typeof useEditableRootDrafts>

function useEditableLibraryRootActions(refresh: () => Promise<void>, drafts: EditableRootDrafts) {
  const [pathChange, setPathChange] = useState<{ libraryID: string; root: LibraryRoot; draft: LibraryRootInput } | null>(null)
  const saveLibraryRoot = async (libraryID: string, root: LibraryRoot) => {
    const draft = drafts.editableRootDraft(libraryID, root)
    if (!draft.path?.trim()) {
      toast.error('请填写路径')
      return false
    }
    const payload = {
      name: draft.name?.trim(),
      path: draft.path.trim(),
      enabled: draft.enabled,
      sort_order: draft.sort_order,
    }
    if (payload.path !== root.path) {
      setPathChange({ libraryID, root, draft: payload })
      return false
    }
    try {
      await libraryAPI.updateRoot(libraryID, root.id, payload)
      drafts.clearEditableRootDraft(libraryID, root.id)
      toast.success('目录已保存')
      await refresh()
      return true
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '保存目录失败'))
      return false
    }
  }

  const confirmPathChange = async (preview: LibraryPathPreview) => {
    if (!pathChange) return
    await libraryAPI.updateRoot(pathChange.libraryID, pathChange.root.id, {
      ...pathChange.draft,
      path: preview.new_path,
      change_mode: preview.change_mode,
      preview_token: preview.preview_token,
    })
    drafts.clearEditableRootDraft(pathChange.libraryID, pathChange.root.id)
    setPathChange(null)
    toast.success(preview.change_mode === 'migrate' ? '路径已更新，原有媒体记录已保留' : '已切换目录，旧索引已清除')
    await refresh().catch(() => toast.error('已保存，请刷新媒体库列表查看结果'))
  }

  const scanLibraryRoot = async (libraryID: string, root: LibraryRoot) => {
    if (!root.id) return
    await libraryAPI.scanRoot(libraryID, root.id)
    toast.success('路径扫描已加入后台任务')
  }

  const toggleLibraryRoot = async (libraryID: string, root: LibraryRoot) => {
    const enabled = !root.enabled
    await libraryAPI.updateRoot(libraryID, root.id, { enabled })
    drafts.clearEditableRootDraft(libraryID, root.id)
    await refresh()
  }

  const removeLibraryRoot = async (library: Library, root: LibraryRoot) => {
    if (!(await confirmAction({ title: '删除媒体库路径', message: `确定删除「${displayLibraryRootPath(root.path)}」?`, confirmText: '删除' }))) return
    await libraryAPI.removeRoot(library.id, root.id)
    toast.success('路径已删除')
    await refresh()
  }

  return { cancelRootEdit: drafts.clearEditableRootDraft, saveLibraryRoot, scanLibraryRoot, toggleLibraryRoot, removeLibraryRoot, pathChange, confirmPathChange, closePathChange: () => setPathChange(null) }
}

function useLibraryActions(refresh: () => Promise<void>) {
  const [coverLibrary, setCoverLibrary] = useState<Library | null>(null)

  const scanLibrary = async (library: Library) => {
    const result = await libraryAPI.scan(library.id)
    if (result.queued) toast.success(result.message || '媒体库扫描已加入后台队列，会自动入库')
    else toast.success(`扫描完成，新增 ${result.added ?? 0}，更新 ${result.updated ?? 0}`)
  }

  const toggleCarouselLibrary = async (library: Library) => {
    const next = !library.carousel_enabled
    await libraryAPI.update(library.id, { carousel_enabled: next })
    toast.success(next ? `「${library.name}」已加入首页轮播` : `「${library.name}」已移出首页轮播`)
    await refresh()
  }

  const reorderLibraries = async (orderedLibs: Library[]) => {
    await libraryAPI.reorder(orderedLibs.map((l) => l.id))
    await refresh()
  }

  const removeLibrary = async (library: Library) => {
    if (!(await confirmAction({ title: '删除媒体库', message: `确定删除「${library.name}」?`, confirmText: '删除' }))) return
    await libraryAPI.remove(library.id)
    toast.success('已删除')
    await refresh()
  }


  const saveLibraryCover = async (library: Library, coverURL: string) => {
    const value = coverURL.trim()
    await libraryAPI.update(library.id, { cover_url: value })
    toast.success(value ? '媒体库封面已保存' : '已恢复自动封面')
    setCoverLibrary(null)
    await refresh().catch(() => toast.error('封面已保存，媒体库列表刷新失败，请刷新页面'))
  }

  return {
    scanLibrary, removeLibrary, toggleCarouselLibrary, reorderLibraries,
    coverLibrary, editLibraryCover: setCoverLibrary, closeLibraryCover: () => setCoverLibrary(null), saveLibraryCover,
  }
}
