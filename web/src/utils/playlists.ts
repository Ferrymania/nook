import type { Media } from '../types'
import { formatEpisodeLabel } from './episodeNumber'

export function playlistErrorMessage(error: unknown): string {
  return (error as { response?: { data?: { error?: string } } })?.response?.data?.error || '操作失败，请重试'
}
export function playlistMediaMeta(media: Media): string {
  return [media.year || '', media.episode_num > 0 ? formatEpisodeLabel(media) : '', media.episode_title || ''].filter(Boolean).join(' · ')
}
