import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Link, useLocation } from 'react-router-dom'
import {
  ArrowRight,
  ChevronLeft,
  ChevronRight,
  Clock,
  Film,
  FolderOpen,
  Library as LibraryIcon,
  Music,
  Pin,
  Play,
  PlayCircle,
  Star,
  Tv,
} from 'lucide-react'

import { ARTWORK, imageURL } from '../api/client'
import { useInViewOnce } from '../hooks/useInViewOnce'
import { useLazyPreviewBatch } from '../hooks/useLazyPreviewBatch'
import { useRememberedListPosition } from '../hooks/useListPositionMemory'
import { MediaCard } from '../components/MediaCard'
import type { HistoryItem } from '../api/playback'
import type { Library, Media } from '../types'
import type { SeriesCard } from '../utils/groupSeries'
import { seriesCardLink } from '../utils/groupSeries'
import { isRemoteEmbyID } from '../utils/remoteEmby'
import { getLibraryArtworks } from './librariesPageModel'
import { formatEpisodeNumber } from '../utils/episodeNumber'

const TYPE_ICONS: Record<string, ReactNode> = {
  movie: <Film size={18} />,
  movies: <Film size={18} />,
  tv: <Tv size={18} />,
  series: <Tv size={18} />,
  anime: <PlayCircle size={18} />,
  shows: <Tv size={18} />,
  variety: <Tv size={18} />,
  music: <Music size={18} />,
  adult: <Film size={18} />,
}
const TYPE_LABELS: Record<string, string> = {
  movie: '电影',
  movies: '电影',
  tv: '剧集',
  series: '剧集',
  anime: '动漫',
  shows: '综艺',
  variety: '综艺',
  music: '音乐',
  adult: 'Adult',
}

export function HomeLoadingState() {
  return (
    <div className="flex items-center justify-center py-48">
      <div className="flex flex-col items-center gap-4 animate-pulse-soft">
        <div className="relative flex items-center justify-center">
          <div className="h-10 w-10 animate-spin rounded-full border-2 border-(--app-border) border-t-(--app-active-bg)" />
          <Film className="absolute h-4 w-4 text-brand-500" />
        </div>
        <span className="text-sm font-semibold uppercase tracking-widest text-(--app-muted)">
          首页内容准备中…
        </span>
      </div>
    </div>
  )
}

export function HomeEmptyState() {
  return (
    <div className="flex flex-col items-center justify-center py-32 text-center max-w-md mx-auto">
      <div className="mb-6 flex h-24 w-24 items-center justify-center rounded-3xl border border-(--app-border) bg-(--app-panel-soft) shadow-xs">
        <Film className="h-10 w-10 text-(--app-muted)" />
      </div>
      <h1 className="text-xl font-semibold text-(--app-text)">您的家庭影视站暂无内容</h1>
      <p className="mt-2 text-sm leading-relaxed text-(--app-muted)">
        前往媒体库添加媒体目录，扫描后首页将展示海报轮播、媒体库和最新入库。
      </p>
      <Link to="/libraries" className="mt-8 btn-primary">
        前往媒体库
      </Link>
    </div>
  )
}

// ContinueWatchingSkeleton 与 ContinueWatchingSection 同构的占位块：
// 播放记录请求独立渐进加载时，避免区块突然弹入造成的布局跳动。
export function ContinueWatchingSkeleton() {
  return (
    <section className="nook-shelf space-y-4" aria-label="继续观看加载中" aria-busy="true">
      <div className="nook-shelf-heading flex items-center gap-3">
        <h2>继续观看</h2>
      </div>
      <div className="flex gap-4 overflow-hidden pb-3 pt-1">
        {[0, 1, 2].map((i) => (
          <div key={i} className="nook-continue-item shrink-0" aria-hidden="true">
            <div className="nook-continue-artwork animate-pulse" />
            <div className="mt-3 h-4 w-2/3 animate-pulse rounded-sm bg-(--app-panel-soft)" />
            <div className="mt-2 h-3 w-1/2 animate-pulse rounded-sm bg-(--app-panel-soft)" />
          </div>
        ))}
      </div>
    </section>
  )
}

