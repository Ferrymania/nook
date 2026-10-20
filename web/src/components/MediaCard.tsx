import { memo, useEffect, useRef, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { Play, Layers, Star } from 'lucide-react'
import { ARTWORK, imageURL } from '../api/client'
import type { Media } from '../types'
import { ArtworkImage } from './ArtworkImage'
import { artworkSources } from '../utils/artworkSources'

const ACTION_OVERLAY_CLASS =
  'absolute right-2 top-2 z-20 flex flex-wrap justify-end gap-1 opacity-100 transition-opacity sm:pointer-events-none sm:opacity-0 sm:group-hover:pointer-events-auto sm:group-hover:opacity-100 sm:focus-within:pointer-events-auto sm:focus-within:opacity-100'

// memo：父级状态变化（如轮播切图、其它卡片操作）不再级联重渲染所有卡片。
// 注意 actions/renderActions 必须引用稳定（用 renderActions 传函数）memo 才生效。
export const MediaCard = memo(function MediaCard({
  media, progress, count, rating, linkTo, linkState, onClick, actions, renderActions, compact,
}: {
  media: Media
  progress?: number
  count?: number
  rating?: number
  linkTo?: string
  linkState?: { from?: string }
  onClick?: () => void
  actions?: ReactNode
  renderActions?: (media: Media) => ReactNode
  compact?: boolean
}) {
  const ref = useRef<HTMLDivElement>(null)
  const href = linkTo ?? `/media/${media.id}`
  const [posterFit, setPosterFit] = useState<'cover' | 'contain'>('cover')
  // 固定档位而不是在调用点拼尺寸：紧凑布局与普通卡片共用同一份缩略图缓存。
  const posterSources = artworkSources(media, 'poster').map(source => imageURL(source, media.updated_at, ARTWORK.posterCard))
  const blurredPosterSrc = imageURL(media.poster_url, media.updated_at, ARTWORK.posterTiny)
  const displayRating = rating ?? media.rating
  const versionCount = media.versions?.length ?? 0
  // renderActions 延迟到卡片自身渲染时才调用，保证 memo 生效
  const actionContent = actions ?? renderActions?.(media)

  useEffect(() => {
    setPosterFit('cover')
  }, [media.poster_url, media.updated_at])

  const card = (
    <div ref={ref} className={`nook-media-card ${compact ? 'nook-media-card-compact' : ''}`}>
      <div className="nook-media-card-artwork">
        {posterFit === 'contain' && <img src={blurredPosterSrc} alt="" aria-hidden="true" loading="lazy"
          className="absolute inset-0 h-full w-full scale-110 object-cover object-center opacity-25 blur-xl" referrerPolicy="no-referrer" />}
        <ArtworkImage sources={posterSources} alt={media.title} loading="lazy" decoding="async" referrerPolicy="no-referrer"
          onLoad={(event) => {
            const image = event.currentTarget
            setPosterFit(image.naturalWidth > image.naturalHeight ? 'contain' : 'cover')
          }}
          className={`nook-media-card-image ${posterFit === 'contain' ? 'object-contain p-1.5' : 'object-cover'}`} fallbackClassName="nook-artwork-fallback" />
        <div className="nook-media-card-hover" aria-hidden="true">
          <span className="nook-media-play"><Play size={20} fill="currentColor" /></span>
          {!compact && <p>{media.overview || '暂无简介内容'}</p>}
        </div>
        {progress !== undefined && progress > 0 && progress < 1 && (
          <div className="nook-progress-track"><span style={{ width: `${Math.round(progress * 100)}%` }} /></div>
        )}
      </div>
      <div className="nook-media-card-info">
        <p className="nook-media-card-title" title={media.title}>{media.title}</p>
        <div className="nook-media-card-meta">
          <span>{media.year > 0 ? media.year : '未知年份'}</span>
          {displayRating > 0 && <span className="nook-rating ml-auto"><Star size={11} fill="currentColor" />{displayRating.toFixed(1)}</span>}
        </div>
        <div className="nook-media-card-formats">
          {(count !== undefined && count > 1) || (count === undefined && versionCount > 1) ? (
            <span><Layers size={12} /><span>{count !== undefined && count > 1 ? `${count} 集` : `${versionCount} 版本`}</span></span>
          ) : null}
          {media.video_codec && <span className="hidden sm:inline-flex">{media.video_codec}</span>}
        </div>
      </div>
    </div>
  )

  if (onClick) {
    return (
      <div className="group relative block h-full w-full">
        <button type="button" onClick={onClick} className="block h-full w-full text-left">
          {card}
        </button>
        {actionContent && (
          <div className={ACTION_OVERLAY_CLASS}>
            {actionContent}
          </div>
        )}
      </div>
    )
  }

  if (actionContent) {
    return (
      <div className="group relative block h-full">
        <Link to={href} state={linkState} className="block h-full">
          {card}
        </Link>
        <div className={ACTION_OVERLAY_CLASS}>
          {actionContent}
        </div>
      </div>
    )
  }

  return (
    <Link to={href} state={linkState} className="group block h-full">
        {card}
    </Link>
  )
})
