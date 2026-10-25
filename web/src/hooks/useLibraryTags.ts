import { useCallback, useEffect, useRef, useState } from 'react'
import toast from 'react-hot-toast'

import {
  ALL_TAG_ID,
  assignLibrariesToTag,
  attachLibraryToTag,
  dedupeLibraryTags,
  loadLibraryTags,
  normalizeLibraryTags,
  normalizeTagName,
  readCachedLibraryTags,
  readSelectedTagId,
  reorderLibraryTags,
  resolveSelectedTagId,
  saveLibraryTags,
  writeSelectedTagId,
  type LibraryTag,
} from '../utils/libraryTags'

export type UseLibraryTagsResult = {
  /** 当前用户维护的标签（顺序即标签栏顺序）。 */
  tags: LibraryTag[]
  /** 标签数据是否仍在首次加载中。 */
  loading: boolean
  /** 是否有写入在途（可用于禁用重复点击）。 */
  saving: boolean
  /** 最近一次加载或保存失败；读取失败时显示本地缓存。 */
  loadError: boolean
  /** 当前选中的标签栏：ALL_TAG_ID 表示「全部」。 */
  selectedTagId: string
  setSelectedTagId: (tagId: string) => void
  createTag: (name: string) => Promise<string>
  renameTag: (from: string, to: string) => Promise<void>
  /** 按给定标签名顺序重排标签栏（拖拽排序的落点）。 */
  reorderTags: (orderedNames: string[]) => Promise<void>
  removeTag: (name: string) => Promise<void>
  /** 覆盖某个标签下的媒体库集合（保持传入顺序）。 */
  setTagLibraries: (name: string, libraryIds: string[]) => Promise<void>
  /** 把媒体库挂到标签下；tagName 为空表示移出所有标签。 */
  assignLibrary: (libraryId: string, tagName: string) => Promise<void>
  /** 批量把媒体库挂到标签下；tagName 为空表示批量移出所有标签。一次写入。 */
  assignLibraries: (libraryIds: string[], tagName: string) => Promise<void>
}

/**
 * 媒体库标签的用户级读写。
 *
 * 写入采用「先乐观更新、串行提交、失败回滚」：标签是整份替换（`PUT`），
 * 并发请求可能让旧快照最后落库，因此所有写操作串行排队，每次发送的都是
 * 排队时的最新本地状态。读取失败时显示缓存，写入失败时回滚并提示。
 */