/* =========================================================================
   1. 海报轮播区 (Hero Carousel Section)
   ========================================================================= */

export function HomeCarouselSection({
  items,
  libraryMap,
}: {
  items: Media[]
  libraryMap: Map<string, Library>
}) {
  const [currentIndex, setCurrentIndex] = useState(0)
  const [isPaused, setIsPaused] = useState(false)
  const currentItem = items[currentIndex] || items[0]
  const count = items.length

  useEffect(() => {
    if (count <= 1 || isPaused) return
    const timer = setInterval(() => {
      setCurrentIndex((prev) => (prev + 1) % count)
    }, 5000)
    return () => clearInterval(timer)
  }, [count, isPaused])

  if (!currentItem) return null

  const handlePrev = () => setCurrentIndex((prev) => (prev - 1 + count) % count)
  const handleNext = () => setCurrentIndex((prev) => (prev + 1) % count)
  const visual = currentItem.backdrop_url || currentItem.poster_url || ''
  const poster = currentItem.poster_url || currentItem.backdrop_url || ''
  const lib = libraryMap.get(currentItem.display_library_id || currentItem.library_id)

  return (
    <section
      aria-label="焦点推荐"
      aria-roledescription="轮播"
      onMouseEnter={() => setIsPaused(true)}
      onMouseLeave={() => setIsPaused(false)}
      className="nook-spotlight"
    >
      <div className="nook-spotlight-copy">
        <div className="nook-film-meta">
          {currentItem.rating > 0 && <span className="nook-rating"><Star size={13} fill="currentColor" />{currentItem.rating.toFixed(1)}</span>}
          {currentItem.year > 0 && <span>{currentItem.year} 年</span>}
          {currentItem.video_codec && <span>{currentItem.video_codec}</span>}
          {currentItem.container && <span>{currentItem.container}</span>}
          {lib && <span className="nook-spotlight-library">{TYPE_ICONS[lib.type] || <FolderOpen size={14} />}{lib.name}</span>}
        </div>
        <h2 title={currentItem.title}>{currentItem.title}</h2>
        {currentItem.original_name && currentItem.original_name !== currentItem.title && (
          <p className="nook-original-title">{currentItem.original_name}</p>
        )}
        <p className="nook-spotlight-overview">
          {currentItem.overview || '家庭私人媒体中心收藏。支持多端播放、高码率串流与智能刮削。'}
        </p>
        <div className="nook-spotlight-actions">
          <Link to={`/play/${currentItem.id}`} className="btn-primary"><Play size={16} fill="currentColor" />立即播放</Link>
          <Link to={`/media/${currentItem.id}`} className="btn-ghost">查看详情<ArrowRight size={15} /></Link>
        </div>
        {count > 1 && (
          <div className="nook-spotlight-controls">
            <div className="nook-slide-dots">
              {items.map((item, idx) => (
                <button key={`${item.id}-${idx}`} type="button" onClick={() => setCurrentIndex(idx)}
                  aria-pressed={idx === currentIndex} title={`第 ${idx + 1} 张海报`} aria-label={`第 ${idx + 1} 张海报`} />
              ))}
            </div>
            <span className="nook-slide-count">{currentIndex + 1} / {count}</span>
            <button type="button" className="nook-icon-button" onClick={handlePrev} title="上一张" aria-label="上一张"><ChevronLeft size={16} /></button>
            <button type="button" className="nook-icon-button" onClick={handleNext} title="下一张" aria-label="下一张"><ChevronRight size={16} /></button>
          </div>
        )}
      </div>
      <div className="nook-spotlight-artwork">
        <div className="nook-artwork-fallback" aria-hidden="true"><Film size={40} /><span>{currentItem.title}</span></div>
        {visual && (
          <img key={visual + currentItem.id} src={imageURL(visual, currentItem.updated_at, ARTWORK.backdropHero)}
            alt={currentItem.title} fetchPriority="high" decoding="async" referrerPolicy="no-referrer"
            className={!currentItem.backdrop_url ? 'nook-spotlight-poster' : undefined}
            onError={(event) => {
              const image = event.currentTarget
              if (poster && visual !== poster && image.dataset.fallback !== 'true') {
                image.dataset.fallback = 'true'
                image.src = imageURL(poster, currentItem.updated_at, ARTWORK.posterDetail)
                image.classList.add('nook-spotlight-poster')
              } else {
                image.style.display = 'none'
              }
            }} />
        )}
      </div>
    </section>
  )
}

