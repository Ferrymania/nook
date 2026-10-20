import { useCallback, useEffect, useRef, useState } from 'react'
import toast from 'react-hot-toast'

import { libraryAPI } from '../api/library'
import { scraperAPI } from '../api/scraper'
import { confirmAction } from '../components/confirmAction'

export function useLibraryScrapeStatus({ libraryID = '', allLibraries = false, isAdmin, episodeArtwork, onLibraryChanged }: {
  libraryID?: string
  allLibraries?: boolean
  isAdmin: boolean
  episodeArtwork: boolean
  onLibraryChanged: () => void
}) {
  const scope = allLibraries ? 'all' : libraryID
  const enabled = isAdmin && !!scope
  const [state, setState] = useState({ scope, active: false })
  const active = useRef(false)
  const submitting = useRef(false)
  const generation = useRef(0)
  const controller = useRef<AbortController | null>(null)
  const changed = useRef(onLibraryChanged)
  useEffect(() => { changed.current = onLibraryChanged }, [onLibraryChanged])

  const apply = useCallback((next: boolean) => {
    const finished = active.current && !next
    active.current = next
    setState({ scope, active: next })
    if (finished) changed.current()
  }, [scope])

  const refresh = useCallback(async () => {
    if (!enabled || submitting.current || controller.current) return
    const request = new AbortController()
    const version = generation.current
    controller.current = request
    try {
      const counts = allLibraries
        ? (await scraperAPI.queue(undefined, 1, 1, request.signal)).counts
        : await libraryAPI.scrapeStatus(libraryID, request.signal)
      if (!request.signal.aborted && version === generation.current) apply(counts.pending + counts.running > 0)
    } catch {
      // A lost status request must not announce that known background work ended.
    } finally {
      if (controller.current === request) controller.current = null
    }
  }, [allLibraries, apply, enabled, libraryID])

  useEffect(() => {
    generation.current += 1
    active.current = false
    submitting.current = false
    setState({ scope, active: false })
    void refresh()
    const visible = () => { if (document.visibilityState === 'visible') void refresh() }
    document.addEventListener('visibilitychange', visible)
    window.addEventListener('focus', visible)
    const timer = window.setInterval(visible, 5000)
    return () => {
      generation.current += 1
      controller.current?.abort()
      controller.current = null
      window.clearInterval(timer)
      document.removeEventListener('visibilitychange', visible)
      window.removeEventListener('focus', visible)
    }
  }, [scope, refresh])

  const handleScrape = useCallback(async () => {
    if (!enabled || submitting.current || active.current) return
    const version = generation.current
    submitting.current = true
    try {
      if (allLibraries) {
        const confirmed = await confirmAction({
          title: '刮削全部媒体库',
          message: '将刮削所有已启用的本地媒体库，包括已匹配的媒体，可能产生较多请求。进度可在刮削队列查看。',
          confirmText: '开始刮削',
          danger: false,
        })
        if (!confirmed || version !== generation.current || active.current) return
      }
      controller.current?.abort()
      controller.current = null
      apply(true)
      const options = { episode_images: episodeArtwork, refresh_matched: true }
      const result = allLibraries
        ? await scraperAPI.enqueueAll(options)
        : await libraryAPI.scrape(libraryID, options)
      if (version !== generation.current) return
      toast.success(result.enqueued > 0 ? `${allLibraries ? '全部' : ''}媒体库刮削已加入队列，可在任务队列查看进度` : '没有新增刮削任务，正在核对当前队列')
    } catch {
      if (version !== generation.current) return
      toast.error('刮削请求未确认，请查看任务队列')
    } finally {
      if (version === generation.current) {
        submitting.current = false
        void refresh()
      }
    }
  }, [allLibraries, apply, enabled, episodeArtwork, libraryID, refresh])

  return { scraping: enabled && state.scope === scope && state.active, handleScrape }
}
