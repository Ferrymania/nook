import { ManualScrapeDialog } from '../components/ManualScrapeDialog'
import { MetadataEditDialog } from '../components/MetadataEditDialog'
import { OrganizeMediaDialog } from '../components/OrganizeMediaDialog'
import { MediaSegmentQueryDialog } from '../components/MediaSegmentQueryButton'
import type { Media } from '../types'
import { isTheatricalFeature, seriesTitle, type SeriesCard } from '../utils/groupSeries'

export type LibraryMovieDialog = { kind: 'metadata' | 'organize' | 'segments'; media: Media } | null

type LibraryPageDialogsProps = {
  manualSeriesScrapeOpen: boolean
  seriesMetadataEditOpen: boolean
  manualMovie: Media | null
  movieDialog: LibraryMovieDialog
  selectedSeries: SeriesCard | null
  selectedSeriesScrapeMedia: Media | null
  selectedSeriesMediaIDs: string[]
  libraryType?: string
  scrapeEpisodeArtwork: boolean
  onCloseManualSeriesScrape: () => void
  onCloseSeriesMetadataEdit: () => void
  onCloseManualMovie: () => void
  onCloseMovieDialog: () => void
  onApplied: () => void
}

export function LibraryPageDialogs({
  manualSeriesScrapeOpen,
  seriesMetadataEditOpen,
  manualMovie,
  movieDialog,
  selectedSeries,
  selectedSeriesScrapeMedia,
  selectedSeriesMediaIDs,
  libraryType,
  scrapeEpisodeArtwork,
  onCloseManualSeriesScrape,
  onCloseSeriesMetadataEdit,
  onCloseManualMovie,
  onCloseMovieDialog,
  onApplied,
}: LibraryPageDialogsProps) {
  const selectedSeriesTitle = selectedSeries ? seriesTitle(selectedSeries.rep) : ''

  return (
    <>
      {movieDialog?.kind === 'metadata' && <MetadataEditDialog open media={movieDialog.media} onClose={onCloseMovieDialog} onSaved={onApplied} />}
      {movieDialog?.kind === 'organize' && <OrganizeMediaDialog open media={movieDialog.media} onClose={onCloseMovieDialog} onOrganized={onApplied} />}
      {movieDialog?.kind === 'segments' && <MediaSegmentQueryDialog media={movieDialog.media} onClose={onCloseMovieDialog} />}
      <ManualScrapeDialog
        open={manualSeriesScrapeOpen}
        media={selectedSeriesScrapeMedia}
        mediaIds={selectedSeriesMediaIDs}
        defaultQuery={selectedSeriesTitle}
        mediaType="tv"
        scopeLabel={selectedSeriesTitle || '当前剧集'}
        sourcePath={selectedSeries?.rep.path}
        episodeArtwork={scrapeEpisodeArtwork}
        onClose={onCloseManualSeriesScrape}
        onApplied={onApplied}
      />
      <MetadataEditDialog
        open={seriesMetadataEditOpen}
        media={selectedSeries?.rep ?? null}
        mediaIds={selectedSeriesMediaIDs}
        mode="series"
        scopeLabel={selectedSeriesTitle || '当前剧集'}
        onClose={onCloseSeriesMetadataEdit}
        onSaved={onApplied}
      />
      <ManualScrapeDialog
        open={!!manualMovie}
        media={manualMovie}
        defaultQuery={manualMovie?.title ?? ''}
        mediaType={manualMovie ? scrapeMediaType(libraryType, manualMovie) : libraryType || 'movie'}
        scopeLabel={manualMovie?.title ?? '当前电影'}
        episodeArtwork={scrapeEpisodeArtwork}
        onClose={onCloseManualMovie}
        onApplied={onApplied}
      />
    </>
  )
}

function scrapeMediaType(libraryType: string | undefined, media: Media): string {
  if (isTheatricalFeature(media)) {
    return 'movie'
  }
  if ((media.season_num ?? 0) > 0 || (media.episode_num ?? 0) > 0) {
    return 'tv'
  }
  return libraryType || 'movie'
}