/* =========================================================================
   2. 媒体库卡片区 (Libraries Grid)
   ========================================================================= */

export function HomeLibrariesSection({
  libraries,
  libraryData,
  libraryCounts,
  onNeedPreviews,
  pinnedIds = [],
  onTogglePin,
  showAllLink = true,
  title = '媒体库',
}: {
  libraries: Library[]
  libraryData?: Record<string, { cards: SeriesCard[]; items: Media[]; total: number }>
  libraryCounts: Record<string, number>
  onNeedPreviews?: (ids: string[], limit?: number) => void
  pinnedIds?: string[]
  onTogglePin?: (libraryId: string) => void
  showAllLink?: boolean
  title?: string
}) {
  const PAGE_SIZE = 20
  // 分页位置按路由分别记忆：首页「媒体库」和 /libraries 的「媒体库入口」
  // 各自保留自己的页码，进入媒体库详情再返回时不会掉回第一页。
  const { pathname } = useLocation()
  const [currentPage, setCurrentPage] = useRememberedListPosition(
    `libraries-grid:${pathname}`,
    1,
  )
  const totalPages = Math.max(1, Math.ceil(libraries.length / PAGE_SIZE))
  const effectivePage = Math.min(currentPage, totalPages)
  const queuePreview = useLazyPreviewBatch((ids) => onNeedPreviews?.(ids, 2))

  const pagedLibraries = useMemo<Library[]>(() => {
    const start = (effectivePage - 1) * PAGE_SIZE
    return libraries.slice(start, start + PAGE_SIZE)
  }, [libraries, effectivePage])

  return (
    <section className="nook-shelf space-y-4">
      <div className="nook-shelf-heading flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2.5">
          <span className="nook-section-icon">
            <LibraryIcon size={18} />
          </span>
          <div>
            <div className="flex items-center gap-2">
              <h2 className="font-display text-xl font-extrabold tracking-tight text-(--app-text)">
                {title}
              </h2>
              {libraries.length > PAGE_SIZE && (
                <span className="rounded-md border border-(--app-border) bg-(--app-panel-soft) px-1.5 py-0.5 text-[11px] font-semibold text-(--app-muted)">
                  共 {libraries.length} 个 · 每页 {PAGE_SIZE} 个
                </span>
              )}
            </div>
            <p className="text-xs text-(--app-muted)">
              点击卡片浏览对应媒体库
            </p>
          </div>
        </div>

        {(totalPages > 1 || showAllLink) && (
          <div className="flex items-center gap-3">
            {totalPages > 1 && (
              <div className="flex items-center gap-2 rounded-xl border border-(--app-border) bg-(--app-panel-soft) px-2 py-1 text-xs">
                <button
                  type="button"
                  onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}
                  disabled={effectivePage <= 1}
                  className="rounded-lg p-1 text-(--app-muted) hover:bg-(--app-hover) hover:text-(--app-text) disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-(--app-muted) transition-colors"
                  title="上一页"
                >
                  <ChevronLeft size={14} />
                </button>
                <span className="text-xs font-medium tabular-nums text-(--app-subtle)">
                  {effectivePage} / {totalPages}
                </span>
                <button
                  type="button"
                  onClick={() => setCurrentPage((p) => Math.min(totalPages, p + 1))}
                  disabled={effectivePage >= totalPages}
                  className="rounded-lg p-1 text-(--app-muted) hover:bg-(--app-hover) hover:text-(--app-text) disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-(--app-muted) transition-colors"
                  title="下一页"
                >
                  <ChevronRight size={14} />
                </button>
              </div>
            )}

            {showAllLink && (
              <Link
                to="/libraries"
                className="group inline-flex items-center gap-1 text-xs font-bold text-(--app-subtle) transition-colors hover:text-brand-500"
              >
                <span>全部媒体库</span>
                <ArrowRight size={14} className="transition-transform group-hover:translate-x-0.5" />
              </Link>
            )}
          </div>
        )}
      </div>

      <div className="nook-library-grid">
        {pagedLibraries.map((lib) => (
          <HomeLibraryCard
            key={lib.id}
            library={lib}
            count={libraryCounts[lib.id] ?? 0}
            cards={libraryData?.[lib.id]?.cards ?? []}
            pinned={pinnedIds.includes(lib.id)}
            onTogglePin={onTogglePin ? () => onTogglePin(lib.id) : undefined}
            onVisible={() => {
              if (!lib.cover_url) {
                queuePreview(lib.id)
              }
            }}
          />
        ))}
      </div>
    </section>
  )
}

