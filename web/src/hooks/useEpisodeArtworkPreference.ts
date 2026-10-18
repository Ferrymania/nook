import { useCallback, useEffect, useRef, useState, type Dispatch, type SetStateAction } from 'react'
import toast from 'react-hot-toast'

import { adminAPI } from '../api/admin'
import { useAuthStore } from '../stores/auth'

const SETTING_KEY = 'scrape.episode_images'
const CHANGED_EVENT = 'nook:episode-artwork-changed'

export function notifyEpisodeArtworkPreference(enabled: boolean) {
  window.dispatchEvent(new CustomEvent(CHANGED_EVENT, { detail: enabled }))
}

// This is an instance-wide scraper preference. The existing admin settings
// endpoint stores it in the Setting table, so it follows the server instead
// of a particular browser.
export function useEpisodeArtworkPreference(): [boolean, Dispatch<SetStateAction<boolean>>] {
  const [enabled, setEnabledState] = useState(false)
  const enabledRef = useRef(false)
  const changedLocally = useRef(false)
  const saveQueue = useRef(Promise.resolve())
  const isAdmin = useAuthStore((state) => state.user?.role === 'admin')

  useEffect(() => {
    if (!isAdmin) return

    let active = true
    const onChanged = (event: Event) => {
      const value = (event as CustomEvent<boolean>).detail
      changedLocally.current = true
      enabledRef.current = value
      setEnabledState(value)
    }
    window.addEventListener(CHANGED_EVENT, onChanged)
    adminAPI.listSettings()
      .then((settings) => {
        if (!active || changedLocally.current) return
        const value = settings.find((setting) => setting.key === SETTING_KEY)?.value
        const next = value === 'true' || value === '1'
        enabledRef.current = next
        setEnabledState(next)
      })
      .catch(() => {
        if (active) {
          toast.error('读取每集图片设置失败')
        }
      })
    return () => {
      active = false
      window.removeEventListener(CHANGED_EVENT, onChanged)
    }
  }, [isAdmin])

  const setEnabled = useCallback((nextValue: SetStateAction<boolean>) => {
    if (!isAdmin) return

    const next = typeof nextValue === 'function' ? nextValue(enabledRef.current) : nextValue
    if (enabledRef.current === next) return
    changedLocally.current = true
    enabledRef.current = next
    setEnabledState(next)
    notifyEpisodeArtworkPreference(next)

    saveQueue.current = saveQueue.current
      .catch(() => undefined)
      .then(() => adminAPI.updateSetting(SETTING_KEY, String(next)))
      .catch(() => {
        toast.error('保存每集图片设置失败')
      })
  }, [isAdmin])

  return [enabled, setEnabled]
}
