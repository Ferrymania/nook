import type { Library } from '../types/library'
import type { Media } from '../types/media'
import { selectHomeCarouselMedia } from './homeCarousel'

const failures: string[] = []
let checks = 0

function check(name: string, actual: Media[], expected: string[]) {
  checks += 1
  const ids = actual.map((media) => media.id)
  if (JSON.stringify(ids) !== JSON.stringify(expected)) {
    failures.push(`${name}: got ${JSON.stringify(ids)}, want ${JSON.stringify(expected)}`)
  }
}

function library(id: string, carousel_enabled?: boolean): Pick<Library, 'id' | 'carousel_enabled'> {
  return { id, carousel_enabled }
}

function card(id: string, artwork: Partial<Pick<Media, 'poster_url' | 'backdrop_url'>> = { poster_url: '/poster.webp' }) {
  return { rep: { id, ...artwork } as Media }
}

const previews = {
  movies: { cards: [card('movie-new'), card('movie-old')] },
  shows: { cards: [card('show-new'), card('show-old')] },
}

check('all switches off hides the carousel even with loaded artwork',
  selectHomeCarouselMedia([library('movies', false), library('shows', false)], previews), [])
check('an unset switch does not opt a library in',
  selectHomeCarouselMedia([library('movies')], previews), [])
check('an empty opted-in library does not fall back to an opted-out library',
  selectHomeCarouselMedia([library('empty', true), library('movies', false)], { ...previews, empty: { cards: [] } }), [])
check('an opted-in library still loading does not fall back to another library',
  selectHomeCarouselMedia([library('loading', true), library('movies', false)], previews), [])
check('entries with missing or blank artwork are skipped without falling back',
  selectHomeCarouselMedia([library('blank', true), library('movies', false)], {
    ...previews, blank: { cards: [card('no-art', {}), card('blank-art', { poster_url: ' ', backdrop_url: '\t' })] },
  }), [])
check('opted-in libraries alternate while preserving each preview order',
  selectHomeCarouselMedia([library('movies', true), library('shows', true)], previews),
  ['movie-new', 'show-new', 'movie-old', 'show-old'])
check('short and artwork-less libraries do not block the remaining queues',
  selectHomeCarouselMedia([library('movies', true), library('shows', true), library('anime', true)], {
    movies: { cards: [card('no-art', {}), card('movie-new'), card('movie-old'), card('movie-earliest')] },
    shows: { cards: [card('show-new')] },
    anime: { cards: [card('anime-new', { backdrop_url: '/backdrop.webp' }), card('blank-art', { poster_url: ' ' })] },
  }), ['movie-new', 'show-new', 'anime-new', 'movie-old', 'movie-earliest'])

const movieCards = Array.from({ length: 12 }, (_, index) => card(`movie-${index}`))
const showCards = Array.from({ length: 12 }, (_, index) => card(`show-${index}`))
check('the ten-item limit is shared fairly across libraries',
  selectHomeCarouselMedia([library('movies', true), library('shows', true)], {
    movies: { cards: movieCards }, shows: { cards: showCards },
  }), Array.from({ length: 5 }, (_, index) => [`movie-${index}`, `show-${index}`]).flat())
check('a single opted-in library retains its order and the ten-item cap',
  selectHomeCarouselMedia([library('movies', true), library('shows', false)], {
    movies: { cards: movieCards }, shows: { cards: showCards },
  }), movieCards.slice(0, 10).map((item) => item.rep.id))
check('cached previews outside the visible or selected-tag libraries are excluded',
  selectHomeCarouselMedia([library('movies', true)], previews), ['movie-new', 'movie-old'])
check('no visible libraries hides the carousel despite cached previews',
  selectHomeCarouselMedia([], previews), [])

if (failures.length > 0) throw new Error(`homeCarousel regressions:\n${failures.join('\n')}`)
console.log(`homeCarousel.test.ts: ${checks} checks passed`)