export function HomeLibraryCard({
  library,
  count,
  cards,
  pinned = false,
  onTogglePin,
  onVisible,
}: {
  library: Library
  count: number
  cards: SeriesCard[]
  pinned?: boolean
  onTogglePin?: () => void
  onVisible: () => void
}) {
  const ref = useInViewOnce<HTMLDivElement>(onVisible)
  const artwork = getLibraryArtworks(library, cards)

  return (
    <div
      ref={ref}
      className={`nook-library-card group relative h-full overflow-hidden rounded-lg border bg-(--app-panel) transition-colors hover:bg-(--app-hover) ${
        pinned ? 'border-brand-500/60 ring-1 ring-brand-500/20' : 'border-(--app-border)'
      }`}
    >
      <Link
        to={`/library/${library.id}`}
        className="nook-library-link flex h-full flex-col justify-between p-3"
        title={library.name}
      >
        <div
          className={`nook-library-artwork relative h-28 w-full overflow-hidden rounded-lg bg-(--app-panel-soft) ${
            artwork.length > 1 ? 'grid grid-cols-2 gap-0.5' : ''
          }`}
        >
          {artwork.length > 0 ? (
            artwork.map(({ src, version }, index) => (
              <img
                key={`${src}-${index}`}
                src={imageURL(
                  src,
                  version,
                  library.cover_url ? undefined : ARTWORK.backdropStrip,
                )}
                alt=""
                loading="lazy"
                referrerPolicy="no-referrer"
                className="h-full w-full object-cover transition-transform duration-500 group-hover:scale-105"
                onError={(event) => {
                  event.currentTarget.style.visibility = 'hidden'
                }}
              />
            ))
          ) : (
            <div className="flex h-full w-full items-center justify-center text-brand-500">
              {TYPE_ICONS[library.type] || <FolderOpen size={28} />}
            </div>
          )}

          <div className="absolute top-2 right-2 rounded-lg bg-[#111] px-2 py-0.5 text-[11px] font-medium text-white">
            {TYPE_LABELS[library.type] || '自定义'}
          </div>
        </div>

        <div className="nook-library-info mt-3 flex flex-col justify-between">
          <h3
            className="line-clamp-2 wrap-break-word font-display text-sm font-bold text-(--app-text) group-hover:text-brand-500"
            title={library.name}
          >
            {library.name}
          </h3>
          <p className="mt-0.5 text-xs text-(--app-muted)">
            {count > 0 ? `${count} 部媒体` : '暂无条目'}
          </p>
        </div>
      </Link>

      {onTogglePin && (
        <button
          type="button"
          onClick={onTogglePin}
          className={`absolute left-2 top-2 z-10 rounded-lg border p-1.5 transition-[opacity,background-color] duration-200 opacity-100 sm:pointer-events-none sm:opacity-0 sm:group-hover:pointer-events-auto sm:group-hover:opacity-100 sm:group-focus-within:pointer-events-auto sm:group-focus-within:opacity-100 sm:focus-visible:pointer-events-auto sm:focus-visible:opacity-100 ${
            pinned
              ? 'border-brand-400/50 bg-brand-500 text-white'
              : 'border-white/20 bg-black/60 text-white hover:bg-black/75'
          }`}
          title={pinned ? '取消置顶' : '置顶媒体库'}
          aria-label={pinned ? '取消置顶' : '置顶媒体库'}
          aria-pressed={pinned}
        >
          <Pin size={13} className={pinned ? 'fill-current' : ''} />
        </button>
      )}
    </div>
  )
}

