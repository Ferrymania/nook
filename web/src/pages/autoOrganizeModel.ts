import type { Setting } from '../types'

export type AutoOrganizeConfig = {
  enabled: string
  scrapeAfter: string
  smartClassify: string
  autoAddLibrary: string
  sourceDir: string
  targetDir: string
  transferMode: string
  intervalSeconds: string
  movieFormat: string
  tvFormat: string
  animeFormat: string
}

export const AUTO_ORGANIZE_DEFAULTS: AutoOrganizeConfig = {
  enabled: 'false',
  scrapeAfter: 'true',
  smartClassify: 'true',
  autoAddLibrary: 'true',
  sourceDir: '',
  targetDir: '',
  transferMode: 'hardlink',
  intervalSeconds: '300',
  movieFormat: '{title} ({year})/{title} ({year})',
  tvFormat: '{title} ({year})/Season {season:02}/{title} S{season:02}E{episode:02}',
  animeFormat: '{title}/Season {season:02}/{title} S{season:02}E{episode:02}',
}

export const AUTO_ORGANIZE_KEYS: Record<keyof AutoOrganizeConfig, string> = {
  enabled: 'organize.auto',
  scrapeAfter: 'organize.scrape_after',
  smartClassify: 'organizer.smart_classify',
  autoAddLibrary: 'organize.auto_add_library',
  sourceDir: 'organize.source_dir',
  targetDir: 'organize.target_dir',
  transferMode: 'organize.transfer_mode',
  intervalSeconds: 'organize.interval_seconds',
  movieFormat: 'organize.movie_format',
  tvFormat: 'organize.tv_format',
  animeFormat: 'organize.anime_format',
}

export type AutoOrganizeTab = 'basic' | 'naming'

export function mergeAutoOrganizeSettings(rows: Setting[]): AutoOrganizeConfig {
  const idx = settingIndex(rows)
  return {
    enabled: idx[AUTO_ORGANIZE_KEYS.enabled] ?? AUTO_ORGANIZE_DEFAULTS.enabled,
    scrapeAfter: idx[AUTO_ORGANIZE_KEYS.scrapeAfter] ?? AUTO_ORGANIZE_DEFAULTS.scrapeAfter,
    smartClassify: idx[AUTO_ORGANIZE_KEYS.smartClassify] ?? AUTO_ORGANIZE_DEFAULTS.smartClassify,
    autoAddLibrary: idx[AUTO_ORGANIZE_KEYS.autoAddLibrary] ?? AUTO_ORGANIZE_DEFAULTS.autoAddLibrary,
    sourceDir: idx[AUTO_ORGANIZE_KEYS.sourceDir] ?? AUTO_ORGANIZE_DEFAULTS.sourceDir,
    targetDir: idx[AUTO_ORGANIZE_KEYS.targetDir] ?? AUTO_ORGANIZE_DEFAULTS.targetDir,
    transferMode: idx[AUTO_ORGANIZE_KEYS.transferMode] ?? AUTO_ORGANIZE_DEFAULTS.transferMode,
    intervalSeconds: idx[AUTO_ORGANIZE_KEYS.intervalSeconds] ?? AUTO_ORGANIZE_DEFAULTS.intervalSeconds,
    movieFormat: idx[AUTO_ORGANIZE_KEYS.movieFormat] ?? AUTO_ORGANIZE_DEFAULTS.movieFormat,
    tvFormat: idx[AUTO_ORGANIZE_KEYS.tvFormat] ?? AUTO_ORGANIZE_DEFAULTS.tvFormat,
    animeFormat: idx[AUTO_ORGANIZE_KEYS.animeFormat] ?? AUTO_ORGANIZE_DEFAULTS.animeFormat,
  }
}

export function settingOn(value: string): boolean {
  return ['1', 'true', 'yes', 'on', 'enabled', '启用', '开启'].includes(value.trim().toLowerCase())
}

function settingIndex(rows: Setting[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const row of rows) out[row.key] = row.value
  return out
}
