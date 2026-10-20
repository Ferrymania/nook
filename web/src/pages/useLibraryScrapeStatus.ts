import { useCallback, useEffect, useRef, useState } from 'react'
import toast from 'react-hot-toast'

import { libraryAPI } from '../api/library'

export function useLibraryScrapeStatus({ libraryID, isAdmin, episodeArtwork, onLibraryChanged }: {
  libraryID: string
  isAdmin: boolean
  episodeArtwork: boolean
  onLibraryChanged: () => void
}) {
  const [state, setState] = useState({ libraryID, active: false })
  const active = useRef(false)
  const submitting = useRef(false)
  const generation = useRef(0)
  const controller = useRef<AbortController | null>(null)
  const changed = useRef(onLibraryChanged)
  useEffect(() => { changed.current = onLibraryChanged }, [onLibraryChanged])

  const apply = useCallback((next: boolean) => {
    const finished = active.current && !next
    active.current = next
    setState({ libraryID, active: next })
    if (finished) changed.current()
  }, [libraryID])

  const refresh = useCallback(async () => {
    if (!isAdmin || !libraryID || submitting.current || controller.current) return
    const request = new AbortController()
    const version = generation.current
    controller.current = request
    try {
      const counts = await libraryAPI.scrapeStatus(libraryID, request.signal)
      if (!request.signal.aborted && version === generation.current) apply(counts.pending + counts.running > 0)
    } catch {
      // A lost status request must not announce that known background work ended.
    } finally {
      if (controller.current === request) controller.current = null
    }
  }, [apply, isAdmin, libraryID])

  useEffect(() => {
    generation.current += 1
    active.current = false
    submitting.current = false
    setState({ libraryID, active: false })
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
  }, [libraryID, refresh])

  const handleScrape = useCallback(async () => {
    if (!isAdmin || !libraryID || submitting.current || active.current) return
    const version = generation.current
    controller.current?.abort()
    controller.current = null
    submitting.current = true
    apply(true)
    try {
      const result = await libraryAPI.scrape(libraryID, { episode_images: episodeArtwork, refresh_matched: true })
      if (version !== generation.current) return
      toast.success(result.enqueued > 0 ? '媒体库刮削已加入队列，可在任务队列查看进度' : '没有新增刮削任务，正在核对当前队列')
    } catch {
      if (version !== generation.current) return
      toast.error('刮削请求未确认，请查看任务队列')
    } finally {
      if (version === generation.current) {
        submitting.current = false
        void refresh()
      }
    }
  }, [apply, episodeArtwork, isAdmin, libraryID, refresh])

  return { scraping: state.libraryID === libraryID && state.active, handleScrape }
}
