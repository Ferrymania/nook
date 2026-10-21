import type { Library } from '../types/library'
import type { Media } from '../types/media'

type CarouselLibrary = Pick<Library, 'id' | 'carousel_enabled'>
type CarouselPreview = { cards: ReadonlyArray<{ rep: Media }> }

export function selectHomeCarouselMedia(
  libraries: readonly CarouselLibrary[],
  previews: Readonly<Record<string, CarouselPreview | undefined>>,
): Media[] {
  // The caller supplies only visible libraries in the current tag. Cached
  // previews from other libraries must never bypass that scope or an off switch.
  const queues = libraries
    .filter((library) => library.carousel_enabled === true)
    .map((library) => (previews[library.id]?.cards ?? [])
      .map((card) => card.rep)
      .filter((media) => Boolean(media.poster_url?.trim() || media.backdrop_url?.trim())))

  const selected: Media[] = []
  // Preview cards already follow last-added order. Take one per library each
  // round so a large library cannot consume all ten slots before the others.
  for (let index = 0; selected.length < 10; index += 1) {
    let added = false
    for (const queue of queues) {
      if (queue[index]) {
        selected.push(queue[index])
        added = true
        if (selected.length === 10) return selected
      }
    }
    if (!added) break
  }
  return selected
}
