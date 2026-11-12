import { isPlaybackLibraryHidden, setPlaybackLibraryHidden } from './playbackLibraryVisibility'

const libraries = ['movies', 'tv', 'anime']
function assert(condition: boolean, message: string) {
  if (!condition) throw new Error(message)
}

// Loading existing preferences must retain the same visible libraries.
assert(libraries.every((id) => !isPlaybackLibraryHidden([], id)), 'default must show all libraries')
assert(!isPlaybackLibraryHidden(['movies'], 'movies'), 'existing allowed library must remain visible')
assert(isPlaybackLibraryHidden(['movies'], 'tv'), 'existing excluded library must appear checked')

let allowed: string[] = []
for (const id of libraries) allowed = setPlaybackLibraryHidden(allowed, libraries, id, true)
assert(libraries.every((id) => isPlaybackLibraryHidden(allowed, id)), 'hiding all must not reset to unrestricted')

allowed = setPlaybackLibraryHidden(allowed, libraries, 'tv', false)
assert(!isPlaybackLibraryHidden(allowed, 'tv'), 'unchecking must restore the selected library')
assert(isPlaybackLibraryHidden(allowed, 'movies'), 'unchecking one must keep the other libraries hidden')

for (const id of libraries) allowed = setPlaybackLibraryHidden(allowed, libraries, id, false)
assert(allowed.length === 0, 'unchecking all must restore the existing unrestricted preference')

const single = setPlaybackLibraryHidden([], ['movies'], 'movies', true)
assert(isPlaybackLibraryHidden(single, 'movies'), 'single available library must be hideable')
assert(setPlaybackLibraryHidden(single, ['movies'], 'movies', false).length === 0, 'single library must be restorable')
assert(setPlaybackLibraryHidden(['movies'], libraries, 'unauthorized', false).join() === 'movies', 'cannot add an unavailable library')

console.log('playback library visibility tests passed')
