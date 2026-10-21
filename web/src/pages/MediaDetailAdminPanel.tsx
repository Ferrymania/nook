import { EpisodeArtworkToggle } from '../components/EpisodeArtworkToggle'
import { MediaManagementPanel } from '../components/MediaManagementPanel'
import { MediaSegmentQueryButton } from '../components/MediaSegmentQueryButton'
import type { Media } from '../types'
import { isRemoteEmbyID } from '../utils/remoteEmby'

type MediaDetailAdminPanelProps = {
  media: Media
  scrapeEpisodeArtwork: boolean
  onScrapeEpisodeArtworkChange: (checked: boolean) => void
  onSmartScrape: () => void
  onManualScrape: () => void
  onMetadataEdit: () => void
  onOrganize: () => void
  onProbe: () => void
  onExportNFO: () => void
  onDelete: () => void
}

export function MediaDetailAdminPanel({
  media,
  scrapeEpisodeArtwork,
  onScrapeEpisodeArtworkChange,
  onSmartScrape,
  onManualScrape,
  onMetadataEdit,
  onOrganize,
  onProbe,
  onExportNFO,
  onDelete,
}: MediaDetailAdminPanelProps) {
  if (isRemoteEmbyID(media.id)) {
    return null
  }

  return (
    <MediaManagementPanel
      scope={isEpisodeArtworkTarget(media) ? '当前单集' : '当前影片'}
      headerActions={isEpisodeArtworkTarget(media) && (
        <EpisodeArtworkToggle
          checked={scrapeEpisodeArtwork}
          onChange={onScrapeEpisodeArtworkChange}
          title="关闭后仍会获取每集简介、评分和时长，只跳过单集图片"
          className="h-9 text-xs"
        />
      )}
      onSmartScrape={onSmartScrape}
      onManualScrape={onManualScrape}
      onMetadataEdit={onMetadataEdit}
      onProbe={onProbe}
      onExportNFO={onExportNFO}
      onOrganize={onOrganize}
      onDelete={onDelete}
      segmentAction={<MediaSegmentQueryButton key={media.id} media={media} />}
    />
  )
}

function isEpisodeArtworkTarget(media: Media): boolean {
  return media.season_num > 0 || media.episode_num > 0
}
