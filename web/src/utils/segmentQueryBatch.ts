import type { Media } from '../types/media'
import { episodeNumberValue } from './episodeNumber'

export type SegmentQueryTarget = Pick<Media, 'id' | 'title' | 'season_num' | 'episode_num' | 'episode_fraction'>
export type SegmentBatchResult = { media: SegmentQueryTarget; status: 'found' | 'missing' | 'failed'; error?: string }

export function segmentQueryTargets(media: SegmentQueryTarget[]): SegmentQueryTarget[] {
  return [...new Map(media.filter(item => item.id).map(item => [item.id, item])).values()]
    .sort((a, b) => (a.season_num - b.season_num) || episodeNumberValue(a) - episodeNumberValue(b))
}

// Use the existing per-media query API so its validation, saved timestamps and
// task history remain authoritative. Never fan out a whole season in parallel.
export async function querySegmentBatch(
  targets: SegmentQueryTarget[],
  query: (mediaID: string, signal: AbortSignal) => Promise<{ found: boolean }>,
  signal: AbortSignal,
  onProgress: (results: SegmentBatchResult[]) => void,
): Promise<SegmentBatchResult[]> {
  const results: SegmentBatchResult[] = []
  for (const media of segmentQueryTargets(targets)) {
    if (signal.aborted) break
    let row: SegmentBatchResult
    try {
      const result = await query(media.id, signal)
      if (signal.aborted) break
      row = { media, status: result.found ? 'found' : 'missing' }
    } catch (error: unknown) {
      if (signal.aborted) break
      row = { media, status: 'failed', error: segmentQueryError(error) }
    }
    results.push(row)
    onProgress([...results])
  }
  return results
}

export function segmentQueryError(error: unknown): string {
  return (error as { response?: { data?: { error?: string } } })?.response?.data?.error
    || '查询失败，请检查网络后重试；已有时间点保留'
}
