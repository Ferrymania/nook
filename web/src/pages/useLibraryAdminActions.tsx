import { useState, type ReactNode } from 'react'
import toast from 'react-hot-toast'

import { api } from '../api/client'
import { mediaAPI } from '../api/library'
import { confirmActionResult } from '../components/confirmAction'
import { useEpisodeArtworkPreference } from '../hooks/useEpisodeArtworkPreference'
import type { Media } from '../types'
import { seriesTitle, type SeriesCard } from '../utils/groupSeries'
import { LibraryMovieActions } from './LibraryMovieActions'
import { useLibraryScrapeStatus } from './useLibraryScrapeStatus'

type UseLibraryAdminActionsOptions = {
  libraryID: string
  role?: string
  selectedSeries: SeriesCard | null
  selectedSeriesEpisodes: Media[]
  reloadCurrentLibrary: () => void
  clearSelectedSeries: () => void
  onManualScrape: (media: Media) => void
  manualScrapeLoadingID: string
  onMovieMetadataEdit: (media: Media) => void
  onMovieQuerySegments: (media: Media) => void
}

export function useLibraryAdminActions({
  libraryID,
  role,
  selectedSeries,
  selectedSeriesEpisodes,
  reloadCurrentLibrary,
  clearSelectedSeries,
  onManualScrape,
  manualScrapeLoadingID,
  onMovieMetadataEdit,
  onMovieQuerySegments,
}: UseLibraryAdminActionsOptions) {
  const [scrapeEpisodeArtwork] = useEpisodeArtworkPreference()
  const [seriesToolBusy, setSeriesToolBusy] = useState('')
  const [movieToolBusy, setMovieToolBusy] = useState('')
  const { scraping, handleScrape } = useLibraryScrapeStatus({ libraryID, isAdmin: role === 'admin', episodeArtwork: scrapeEpisodeArtwork, onLibraryChanged: reloadCurrentLibrary })

  const runSeriesTool = async (key: string, label: string, action: (media: Media) => Promise<unknown>) => {
    if (selectedSeriesEpisodes.length === 0) return
    setSeriesToolBusy(key)
    try {
      for (const ep of selectedSeriesEpisodes) {
        await action(ep)
      }
      toast.success(`${label}完成：${selectedSeriesEpisodes.length} 个媒体`)
      reloadCurrentLibrary()
    } catch (err: unknown) {
      const serverErr = (err as { response?: { data?: { error?: string } } })?.response?.data?.error
      const msg = serverErr || (err instanceof Error ? err.message : '')
      toast.error(msg ? `${label}失败：${msg}` : `${label}失败`)
    } finally {
      setSeriesToolBusy('')
    }
  }

  const handleSeriesSmartScrape = () => {
    runSeriesTool('scrape', '整剧智能刮削', (media) =>
      api.post(`/media/${media.id}/scrape`, smartScrapeOptions(scrapeEpisodeArtwork)),
    )
  }

  const handleSeriesProbe = () => {
    runSeriesTool('probe', '整剧媒体轨探测', (media) => api.post(`/media/${media.id}/probe`))
  }

  const handleSeriesNFO = () => {
    runSeriesTool('nfo', '整剧 NFO 写出', (media) => mediaAPI.exportNFO(media.id))
  }

  const handleSeriesDelete = async () => {
    if (!selectedSeries || selectedSeriesEpisodes.length === 0) return
    const result = await confirmActionResult({
      title: '删除整剧',
      message: `确定从媒体库删除「${seriesTitle(selectedSeries.rep)}」的 ${selectedSeriesEpisodes.length} 个媒体？\n默认仅移除索引记录，磁盘文件会保留。`,
      confirmText: '删除',
      checkboxLabel: '同时删除本地文件（含同名 NFO）',
    })
    if (!result.confirmed) return
    await runSeriesTool('delete', '整剧删除', (media) => mediaAPI.delete(media.id, { deleteFiles: result.checked }))
    clearSelectedSeries()
  }

  const runMovieTool = async (media: Media, key: string, label: string, action: (media: Media) => Promise<unknown>) => {
    const busyKey = `${key}:${media.id}`
    setMovieToolBusy(busyKey)
    try {
      await action(media)
      toast.success(`${label}完成：${media.title}`)
      reloadCurrentLibrary()
    } catch (err: unknown) {
      const serverErr = (err as { response?: { data?: { error?: string } } })?.response?.data?.error
      const msg = serverErr || (err instanceof Error ? err.message : '')
      toast.error(msg ? `${label}失败：${msg}` : `${label}失败`)
    } finally {
      setMovieToolBusy('')
    }
  }

  const handleMovieSmartScrape = (media: Media) => {
    runMovieTool(media, 'scrape', '智能刮削', (item) =>
      api.post(`/media/${item.id}/scrape`, smartScrapeOptions(scrapeEpisodeArtwork)),
    )
  }

  const handleMovieProbe = (media: Media) => {
    runMovieTool(media, 'probe', '媒体轨探测', (item) => api.post(`/media/${item.id}/probe`))
  }

  const handleMovieNFO = (media: Media) => {
    runMovieTool(media, 'nfo', 'NFO 写出', (item) => mediaAPI.exportNFO(item.id))
  }

  const handleMovieDelete = async (media: Media) => {
    const result = await confirmActionResult({
      title: '删除媒体',
      message: `确定从媒体库删除「${media.title}」？\n默认仅移除索引记录，磁盘文件会保留。`,
      confirmText: '删除',
      checkboxLabel: '同时删除本地文件（含同名 NFO）',
    })
    if (!result.confirmed) return
    await runMovieTool(media, 'delete', '删除', (item) => mediaAPI.delete(item.id, { deleteFiles: result.checked }))
  }

  const movieActions = (media: Media): ReactNode => {
    if (role !== 'admin') return undefined
    return (
      <LibraryMovieActions
        media={media}
        busy={movieToolBusy.endsWith(`:${media.id}`) || manualScrapeLoadingID === media.id}
        onSmartScrape={handleMovieSmartScrape}
        onManualScrape={onManualScrape}
        onMetadataEdit={onMovieMetadataEdit}
        onProbe={handleMovieProbe}
        onNFO={handleMovieNFO}
        onQuerySegments={onMovieQuerySegments}
        onDelete={handleMovieDelete}
      />
    )
  }

  return {
    scraping,
    scrapeEpisodeArtwork,
    seriesToolBusy,
    handleScrape,
    handleSeriesSmartScrape,
    handleSeriesProbe,
    handleSeriesNFO,
    handleSeriesDelete,
    movieActions,
  }
}

function smartScrapeOptions(episodeImages: boolean) {
  return {
    episode_images: episodeImages,
    refresh_matched: true,
    include_matched: true,
  }
}
