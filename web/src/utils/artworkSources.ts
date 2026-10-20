import type { Media } from '../types/media'

export function artworkSources(media: Pick<Media, 'poster_url' | 'backdrop_url' | 'episode_still_url'>, kind: 'poster' | 'episode'): string[] {
  const candidates = kind === 'episode'
    ? [media.episode_still_url, media.backdrop_url, media.poster_url]
    : [media.poster_url, media.backdrop_url]
  return [...new Set(candidates.map(value => value?.trim()).filter((value): value is string => Boolean(value)))]
}

// The image proxy deliberately answers failed reads with an HTTP 200 1×1 PNG.
// onError alone cannot detect this successful-but-empty response.
export function isMissingArtwork(width: number, height: number): boolean {
  return width <= 1 && height <= 1
}
