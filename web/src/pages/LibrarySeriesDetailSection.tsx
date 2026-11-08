import { AnimatePresence, motion } from 'framer-motion'

import type { Media } from '../types'
import type { SeriesCard } from '../utils/groupSeries'
import { LibrarySeriesDetailHeader } from './LibrarySeriesDetailHeader'
import { LibrarySeriesEpisodes } from './LibrarySeriesEpisodes'

type SeasonEpisodes = {
  season: number
  episodes: Media[]
}

type LibrarySeriesDetailSectionProps = {
  selectedSeries: SeriesCard | null
  selectedEpisodes: SeasonEpisodes[]
  selectedSeason: number | null
  visibleEpisodes: Media[]
  allEpisodes: Media[]
  loadingEpisodes: boolean
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
  onDelete: () => void
  onDeleteSeason: (episodes: Media[]) => void
  onDeleteEpisode: (episode: Media) => void
  onSeasonChange: (season: number) => void
  onManualScrapeMedia: (media: Media) => void
}

export function LibrarySeriesDetailSection({
  selectedSeries,
  selectedEpisodes,
  selectedSeason,
  visibleEpisodes,
  allEpisodes,
  loadingEpisodes,
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
  onDelete,
  onDeleteSeason,
  onDeleteEpisode,
  onSeasonChange,
  onManualScrapeMedia,
}: LibrarySeriesDetailSectionProps) {
  return (
    <AnimatePresence mode="wait">
      {selectedSeries && (
        <motion.div
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0 }}
          className="space-y-6"
        >
          <LibrarySeriesDetailHeader
            series={selectedSeries}
            allEpisodes={allEpisodes}
            playbackFrom={playbackFrom}
            isAdmin={isAdmin}
            canFavorite={canFavorite}
            favourite={favourite}
            favouriteBusy={favouriteBusy}
            onToggleFavourite={onToggleFavourite}
            seriesToolBusy={seriesToolBusy}
            onBack={onBack}
            onSmartScrape={onSmartScrape}
            onManualScrape={onManualScrape}
            onMetadataEdit={onMetadataEdit}
            onProbe={onProbe}
            onNFO={onNFO}
            onDelete={onDelete}
          />

          <LibrarySeriesEpisodesPanel
            loading={loadingEpisodes}
            selectedEpisodes={selectedEpisodes}
            selectedSeason={selectedSeason}
            visibleEpisodes={visibleEpisodes}
            playbackFrom={playbackFrom}
            isAdmin={isAdmin}
            onSeasonChange={onSeasonChange}
            onManualScrape={onManualScrapeMedia}
            deleteBusy={Boolean(seriesToolBusy)}
            onDeleteSeason={onDeleteSeason}
            onDeleteEpisode={onDeleteEpisode}
          />
        </motion.div>
      )}
    </AnimatePresence>
  )
}

type LibrarySeriesEpisodesPanelProps = {
  loading: boolean
  selectedEpisodes: SeasonEpisodes[]
  selectedSeason: number | null
  visibleEpisodes: Media[]
  playbackFrom: string
  isAdmin: boolean
  onSeasonChange: (season: number) => void
  onManualScrape: (media: Media) => void
  deleteBusy: boolean
  onDeleteSeason: (episodes: Media[]) => void
  onDeleteEpisode: (episode: Media) => void
}

function LibrarySeriesEpisodesPanel({
  loading,
  selectedEpisodes,
  selectedSeason,
  visibleEpisodes,
  playbackFrom,
  isAdmin,
  onSeasonChange,
  onManualScrape,
  deleteBusy,
  onDeleteSeason,
  onDeleteEpisode,
}: LibrarySeriesEpisodesPanelProps) {
  return (
    <div className="space-y-6">
      <LibrarySeriesEpisodes
        loading={loading}
        selectedEpisodes={selectedEpisodes}
        selectedSeason={selectedSeason}
        visibleEpisodes={visibleEpisodes}
        playbackFrom={playbackFrom}
        isAdmin={isAdmin}
        onSeasonChange={onSeasonChange}
        onManualScrape={onManualScrape}
        deleteBusy={deleteBusy}
        onDeleteSeason={onDeleteSeason}
        onDeleteEpisode={onDeleteEpisode}
      />
    </div>
  )
}
