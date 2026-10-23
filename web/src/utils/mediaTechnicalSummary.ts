import type { Media } from '../types/media'

type TechnicalMedia = Pick<Media, 'duration_sec' | 'width' | 'height' | 'size_bytes' | 'video_metadata'>

export function mediaTechnicalSummary(media: TechnicalMedia): string[] {
  const { duration_sec: seconds, width, height, size_bytes: bytes, video_metadata: video } = media
  const values: string[] = []
  if (positive(seconds)) {
    const hours = Math.floor(seconds / 3600)
    const minutes = Math.floor(seconds % 3600 / 60)
    values.push(hours > 0 ? `${hours}h${minutes > 0 ? `${minutes}m` : ''}` : minutes > 0 ? `${minutes}m` : `${Math.floor(seconds)}s`)
  }
  if (positive(width) && positive(height)) {
    const long = Math.max(width, height)
    const short = Math.min(width, height)
    values.push(long >= 7680 || short >= 4320 ? '8K'
      : long >= 3840 || short >= 2160 ? '4K'
      : long >= 2560 || short >= 1440 ? '1440p'
      : long >= 1920 || short >= 1080 ? '1080p'
      : long >= 1280 || short >= 720 ? '720p' : 'SD')
  }
  const range = videoRangeLabel(video)
  if (range) values.push(range)
  if (positive(video?.frame_rate)) values.push(`${Number(video.frame_rate.toFixed(3))}FPS`)
  if (positive(bytes)) {
    const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
    const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
    values.push(`${Number((bytes / 1024 ** index).toFixed(index === 0 ? 0 : 2))}${units[index]}`)
  }
  return values
}

function videoRangeLabel(video: Media['video_metadata']): string {
  switch (video?.extended_video_type) {
    case 'DolbyVision': {
      const profile = /^DoviProfile(\d)(\d)$/.exec(video.extended_video_sub_type ?? '')
      return profile ? `DV(P${profile[1]}.${profile[2]})` : 'DV'
    }
    case 'Hdr10Plus': return 'HDR10+'
    case 'Hdr10': return 'HDR10'
    case 'HyperLogGamma': return 'HLG'
    default: return ''
  }
}

function positive(value: number | undefined): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
}
