import { duplicateLibrary, rootDraftError, scrapeSettingsError } from './adminLibraryPanelModel.ts'
import type { Library, LibraryRoot } from '../types/library.ts'
import type { LibraryScrapeSettingsView } from '../api/library.ts'

function check(value: unknown, message: string) { if (!value) throw new Error(message) }
const library = { id: 'movie-library', name: 'Movies', type: 'movie' } as Library
check(duplicateLibrary([library], ' movies ', 'movie')?.id === library.id, 'same library must be an explicit add-directory flow')
check(!duplicateLibrary([library], 'Movies', 'tv'), 'different media types may share a name')
check(!duplicateLibrary([library], 'New Movies', 'movie'), 'new names must remain creatable')
check(Boolean(rootDraftError([{ path: '' }])), 'empty roots must be rejected')
check(Boolean(rootDraftError([{ path: '/media/movies' }, { path: '/media/movies/' }])), 'trailing slash must not bypass duplicate check')
check(Boolean(rootDraftError([{ path: '/media/movies/' }], [{ path: '/media/movies' }] as LibraryRoot[])), 'existing target roots must be rejected')
check(!rootDraftError([{ path: '/disk2/movies' }], [{ path: '/media/movies' }] as LibraryRoot[]), 'new target roots must be accepted')
const data = { settings: { providers: ['tmdb'], language: 'zh-CN', delay_min_ms: 0, delay_max_ms: 0 } } as LibraryScrapeSettingsView
check(!scrapeSettingsError(data), 'zero interval is valid')
check(Boolean(scrapeSettingsError({ ...data, settings: { ...data.settings, providers: ['fanart'] } })), 'image-only provider must not be accepted')
for (const [min, max] of [[100, 99], [-1, 0], [0, 300001], [0.5, 1], [0, NaN]]) {
  check(Boolean(scrapeSettingsError({ ...data, settings: { ...data.settings, delay_min_ms: min, delay_max_ms: max } })), 'invalid scrape interval must be rejected')
}
console.log('library management: 14 regression checks passed')
