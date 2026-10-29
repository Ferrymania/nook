import { useCallback, useEffect, useRef, useState } from 'react'
import toast from 'react-hot-toast'

import { libraryAPI } from '../api/library'
import type { SeriesCard } from '../utils/groupSeries'
import { seriesManualScrapeTarget, type SeriesManualScrapeTarget } from '../utils/manualScrapeTargets'

export function useLibraryManualScrape(libraryID: string) {
  const [target, setTarget] = useState<SeriesManualScrapeTarget | null>(null)
  const [loadingMediaID, setLoadingMediaID] = useState('')
  const request = useRef(0)
  const inFlight = useRef(false)
  const invalidateRequest = useCallback(() => { request.current++ }, [])

  useEffect(() => {
    setTarget(null)
    setLoadingMediaID('')
    inFlight.current = false
    return invalidateRequest
  }, [libraryID, invalidateRequest])

  const open = useCallback(async (series: SeriesCard) => {
    if (inFlight.current) return
    const seq = ++request.current
    inFlight.current = true
    setLoadingMediaID(series.rep.id)
    try {
      // Fetch the entire series, independent of list pagination and the selected season.
      const page = await libraryAPI.listSeriesEpisodes(libraryID, series.key, series.rep.id)
      if (seq !== request.current) return
      setTarget(seriesManualScrapeTarget(series, page.items ?? []))
    } catch (err: unknown) {
      if (seq !== request.current) return
      const message = (err as { response?: { data?: { error?: string } } })?.response?.data?.error
        || (err instanceof Error ? err.message : '加载整剧集列表失败，请重试')
      toast.error(message)
    } finally {
      if (seq === request.current) {
        inFlight.current = false
        setLoadingMediaID('')
      }
    }
  }, [libraryID])

  const close = useCallback(() => setTarget(null), [])
  return { target, loadingMediaID, open, close }
}