/* =========================================================================
   3. 单个媒体库内容行 (Horizontal Scroll Row)
   ========================================================================= */

export function HomeLibraryRowSection({
  library,
  cards,
}: {
  library: Library
  cards: SeriesCard[]
}) {
  const scrollRef = useRef<HTMLDivElement>(null)

  if (!cards || cards.length === 0) return null

  const scroll = (direction: 'left' | 'right') => {
    if (scrollRef.current) {
      const scrollAmount = direction === 'left' ? -480 : 480
      scrollRef.current.scrollBy({ left: scrollAmount, behavior: 'smooth' })
    }
  }

  return (
    <section className="nook-shelf space-y-4">
      {/* Row Header */}
      <div className="nook-shelf-heading flex items-center justify-between gap-3">
        <div className="flex items-center gap-2.5">
          <span className="nook-section-icon">
            {TYPE_ICONS[library.type] || <FolderOpen size={18} />}
          </span>
          <div>
            <h2 className="font-display text-xl font-extrabold tracking-tight text-(--app-text)" title={library.name}>
              {library.name}
            </h2>
            <span className="text-xs text-(--app-muted)">
              最近添加 · {cards.length} 部
            </span>
          </div>
        </div>

        <div className="flex items-center gap-3">
          <Link
            to={`/library/${library.id}`}
            className="group inline-flex items-center gap-1 text-xs font-bold text-(--app-subtle) transition-colors hover:text-brand-500"
          >
            <span>查看全部</span>
            <ArrowRight size={14} className="transition-transform group-hover:translate-x-0.5" />
          </Link>

          {/* Scroll Buttons */}
          <div className="hidden sm:flex items-center gap-1">
            <button
              type="button"
              onClick={() => scroll('left')}
              className="rounded-xl border border-(--app-border) p-1.5 text-(--app-muted) hover:bg-(--app-hover) hover:text-(--app-text) transition-colors"
              title="向左滚动"
            >
              <ChevronLeft size={16} />
            </button>
            <button
              type="button"
              onClick={() => scroll('right')}
              className="rounded-xl border border-(--app-border) p-1.5 text-(--app-muted) hover:bg-(--app-hover) hover:text-(--app-text) transition-colors"
              title="向右滚动"
            >
              <ChevronRight size={16} />
            </button>
          </div>
        </div>
      </div>

      {/* Horizontal Carousel Row */}
      <div
        ref={scrollRef}
        className="flex gap-4 overflow-x-auto pb-3 pt-1 no-scrollbar scroll-smooth snap-x"
        style={{ scrollbarWidth: 'none', msOverflowStyle: 'none' }}
      >
        {cards.map((card) => (
          <div
            key={card.key}
            className="nook-shelf-item shrink-0 snap-start"
          >
            <MediaCard
              media={card.rep}
              count={card.count}
              linkTo={seriesCardLink(card)}
              compact
            />
          </div>
        ))}
      </div>
    </section>
  )
}

