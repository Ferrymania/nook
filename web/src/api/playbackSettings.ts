import { api } from './client'
import type { Library } from '../types'

export interface PlaybackSettings {
  content_rating_limit: string
  allow_adult: boolean
  preferred_subtitle_lang: string
  preferred_audio_lang: string
  autoplay_next: boolean
  skip_intro: boolean
  allowed_library_ids: string[]
}

export const playbackSettingsAPI = {
  get: () => api.get<PlaybackSettings>('/me/playback-settings').then((r) => r.data),
  save: (settings: PlaybackSettings, password?: string) =>
    api.put<PlaybackSettings>('/me/playback-settings', { ...settings, password }).then((r) => r.data),
  libraries: () => api.get<Library[]>('/me/playback-settings/libraries').then((r) => r.data),
}
