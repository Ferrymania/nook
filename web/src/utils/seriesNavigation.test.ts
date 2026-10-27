import { seriesCardLink, type SeriesCard } from './groupSeries'
import { favouriteMediaLink } from './mediaNavigation'
import type { Media } from '../types'

function check(name: string, condition: boolean) {
  if (!condition) throw new Error(`seriesNavigation: ${name}`)
}

const episode = {
  id: 'episode & 24', library_id: 'anime', title: '甲贺忍法帖',
  path: '/media/番剧/Basilisk/S01E24.strm', season_num: 1, episode_num: 24,
} as Media
const card: SeriesCard = { key: 'series:canonical', rep: episode, count: 24 }
const link = new URL(seriesCardLink(card), 'http://localhost')
check('preview opens its library', link.pathname === '/library/anime')
check('preview keeps server key', link.searchParams.get('series') === card.key)
check('preview carries stable ID with correct encoding', link.searchParams.get('media_id') === episode.id)

const favourite = new URL(favouriteMediaLink(episode), 'http://localhost')
check('favourites carry stable media ID too', favourite.searchParams.get('media_id') === episode.id)

const movie = { id: 'movie', library_id: 'movies', title: '电影', path: '/media/movies/movie.mkv' } as Media
check('movie links remain media detail links', seriesCardLink({ key: 'movie', rep: movie, count: 1 }) === '/media/movie')

const remote = { ...episode, id: 'embyremote~mount~show' }
const remoteCard: SeriesCard = { key: remote.id, rep: remote, count: 24, is_series: true }
check('remote series retains the remote key', new URL(seriesCardLink(remoteCard), 'http://localhost').searchParams.get('series') === remote.id)

console.log('seriesNavigation tests passed')
