export function isPlaybackLibraryHidden(allowedIDs: readonly string[], libraryID: string): boolean {
  return allowedIDs.length > 0 && !allowedIDs.includes(libraryID)
}

// The existing API stores an allow-list, where [] means all authorized libraries.
// Use the server's no-access marker when every available library is hidden.
export function setPlaybackLibraryHidden(
  allowedIDs: readonly string[],
  availableIDs: readonly string[],
  libraryID: string,
  hidden: boolean,
): string[] {
  if (!availableIDs.includes(libraryID)) return [...allowedIDs]
  const visible = availableIDs.filter((id) => id === libraryID
    ? !hidden
    : !isPlaybackLibraryHidden(allowedIDs, id))
  if (visible.length === availableIDs.length) return []
  return visible.length ? visible : ['__no_access__']
}
