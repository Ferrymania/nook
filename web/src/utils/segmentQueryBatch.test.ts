import { querySegmentBatch, segmentQueryTargets, type SegmentBatchResult } from './segmentQueryBatch'

function check(label: string, actual: unknown, expected: unknown) {
  if (JSON.stringify(actual) !== JSON.stringify(expected)) throw new Error(`${label}: got ${JSON.stringify(actual)}, want ${JSON.stringify(expected)}`)
}
const media = [
  { id: 'later', title: '后集', season_num: 2, episode_num: 1 },
  { id: 'fraction', title: '特别篇', season_num: 1, episode_num: 1, episode_fraction: 0.5 },
  { id: 'first', title: '第一集', season_num: 1, episode_num: 1 },
  { id: 'first', title: '第一集', season_num: 1, episode_num: 1 },
]

async function main() {
  check('one query per media in season and fractional episode order', segmentQueryTargets(media).map(row => row.id), ['first', 'fraction', 'later'])
  const calls: string[] = [], snapshots: SegmentBatchResult[][] = []
  let active = 0, maxActive = 0
  const rows = await querySegmentBatch(media, async id => {
    calls.push(id)
    maxActive = Math.max(maxActive, ++active)
    await Promise.resolve()
    active--
    if (id === 'fraction') throw { response: { data: { error: '缺少季集信息，请先完成刮削' } } }
    return { found: id === 'first' }
  }, new AbortController().signal, next => snapshots.push(next))
  check('no parallel provider requests', maxActive, 1)
  check('failed episode does not prevent later queries', calls, ['first', 'fraction', 'later'])
  check('found, missing and failures stay distinct', rows.map(row => row.status), ['found', 'failed', 'missing'])
  check('actionable API error survives', rows[1].error, '缺少季集信息，请先完成刮削')
  check('progress snapshots do not mutate after publication', snapshots.map(snapshot => snapshot.length), [1, 2, 3])

  const cancel = new AbortController(), canceledCalls: string[] = []
  await querySegmentBatch(media, async id => { canceledCalls.push(id); return { found: true } }, cancel.signal, () => cancel.abort())
  check('closing stops unstarted queries', canceledCalls, ['first'])

  const inFlight = new AbortController()
  const interrupted = await querySegmentBatch(media, async () => { inFlight.abort(); throw new Error('request aborted') }, inFlight.signal, () => { throw new Error('aborted response published') })
  check('aborted response does not become a failed episode', interrupted, [])
}
void main().catch(error => { throw error })