export function useLibraryTags(): UseLibraryTagsResult {
  const [tags, setTags] = useState<LibraryTag[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [loadError, setLoadError] = useState(false)
  const [selectedTagId, setSelectedTagIdState] = useState(() => readSelectedTagId())

  const tagsRef = useRef<LibraryTag[]>([])
  const selectedRef = useRef(selectedTagId)
  const savedRef = useRef<LibraryTag[]>([])
  const chainRef = useRef<Promise<unknown>>(Promise.resolve())
  const pendingRef = useRef(0)

  useEffect(() => {
    tagsRef.current = tags
  }, [tags])

  useEffect(() => {
    selectedRef.current = selectedTagId
  }, [selectedTagId])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    loadLibraryTags()
      .then((rows) => {
        if (cancelled) return
        tagsRef.current = rows
        savedRef.current = rows
        setTags(rows)
        setLoadError(false)
      })
      .catch(() => {
        if (cancelled) return
        // 读取失败时显示缓存；后续编辑仍需由服务器确认保存。
        const cached = readCachedLibraryTags()
        tagsRef.current = cached
        savedRef.current = cached
        setTags(cached)
        setLoadError(true)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [])

  // 选中的标签可能被删除，落回「全部」以免标签栏空白。
  // 注意：标签首次加载完成前 tags 为空集合，此时不能据此判定标签已被删除，
  // 否则从子页面返回时，持久化的选中标签会被误重置为「全部」。
  useEffect(() => {
    if (loading) return
    const resolved = resolveSelectedTagId(tags, selectedTagId)
    if (resolved !== selectedTagId) {
      selectedRef.current = resolved
      setSelectedTagIdState(resolved)
    }
  }, [loading, tags, selectedTagId])

  const setSelectedTagId = useCallback((tagId: string) => {
    const next = tagId && tagId !== ALL_TAG_ID ? tagId : ALL_TAG_ID
    selectedRef.current = next
    writeSelectedTagId(next)
    setSelectedTagIdState(next)
  }, [])

  const mutate = useCallback((updater: (current: LibraryTag[]) => LibraryTag[]) => {
    const rollback = savedRef.current
    const next = dedupeLibraryTags(normalizeLibraryTags(updater(tagsRef.current)))
    tagsRef.current = next
    setTags(next)

    pendingRef.current += 1
    setSaving(true)
    const task = chainRef.current.then(async () => {
      // 发送排队时的最新本地状态：链上更早的写入已经被后面的状态取代。
      const snapshot = tagsRef.current
      try {
        const saved = await saveLibraryTags(snapshot)
        savedRef.current = saved
        setLoadError(false)
        if (tagsRef.current === snapshot) {
          tagsRef.current = saved
          setTags(saved)
        }
      } catch {
        // 用最后一次成功保存的状态回滚，避免前端显示一份服务端并不存在的标签。
        tagsRef.current = rollback
        savedRef.current = rollback
        setTags(rollback)
        setLoadError(true)
        toast.error('标签保存失败，请稍后重试')
      }
    })
    chainRef.current = task.catch(() => undefined)
    return task.finally(() => {
      pendingRef.current = Math.max(0, pendingRef.current - 1)
      if (pendingRef.current === 0) setSaving(false)
    })
  }, [])

  const createTag = useCallback(
    async (name: string) => {
      const trimmed = normalizeTagName(name)
      if (!trimmed) return ''
      if (tagsRef.current.some((tag) => tag.name.toLowerCase() === trimmed.toLowerCase())) {
        return trimmed
      }
      await mutate((current) => [...current, { name: trimmed, library_ids: [] }])
      return tagsRef.current.some((tag) => tag.name === trimmed) ? trimmed : ''
    },
    [mutate],
  )

  const renameTag = useCallback(
    async (from: string, to: string) => {
      const trimmed = normalizeTagName(to)
      if (!trimmed || trimmed === from) return
      await mutate((current) =>
        current.map((tag) => (tag.name === from ? { ...tag, name: trimmed } : tag)),
      )
      // 重命名后选中项要跟着换到新名字，否则标签栏会落回「全部」。
      if (selectedRef.current === from) setSelectedTagId(trimmed)
    },
    [mutate, setSelectedTagId],
  )

  const reorderTags = useCallback(
    async (orderedNames: string[]) => {
      await mutate((current) => reorderLibraryTags(current, orderedNames))
    },
    [mutate],
  )

  const removeTag = useCallback(
    async (name: string) => {
      await mutate((current) => current.filter((tag) => tag.name !== name))
      if (selectedRef.current === name) setSelectedTagId(ALL_TAG_ID)
    },
    [mutate, setSelectedTagId],
  )

  const setTagLibraries = useCallback(
    async (name: string, libraryIds: string[]) => {
      await mutate((current) =>
        current.map((tag) => (tag.name === name ? { ...tag, library_ids: libraryIds } : tag)),
      )
    },
    [mutate],
  )

  const assignLibrary = useCallback(
    async (libraryId: string, tagName: string) => {
      await mutate((current) => {
        if (tagName) return attachLibraryToTag(current, libraryId, tagName)
        return current.map((tag) => ({
          ...tag,
          library_ids: tag.library_ids.filter((id) => id !== libraryId),
        }))
      })
    },
    [mutate],
  )

  const assignLibraries = useCallback(
    async (libraryIds: string[], tagName: string) => {
      await mutate((current) => assignLibrariesToTag(current, libraryIds, tagName))
    },
    [mutate],
  )

  return {
    tags,
    loading,
    saving,
    loadError,
    selectedTagId,
    setSelectedTagId,
    createTag,
    renameTag,
    reorderTags,
    removeTag,
    setTagLibraries,
    assignLibrary,
    assignLibraries,
  }
}
