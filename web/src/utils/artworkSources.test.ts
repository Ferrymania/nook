import { artworkSources, isMissingArtwork } from './artworkSources'

function check(name: string, actual: unknown, expected: unknown) {
  if (JSON.stringify(actual) !== JSON.stringify(expected)) throw new Error(`${name}: got ${JSON.stringify(actual)}, want ${JSON.stringify(expected)}`)
}
const media = { episode_still_url: '/still.webp', backdrop_url: '/backdrop.webp', poster_url: '/poster.webp' }
check('episode still wins over shared artwork', artworkSources(media, 'episode'), ['/still.webp', '/backdrop.webp', '/poster.webp'])
check('series poster does not use an episode still', artworkSources(media, 'poster'), ['/poster.webp', '/backdrop.webp'])
check('duplicates and blank sources do not produce repeated requests', artworkSources({ episode_still_url: ' /same.webp ', backdrop_url: '/same.webp', poster_url: ' ' }, 'episode'), ['/same.webp'])
check('existing shared artwork works without an episode image', artworkSources({ episode_still_url: '', poster_url: '/poster.webp' }, 'episode'), ['/poster.webp'])
check('no images', artworkSources({}, 'episode'), [])
check('transparent proxy fallback is a failure', isMissingArtwork(1, 1), true)
check('regular poster is valid', isMissingArtwork(560, 840), false)
check('an image with a one-pixel edge is not the proxy placeholder', isMissingArtwork(1, 100), false)
