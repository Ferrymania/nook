import { profileAPI } from '../api/profile'

export async function loadPinnedLibraryIds(): Promise<string[]> {
  return profileAPI.getPinnedLibraries()
}

export async function savePinnedLibraryIds(ids: string[]): Promise<string[]> {
  return profileAPI.setPinnedLibraries(ids)
}

export function togglePinnedLibraryId(current: string[], id: string): string[] {
  const index = current.indexOf(id)
  return index >= 0 ? current.filter((item) => item !== id) : [...current, id]
}

export function isLibraryPinned(id: string, pinnedIds: string[]): boolean {
  return pinnedIds.includes(id)
}