/* =========================================================================
   4. 继续观看行 (Continue Watching)
   ========================================================================= */

export function ContinueWatchingSection({ history }: { history: HistoryItem[] }) {
  const scrollRef = useRef<HTMLDivElement>(null)

  const scroll = (direction: 'left' | 'right') => {
    if (scrollRef.current) {
      const scrollAmount = direction === 'left' ? -400 : 400
      scrollRef.current.scrollBy({ left: scrollAmount, behavior: 'smooth' })
    }
  }

  return (
    <section className="nook-shelf space-y-4">
      <div className="nook-shelf-heading flex items-center justify-between gap-3">
        <div className="flex items-center gap-2.5">
          <span className="nook-section-icon">
            <Clock size={18} />
          </span>
          <div>
            <h2 className="font-display text-xl font-extrabold tracking-tight text-(--app-text)">
              继续观看
            </h2>
            <span className="text-xs text-(--app-muted)">
              {history.length} 条播放记录
            </span>
          </div>
        </div>

        <div className="hidden sm:flex items-center gap-1">
          <button
            type="button"
            onClick={() => scroll('left')}
            className="rounded-xl border border-(--app-border) p-1.5 text-(--app-muted) hover:bg-(--app-hover) hover:text-(--app-text) transition-colors"
            title="向左滚动"
          >
            <ChevronLeft size={16} />
          </button>
          <button
            type="button"
            onClick={() => scroll('right')}
            className="rounded-xl border border-(--app-border) p-1.5 text-(--app-muted) hover:bg-(--app-hover) hover:text-(--app-text) transition-colors"
            title="向右滚动"
          >
            <ChevronRight size={16} />
          </button>
        </div>
      </div>

      <div
        ref={scrollRef}
        className="flex gap-4 overflow-x-auto pb-3 pt-1 no-scrollbar scroll-smooth"
        style={{ scrollbarWidth: 'none', msOverflowStyle: 'none' }}
      >
        {history.slice(0, 12).map((h) => {
          const media: Media = h.media ?? ({
            id: h.media_id,
            title: isRemoteEmbyID(h.media_id) ? '远程媒体' : '媒体',
            poster_url: '',
            updated_at: h.watched_at,
          } as Media)
          const progress = h.duration_ms > 0 ? h.position_ms / h.duration_ms : 0
          return (
            <div key={h.id} className="nook-continue-item shrink-0">
              <ContinueCard media={media} progress={progress} />
            </div>
          )
        })}
      </div>
    </section>
  )
}

function ContinueCard({ media, progress }: { media: Media; progress: number }) {
  const visual = media.backdrop_url || media.poster_url
  const episodeNumber = formatEpisodeNumber(media)

  return (
    <Link to={`/play/${media.id}`} className="nook-continue-card group">
      <div className="nook-continue-artwork">
        {visual ? (
          <img src={imageURL(visual, media.updated_at, ARTWORK.backdropStrip)} alt=""
            loading="lazy" decoding="async" referrerPolicy="no-referrer" />
        ) : <div className="nook-artwork-fallback"><Film size={28} /></div>}
        <span className="nook-resume-play"><Play size={16} fill="currentColor" /></span>
        {progress > 0 && progress < 1 && (
          <div className="nook-progress-track"><span style={{ width: `${Math.round(progress * 100)}%` }} /></div>
        )}
      </div>
      <div className="nook-continue-info">
        <p title={media.title}>{media.title}</p>
        <div className="nook-continue-meta">
          {media.year > 0 && <span>{media.year}</span>}
          {media.season_num >= 0 && episodeNumber !== '' && (
            <span>S{media.season_num}E{episodeNumber}</span>
          )}
          {progress > 0 && progress < 1 && <span className="ml-auto">已观看 {Math.round(progress * 100)}%</span>}
        </div>
      </div>
    </Link>
  )
}
