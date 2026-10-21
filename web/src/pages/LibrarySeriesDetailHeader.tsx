import { isRemoteEmbyID } from '../utils/remoteEmby'
import { Link } from 'react-router-dom'
import { ArrowLeft, Play } from 'lucide-react'

import { ARTWORK, imageURL } from '../api/client'
import { ArtworkImage } from '../components/ArtworkImage'
import { artworkSources } from '../utils/artworkSources'
import { ExternalPlayerButton } from '../components/ExternalPlayerButton'
import { MediaFavouriteButton } from '../components/MediaFavouriteButton'
import { MediaManagementPanel } from '../components/MediaManagementPanel'
import { MediaSegmentQueryButton } from '../components/MediaSegmentQueryButton'
import type { Media } from '../types'
import { expandSeriesMediaVersions, isTheatricalFeature, seriesTitle, type SeriesCard } from '../utils/groupSeries'
import { episodeNumberValue } from '../utils/episodeNumber'

type LibrarySeriesDetailHeaderProps = {
  series: SeriesCard
  allEpisodes: Media[]
  playbackFrom: string
  isAdmin: boolean
  canFavorite: boolean
  favourite: boolean
  favouriteBusy: boolean
  onToggleFavourite: () => void
  seriesToolBusy: string
  onBack: () => void
  onSmartScrape: () => void
  onManualScrape: () => void
  onMetadataEdit: () => void
  onProbe: () => void
  onNFO: () => void
  onOrganize: () => void
  onDelete: () => void
}

export function LibrarySeriesDetailHeader({
  series,
  allEpisodes,
  playbackFrom,
  isAdmin,
  canFavorite,
  favourite,
  favouriteBusy,
  onToggleFavourite,
  seriesToolBusy,
  onBack,
  onSmartScrape,
  onManualScrape,
  onMetadataEdit,
  onProbe,
  onNFO,
  onOrganize,
  onDelete,
}: LibrarySeriesDetailHeaderProps) {
  const tvEpisodes = allEpisodes.filter((media) => !isTheatricalFeature(media))
  const queryEpisodes = expandSeriesMediaVersions(tvEpisodes)
  const theatricalCount = allEpisodes.length - tvEpisodes.length
  const firstEpisode = firstPlayableEpisode(tvEpisodes)

  return (
    <>
      <div className="flex items-center gap-4">
        <button onClick={onBack} className="btn-ghost gap-2">
          <ArrowLeft size={16} />
          返回列表
        </button>
        <h1 className="min-w-0 font-display text-2xl font-semibold text-ink-600">
          {seriesTitle(series.rep)}
        </h1>
        <span className="text-sm text-sand-500">
          共 {tvEpisodes.length} 集{theatricalCount > 0 ? ` · ${theatricalCount} 部剧场版` : ''}
        </span>
      </div>

      <div className="flex flex-col gap-6 sm:flex-row">
        <div className="w-40 shrink-0 overflow-hidden rounded-xl bg-sand-200 shadow-card">
          <ArtworkImage
            sources={artworkSources(series.rep, 'poster').map(source => imageURL(source, series.rep.updated_at, ARTWORK.posterDetail))}
            alt={series.rep.title}
            loading="lazy"
            decoding="async"
            className="aspect-[2/3] w-full object-cover"
            fallbackClassName="aspect-[2/3] w-full"
            referrerPolicy="no-referrer"
          />
        </div>
        <div className="flex-1 space-y-3">
          <p className="text-sm leading-relaxed text-ink-50">
            {series.rep.overview || '暂无简介'}
          </p>

          <div className="flex flex-wrap gap-2">
            {firstEpisode && (
              <>
                <Link to={`/play/${firstEpisode.id}`} state={{ from: playbackFrom }} className="btn-primary inline-flex">
                  <Play size={16} fill="currentColor" />
                  从第一集开始播放
                </Link>
                <ExternalPlayerButton mediaId={firstEpisode.id} label="外部播放器播放" />
              </>
            )}
            {canFavorite && (
              <MediaFavouriteButton
                favourite={favourite}
                disabled={favouriteBusy}
                onToggle={onToggleFavourite}
              />
            )}
          </div>

          {isAdmin && allEpisodes.length > 0 && !isRemoteEmbyID(series.rep.id) && (
            <MediaManagementPanel
              scope={`整剧 · ${expandSeriesMediaVersions(allEpisodes).length} 个媒体`}
              busy={seriesToolBusy}
              manualMatchLabel={theatricalCount > 0 ? '手动匹配 TV 版' : '手动匹配'}
              onSmartScrape={onSmartScrape}
              onManualScrape={onManualScrape}
              onMetadataEdit={onMetadataEdit}
              onProbe={onProbe}
              onExportNFO={onNFO}
              onOrganize={onOrganize}
              onDelete={onDelete}
              segmentAction={<MediaSegmentQueryButton key={series.rep.id} media={series.rep} episodes={queryEpisodes} disabled={!!seriesToolBusy} />}
            />
          )}
        </div>
      </div>
    </>
  )
}

function firstPlayableEpisode(episodes: Media[]): Media | null {
  const sorted = [...episodes]
  sorted.sort((a, b) =>
    (a.season_num || 0) - (b.season_num || 0)
    || episodeNumberValue(a) - episodeNumberValue(b),
  )
  return sorted[0] ?? null
}
