import { motion } from 'framer-motion'
import { FileText, Play } from 'lucide-react'
import { Link } from 'react-router-dom'

import { ARTWORK, imageURL } from '../api/client'
import type { Media } from '../types'

type MediaDetailArtworkProps = {
  media: Media
}

export function MediaDetailBackdrop({ media }: MediaDetailArtworkProps) {
  return (
    <div className="nook-detail-backdrop absolute inset-0 h-[480px] z-0 overflow-hidden" aria-hidden="true">
      {(media.backdrop_url || media.poster_url) && (
        <img src={imageURL(media.backdrop_url || media.poster_url || '', media.updated_at, ARTWORK.backdropHero)}
          alt="" decoding="async" fetchPriority="low" className="w-full h-full object-cover opacity-[0.04]"
          referrerPolicy="no-referrer" />
      )}
    </div>
  )
}

export function MediaDetailPoster({ media }: MediaDetailArtworkProps) {
  return (
    <div className="nook-detail-poster w-56 shrink-0 mx-auto md:mx-0">
      <motion.div
        whileHover={{ scale: 1.02 }}
        className="aspect-[2/3] w-full rounded-lg overflow-hidden bg-[var(--app-panel-soft)] relative group"
      >
        {media.poster_url ? (
          <img
            src={imageURL(media.poster_url, media.updated_at, ARTWORK.posterDetail)}
            alt={media.title}
            decoding="async"
            fetchPriority="high"
            className="h-full w-full object-cover transition-transform duration-500 group-hover:scale-105"
            referrerPolicy="no-referrer"
          />
        ) : (
          <div className="flex h-full w-full flex-col items-center justify-center gap-2 text-gray-500 bg-gray-50">
            <FileText size={40} className="stroke-[1]" />
            <span className="text-xs uppercase tracking-wider font-bold">无海报</span>
          </div>
        )}

        <Link
          to={`/play/${media.id}`}
          state={{ from: `/media/${media.id}` }}
          className="absolute inset-0 bg-[#111827]/40 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center"
        >
          <div className="flex h-14 w-14 items-center justify-center rounded-full bg-[var(--app-command-bg)] text-[var(--app-command-text)] transform scale-90 group-hover:scale-100 transition-transform">
            <Play size={24} fill="currentColor" />
          </div>
        </Link>
      </motion.div>
    </div>
  )
}
