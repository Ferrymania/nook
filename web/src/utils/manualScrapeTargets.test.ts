import type { Media } from '../types'
import { applyManualScrapeTargets, manualScrapeTargetIds, seriesManualScrapeTarget } from './manualScrapeTargets'

function check(label: string, actual: unknown, expected: unknown) {
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error(`${label}: got ${JSON.stringify(actual)}, want ${JSON.stringify(expected)}`)
  }
}

function episode(id: string, season = 1, number = 1): Media {
  return { id, title: '大明王朝1566', season_num: season, episode_num: number,
    path: `/media/电视剧/大明王朝1566/Season ${season}/S${season}E${number}.strm` } as Media
}

async function main() {
  const first = episode('first')
  const alternative = episode('alternative')
  const laterSeason = episode('later-season', 2)
  const special = episode('special', 0)
  const movie = { ...episode('movie'), title: '剧场版', path: '/media/电视剧/大明王朝1566/剧场版/电影.mkv' }
  const series = { key: 'series:test', rep: first, count: 46 }
  const target = seriesManualScrapeTarget(series, [
    { ...first, versions: [first, alternative] }, laterSeason, special, first, movie,
  ])
  check('all seasons, specials and versions, without duplicate IDs or theatrical movies',
    target.mediaIds, ['first', 'alternative', 'later-season', 'special'])
  check('manual search keeps the series title', target.title, first.title)
  check('single episode stays a single target', manualScrapeTargetIds(laterSeason), ['later-season'])
  check('explicit empty series never falls back to first episode', manualScrapeTargetIds(first, []), [])
  check('batch IDs are deduplicated', manualScrapeTargetIds(first, ['later-season', '', 'later-season']), ['later-season'])
  for (const items of [[], [movie]]) {
    let rejected = false
    try { seriesManualScrapeTarget(series, items) } catch { rejected = true }
    check('empty episode response cannot open a representative-only series match', rejected, true)
  }

  const calls: unknown[] = []
  const partial = { applied: 3, errors: ['special: 获取元数据失败'] }
  const applyOne = async (id: string) => { calls.push(['single', id]) }
  const applyBatch = async (ids: string[]) => { calls.push(['batch', ids]); return partial }
  const result = await applyManualScrapeTargets(target.mediaIds, applyOne, applyBatch)
  check('series uses one batch request with every resolved ID', calls, [['batch', target.mediaIds]])
  check('partial failures survive the batch response', result, partial)
  calls.length = 0
  await applyManualScrapeTargets(['later-season'], applyOne, applyBatch)
  check('single request uses its actual target ID', calls, [['single', 'later-season']])
  calls.length = 0
  let emptyRejected = false
  try { await applyManualScrapeTargets([], applyOne, applyBatch) } catch { emptyRejected = true }
  check('empty batch fails before making any request', [emptyRejected, calls], [true, []])
  let failure = ''
  try {
    await applyManualScrapeTargets(target.mediaIds, applyOne, async () => { throw new Error('连接失败') })
  } catch (err) { failure = (err as Error).message }
  check('batch failure does not retry as a single request', [failure, calls], ['连接失败', []])
  console.log('manualScrapeTargets tests passed')
}

void main().catch(error => { throw error })
