import { libraryAPI, mediaAPI, type SeriesEpisodesPage } from '../api/library'
import type { Media } from '../types'
import { groupSeries, type SeriesCard } from './groupSeries'
import { isRemoteEmbyID } from './remoteEmby'

/**
 * 按 series key 反解出剧集卡片。
 *
 * 首页「最新条目」、海报墙等入口点击后带 ?series=<key> 深链进媒体库页，
 * 但媒体库页每次只加载一页剧集卡片（远程 Emby 库 100 条），目标剧集不在
 * 这一页时列表里找不到对应卡片，页面就退化成整个媒体库列表。这里按 key
 * 单独解析目标剧集，与分页无关：
 *
 * - 远程 Emby 的 key 本身就是（伪装后的）剧集 ID，直接取该条目详情；
 * - 本地优先使用服务端完整分组卡片；key 变化时按固定媒体 ID 定位当前归属。
 *
 * key 失效（剧集已删除等）时返回 null，调用方保持"整库列表"原状态。
 */
export async function resolveSeriesCardByKey(libraryID: string, key: string, mediaID = ''): Promise<SeriesCard | null> {
  if (!libraryID || (!key && !mediaID)) return null

  if (isRemoteEmbyID(key)) {
    const rep = await fetchRemoteSeriesMedia(key)
    if (rep) {
      return { key, rep, linkMedia: rep, count: 0, is_series: true }
    }
  }

  const page = await fetchSeriesPage(libraryID, key, mediaID)
  if (page?.card) return page.card
  const episodes = page?.items ?? []
  if (episodes.length === 0) return null
  // 与列表页同一套折叠规则，选出海报/元数据最好的那一集做代表卡片。
  const [grouped] = groupSeries(episodes)
  const rep = grouped?.rep ?? episodes[0]
  return {
    key,
    rep,
    linkMedia: grouped?.linkMedia ?? rep,
    count: episodes.length,
    is_series: true,
  }
}

async function fetchRemoteSeriesMedia(id: string): Promise<Media | null> {
  try {
    const media = await mediaAPI.get(id)
    return media?.id ? media : null
  } catch {
    return null
  }
}

async function fetchSeriesPage(libraryID: string, key: string, mediaID: string): Promise<SeriesEpisodesPage | null> {
  try {
    return await libraryAPI.listSeriesEpisodes(libraryID, key, mediaID)
  } catch {
    return null
  }
}
