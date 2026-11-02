import { api } from './client'
import type { ScrapeOptions } from './library'

// Administrative tools for inspecting media utilities and repairing metadata.
export interface FFmpegToolInfo {
  installed: boolean
  path?: string
  version?: string
}

export interface FFmpegToolPair {
  ffmpeg?: FFmpegToolInfo
  ffprobe?: FFmpegToolInfo
}

export const toolsAPI = {
  // ffToolsStatus 查询当前工具的可用性、路径和版本。
  ffToolsStatus: (signal?: AbortSignal) =>
    api.get<FFmpegToolPair>('/admin/tools/ffmpeg/status', { signal }).then((r) => r.data),

  detectFFTools: (signal?: AbortSignal) =>
    api.get<FFmpegToolPair>('/admin/tools/ffmpeg/detect', { signal }).then((r) => r.data),

  // repairAndRescrapeAll 触发「全库修复+重刮」：先从媒体路径中的
  // {tmdb-N}/{bangumi-N} 占位符回填缺失/错误的外部 ID，再批量重刮整库。
  // 后端异步执行，立即返回；进度通过 WS "scrape" topic 推送。
  repairAndRescrapeAll: (options?: ScrapeOptions) =>
    api
      .post<{ status: string }>('/admin/media/repair-rescrape', options ?? {})
      .then((r) => r.data),

  // repairAndRescrapeLibrary 触发「单库修复+重刮」：只对指定媒体库回填占位符
  // 外部 ID 并重刮，不影响其它库。后端异步执行，进度通过 WS "scrape" topic 推送。
  repairAndRescrapeLibrary: (libraryID: string, options?: ScrapeOptions) =>
    api
      .post<{ status: string }>(`/admin/libraries/${libraryID}/repair-rescrape`, options ?? {})
      .then((r) => r.data),
}
