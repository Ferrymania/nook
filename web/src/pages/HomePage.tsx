import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { libraryAPI } from '../api/library'
import { historyAPI } from '../api/history'
import type { HistoryItem } from '../api/playback'
import type { Library, Media } from '../types'
import type { SeriesCard } from '../utils/groupSeries'
import { fetchLibraries, peekLibraries } from '../utils/libraryCache'
import { usePinnedLibraries } from '../hooks/usePinnedLibraries'
import { useLibraryListSort } from '../hooks/useLibraryListSort'
import { sortLibrariesByField } from '../utils/libraryListSort'
import { partitionPreviewIDs } from '../utils/remoteEmby'
import { selectHomeCarouselMedia } from '../utils/homeCarousel'
import {
  ContinueWatchingSection,
  ContinueWatchingSkeleton,
  HomeCarouselSection,
  HomeEmptyState,
  HomeLibrariesSection,
  HomeLoadingState,
} from './HomePageSections'

export function HomePage() {
  const [libraries, setLibraries] = useState<Library[]>([])
  const [libraryData, setLibraryData] = useState<Record<string, { cards: SeriesCard[]; items: Media[]; total: number }>>({})
  const [history, setHistory] = useState<HistoryItem[]>([])
  // 两个请求独立渐进加载：库列表控制整页首屏，播放记录区块自行渲染，
  // 避免整页被 Promise.all 里最慢的那个请求拖住。
  const [librariesLoading, setLibrariesLoading] = useState(true)
  const [historyLoading, setHistoryLoading] = useState(true)
  const removeHistoryItem = useCallback((id: string) => {
    setHistory((current) => current.filter((item) => item.id !== id))
  }, [])
  const { pinnedIds } = usePinnedLibraries()
  const { field: sortField, order: sortOrder } = useLibraryListSort()

  // 1. 媒体库元数据极速加载（不带 preview，毫秒级秒开首屏）。
  //    会话内已有缓存则先用缓存立即渲染，后台仍刷新一次兜底。
  useEffect(() => {
    let cancelled = false

    const cached = peekLibraries()
    if (cached && cached.length > 0) {
      setLibraries(cached)
      setLibrariesLoading(false)
    }

    fetchLibraries()
      .then((rows) => (Array.isArray(rows) ? rows : []))
      .catch(() => [] as Library[])
      .then((libs) => {
        if (cancelled) return
        // 失败兜底返回空数组时，不要覆盖已先行渲染的缓存数据
        setLibraries((prev) => (libs.length === 0 && prev.length > 0 ? prev : libs))
        setLibrariesLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [])

  // 2. 继续观看记录独立渐进加载
  useEffect(() => {
    let cancelled = false

    historyAPI
      .continueWatching(12)
      .then((rows) =>
        rows.map(
          (row): HistoryItem => ({
            ...row.history,
            created_at: '',
            updated_at: '',
            media: row.media,
          }),
        ),
      )
      .catch(() => [] as HistoryItem[])
      .then((rows) => {
        if (cancelled) return
        setHistory(rows.filter((h) => h && !h.completed))
        setHistoryLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [])

  // 与 /libraries 共用同一份排序偏好（默认库名倒序）：置顶库始终在前，
  // 组内按偏好排序字段排。这样在媒体库页改过排序后，首页顺序也一致。
  const sortedLibraries = useMemo(
    () => sortLibrariesByField(libraries, pinnedIds, sortField, sortOrder),
    [libraries, pinnedIds, sortField, sortOrder],
  )

  // 入口卡片按需加载 2 张封面，开启轮播的库预取 10 张；按数量去重。
  const fetchedPreviewLimitsRef = useRef<Map<string, number>>(new Map())
  const fetchingPreviewLimitsRef = useRef<Map<string, number>>(new Map())

  const fetchPreviews = useCallback(async (ids: string[], limit = 10) => {
    const uniqueIDs = Array.from(new Set(ids.filter(Boolean)))
    const targets = uniqueIDs.filter(
      (id) =>
        (fetchedPreviewLimitsRef.current.get(id) ?? 0) < limit &&
        (fetchingPreviewLimitsRef.current.get(id) ?? 0) < limit,
    )
    if (targets.length === 0) return
    targets.forEach((id) => fetchingPreviewLimitsRef.current.set(id, limit))

    // 本地库一次拉完（同一条窗口查询、缓存键稳定）。远程库单独一批，
    // 避免一个慢的 Emby 挂载拖住整页预览。
    const batches = partitionPreviewIDs(targets)

    await Promise.allSettled(
      batches.map(async (batch) => {
        let loaded = false
        try {
          const rows = await libraryAPI.listPreviews(batch, limit)
          loaded = true
          const accepted = rows.filter(
            (row) => (fetchedPreviewLimitsRef.current.get(row.id) ?? 0) < limit,
          )
          accepted.forEach((row) => {
            fetchedPreviewLimitsRef.current.set(
              row.id,
              Math.max(fetchedPreviewLimitsRef.current.get(row.id) ?? 0, limit),
            )
          })
          setLibraryData((prev) => {
            const next = { ...prev }
            for (const row of accepted) {
              next[row.id] = {
                cards: row.cards ?? [],
                items: [],
                total: row.total ?? 0,
              }
            }
            return accepted.length > 0 ? next : prev
          })
        } catch {
          // 单个批次失败不影响其他批次；导航回来时会重试。
        } finally {
          batch.forEach((id) => {
            if (loaded) {
              fetchedPreviewLimitsRef.current.set(
                id,
                Math.max(fetchedPreviewLimitsRef.current.get(id) ?? 0, limit),
              )
            }
            if (fetchingPreviewLimitsRef.current.get(id) === limit) {
              fetchingPreviewLimitsRef.current.delete(id)
            }
          })
        }
      }),
    )
  }, [])

  // 3. 首屏只预取轮播所需的预览；入口卡片进入视口后自行按批加载。
  useEffect(() => {
    const carouselLibIds = sortedLibraries
      .filter((l) => l.carousel_enabled === true)
      .map((l) => l.id)
    void fetchPreviews(carouselLibIds, 10)
  }, [sortedLibraries, fetchPreviews])

  // Quick lookup map for libraries
  const libraryMap = useMemo(() => {
    const map = new Map<string, Library>()
    for (const lib of libraries) {
      map.set(lib.id, lib)
    }
    return map
  }, [libraries])

  // Counts lookup
  const libraryCounts = useMemo(() => {
    const counts: Record<string, number> = {}
    for (const lib of libraries) {
      counts[lib.id] = lib.total ?? libraryData[lib.id]?.total ?? 0
    }
    return counts
  }, [libraries, libraryData])

  const carouselItems = useMemo(
    () => selectHomeCarouselMedia(sortedLibraries, libraryData),
    [sortedLibraries, libraryData],
  )

  // 库列表还没回来先展示整页 loading；库为空时再等一下播放记录，
  // 以免在"空站点"和"有观看记录"两个终态之间闪空白。
  if (librariesLoading || (libraries.length === 0 && historyLoading)) {
    return <HomeLoadingState />
  }
  const empty =
    libraries.length === 0 &&
    history.length === 0

  if (empty) {
    return <HomeEmptyState />
  }

  return (
    <div className="nook-home pb-10">
      <h1 className="sr-only">首页</h1>
      {/* 1. 顶部海报轮播区 */}
      {carouselItems.length > 0 && (
        <HomeCarouselSection items={carouselItems} libraryMap={libraryMap} />
      )}

      {/* 2. 继续观看（独立加载，未回来前展示同构骨架） */}
      {historyLoading && <ContinueWatchingSkeleton />}
      {!historyLoading && history.length > 0 && <ContinueWatchingSection history={history} onHistoryChanged={removeHistoryItem} />}

      {sortedLibraries.length > 0 && (
        <HomeLibrariesSection
          libraries={sortedLibraries}
          libraryData={libraryData}
          libraryCounts={libraryCounts}
          onNeedPreviews={fetchPreviews}
        />
      )}
    </div>
  )
}
