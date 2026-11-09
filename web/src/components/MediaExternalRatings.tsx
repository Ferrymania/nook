import { useEffect, useState } from 'react'
import { Star } from 'lucide-react'

import { externalRatingsAPI, type ExternalRatings } from '../api/library'
import imdbIcon from '../assets/ratings/imdb.svg'
import metacriticIcon from '../assets/ratings/metacritic.png'
import tmdbIcon from '../assets/ratings/tmdb.svg'
import freshIcon from '../assets/ratings/tomatometer-fresh.svg'
import rottenIcon from '../assets/ratings/tomatometer-rotten.svg'
import type { Media } from '../types'
import { isRemoteEmbyID } from '../utils/remoteEmby'

export function MediaExternalRatings({ media, showMediaInfo = false }: { media: Media; showMediaInfo?: boolean }) {
  const [result, setResult] = useState<{ identity: string; ratings: ExternalRatings }>()
  const id = media.id
  const identity = JSON.stringify([id, media.tmdb_id, media.updated_at])

  useEffect(() => {
    if (!id || isRemoteEmbyID(id)) return
    const controller = new AbortController()
    void externalRatingsAPI.media(id, controller.signal)
      .then(ratings => {
        if (!controller.signal.aborted) setResult({ identity, ratings })
      })
      .catch(() => undefined)
    return () => controller.abort()
  }, [id, identity])

  const scores = result?.identity === identity ? result.ratings : undefined
  const tomatoes = scores?.rotten_tomatoes
  const ratings = [
    { label: 'IMDb', icon: imdbIcon, height: 16, value: scores?.imdb?.toFixed(1) },
    {
      label: '烂番茄影评人评分',
      icon: tomatoes != null && tomatoes >= 60 ? freshIcon : rottenIcon,
      height: 18,
      value: tomatoes == null ? undefined : `${tomatoes}%`,
    },
    { label: 'Metacritic', icon: metacriticIcon, height: 19, value: scores?.metacritic == null ? undefined : `${scores.metacritic}/100` },
  ]
  const rating = showMediaInfo && Number.isFinite(media.rating) && media.rating > 0 ? media.rating.toFixed(1) : undefined
  const year = showMediaInfo && media.year > 0 ? media.year : undefined
  const tmdb = media.rating_source === 'tmdb'
  const ratingLabel = tmdb ? 'TMDB' : '媒体库评分'
  if (!rating && !year && !ratings.some(item => item.value != null)) return null

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
      {ratings.map(({ label, icon, height, value }) => {
        if (value == null) return null
        return (
          <span key={label} className="inline-flex items-center gap-1.5 whitespace-nowrap" title={label}>
            <img src={icon} alt="" className="w-auto shrink-0" style={{ height }} />
            <span className="sr-only">{label} </span>
            <strong className="font-semibold">{value}</strong>
          </span>
        )
      })}
      {year && <span className="text-(--app-subtle)">{year} 年</span>}
    </div>
  )
}
