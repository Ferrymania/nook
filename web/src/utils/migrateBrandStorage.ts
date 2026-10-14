// Run before stores initialize, so existing sessions and preferences survive
// the rename. Never overwrite an already-saved Nook value.
try {
  if (typeof window !== 'undefined') {
    const storage = window.localStorage
    for (const key of Object.keys(storage)) {
      if (!/^mebox[._-]/.test(key)) continue
      const next = key.replace(/^mebox/, 'nook')
      const value = storage.getItem(key)
      if (storage.getItem(next) === null && value !== null) {
        storage.setItem(next, value)
      }
      storage.removeItem(key)
    }
  }
} catch {
  // Storage can be unavailable in restricted browsing contexts.
}
