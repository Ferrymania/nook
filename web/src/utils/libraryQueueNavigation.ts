export const LIBRARY_QUEUE_PATH = '/libraries/queue'

export type LibraryQueueTab = 'scrape' | 'segments'

export function libraryQueueLocation(tab: LibraryQueueTab = 'scrape', search = '') {
  const params = new URLSearchParams(search)
  if (tab === 'segments') {
    params.set('tab', tab)
  } else {
    params.delete('tab')
  }
  const query = params.toString()
  return { pathname: LIBRARY_QUEUE_PATH, search: query ? `?${query}` : '' }
}
