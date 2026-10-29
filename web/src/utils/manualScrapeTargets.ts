import type { Media } from '../types'
import { expandSeriesMediaVersions, isTheatricalFeature, seriesTitle, type SeriesCard } from './groupSeries'

export interface SeriesManualScrapeTarget {
  media: Media
  mediaIds: string[]
  title: string
  sourcePath: string
}

export function manualScrapeTargetIds(media: Media | null, mediaIds?: string[]): string[] {
  // An explicit empty batch must never fall back to the representative episode.
  return Array.from(new Set((mediaIds ?? (media ? [media.id] : [])).filter(Boolean)))
}

export function seriesManualScrapeTarget(series: SeriesCard, episodes: Media[]): SeriesManualScrapeTarget {
  const targets = expandSeriesMediaVersions(episodes).filter(media => !isTheatricalFeature(media))
  if (targets.length === 0) throw new Error('没有可手动匹配的剧集，请刷新后重试')
  return {
    media: targets[0],
    mediaIds: manualScrapeTargetIds(null, targets.map(media => media.id)),
    title: seriesTitle(series.rep),
    sourcePath: series.rep.path,
  }
}

export async function applyManualScrapeTargets(
  targetIds: string[],
  applyOne: (id: string) => Promise<unknown>,
  applyBatch: (ids: string[]) => Promise<{ applied: number; errors?: string[] }>,
): Promise<{ applied: number; errors?: string[] }> {
  if (targetIds.length === 0) throw new Error('没有可手动匹配的媒体，请刷新后重试')
  if (targetIds.length > 1) return applyBatch(targetIds)
  await applyOne(targetIds[0])
  return { applied: 1 }
}
