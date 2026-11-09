import { Star } from 'lucide-react'

import tmdbIcon from '../assets/ratings/tmdb.svg'
import type { Media } from '../types'

export function MediaRating({ media }: { media: Media }) {
  const rating = Number.isFinite(media.rating) && media.rating > 0 ? media.rating.toFixed(1) : undefined
  const year = media.year > 0 ? media.year : undefined
  const tmdb = media.rating_source === 'tmdb'
  const ratingLabel = tmdb ? 'TMDB' : '媒体库评分'
  if (!rating && !year) return null

  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm font-medium tabular-nums">
      {rating && (
        <span className="inline-flex items-center gap-1.5 whitespace-nowrap" title={ratingLabel}>
          {tmdb
            ? <img src={tmdbIcon} alt="" className="w-auto shrink-0" style={{ height: 15 }} />
            : <Star size={15} fill="currentColor" className="text-(--app-danger-text)" aria-hidden="true" />}
          <span className="sr-only">{ratingLabel} </span>
          <strong className="font-semibold">{rating}</strong>
        </span>
      )}
      {year && <span className="text-(--app-subtle)">{year} 年</span>}
    </div>
  )
}
