import { useEffect, useId, useRef, useState, type KeyboardEvent } from 'react'
import { createPortal } from 'react-dom'
import { MoreHorizontal } from 'lucide-react'

import { MEDIA_MANAGEMENT_ACTIONS, type MediaManagementActionKey } from '../components/mediaManagementActions'
import type { Media } from '../types'
import { isRemoteEmbyID } from '../utils/remoteEmby'

type LibraryMovieActionsProps = {
  media: Media
  busy: boolean
  onSmartScrape: (media: Media) => void
  onManualScrape: (media: Media) => void
  onMetadataEdit: (media: Media) => void
  onProbe: (media: Media) => void
  onNFO: (media: Media) => void
  onOrganize: (media: Media) => void
  onQuerySegments: (media: Media) => void
  onDelete: (media: Media) => void
}

export function LibraryMovieActions({
  media,
  busy,
  onSmartScrape,
  onManualScrape,
  onMetadataEdit,
  onProbe,
  onNFO,
  onOrganize,
  onQuerySegments,
  onDelete,
}: LibraryMovieActionsProps) {
  const [position, setPosition] = useState<{ top: number; left: number } | null>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const initialFocus = useRef<'first' | 'last'>('first')
  const menuID = useId()
  const isOpen = position !== null

  useEffect(() => {
    if (!isOpen) return
    const items = menuRef.current?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')
    const item = initialFocus.current === 'last' ? items?.[items.length - 1] : items?.[0]
    item?.focus()

    const closeOutside = (event: Event) => {
      if (!menuRef.current?.contains(event.target as Node) && !triggerRef.current?.contains(event.target as Node)) {
        setPosition(null)
      }
    }
    const close = () => setPosition(null)
    const closeOnScroll = (event: Event) => {
      if (!menuRef.current?.contains(event.target as Node)) close()
    }
    document.addEventListener('pointerdown', closeOutside)
    document.addEventListener('focusin', closeOutside)
    window.addEventListener('scroll', closeOnScroll, true)
    window.addEventListener('resize', close)
    return () => {
      document.removeEventListener('pointerdown', closeOutside)
      document.removeEventListener('focusin', closeOutside)
      window.removeEventListener('scroll', closeOnScroll, true)
      window.removeEventListener('resize', close)
    }
  }, [isOpen])

  const openMenu = (focus: 'first' | 'last' = 'first') => {
    if (busy || !triggerRef.current) return
    const rect = triggerRef.current.getBoundingClientRect()
    const width = 184
    const height = Math.min(MEDIA_MANAGEMENT_ACTIONS.length * 44 + 14, window.innerHeight - 16)
    const spaceBelow = window.innerHeight - rect.bottom
    const top = spaceBelow < height + 8 && rect.top > spaceBelow ? rect.top - height - 6 : rect.bottom + 6
    initialFocus.current = focus
    setPosition({
      top: Math.max(8, Math.min(top, window.innerHeight - height - 8)),
      left: Math.max(8, Math.min(rect.right - width, window.innerWidth - width - 8)),
    })
  }

  const closeAndFocusTrigger = () => {
    setPosition(null)
    triggerRef.current?.focus()
  }

  const handleMenuKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      closeAndFocusTrigger()
      return
    }
    if (event.key === 'Tab') {
      closeAndFocusTrigger()
      return
    }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const items = Array.from(menuRef.current?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)') ?? [])
    const current = items.indexOf(document.activeElement as HTMLButtonElement)
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 :
      (current + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length
    items[next]?.focus()
  }

  // 远程 Emby 条目为只读视图：刮削/探测/NFO/删除不适用。
  if (isRemoteEmbyID(media.id)) {
    return null
  }

  const handlers: Record<MediaManagementActionKey, (media: Media) => void> = {
    scrape: onSmartScrape, match: onManualScrape, metadata: onMetadataEdit,
    probe: onProbe, nfo: onNFO, organize: onOrganize, segments: onQuerySegments, delete: onDelete,
  }

  return <>
    <button
      ref={triggerRef}
      type="button"
      title="媒体管理"
      aria-label={`媒体管理：${media.title}`}
      aria-haspopup="menu"
      aria-expanded={isOpen}
      aria-controls={isOpen ? menuID : undefined}
      disabled={busy}
      className="nook-poster-action"
      onClick={(event) => {
        event.preventDefault()
        event.stopPropagation()
        if (isOpen) closeAndFocusTrigger()
        else openMenu()
      }}
      onKeyDown={(event) => {
        if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
          event.preventDefault()
          openMenu(event.key === 'ArrowUp' ? 'last' : 'first')
        }
      }}
    >
      <MoreHorizontal size={16} aria-hidden="true" />
    </button>
    {position && createPortal(
      <div
        ref={menuRef}
        id={menuID}
        role="menu"
        aria-label={`媒体管理：${media.title}`}
        className="nook-media-actions-menu"
        style={position}
        onKeyDown={handleMenuKeyDown}
      >
        {MEDIA_MANAGEMENT_ACTIONS.map(action => (
          <button
            key={action.key}
            type="button"
            role="menuitem"
            tabIndex={-1}
            disabled={busy}
            className={`nook-media-actions-menu-item${'danger' in action ? ' is-danger' : ''}`}
            onClick={(event) => {
              event.preventDefault()
              event.stopPropagation()
              closeAndFocusTrigger()
              handlers[action.key](media)
            }}
          >
            <action.icon size={16} aria-hidden="true" /><span>{action.label}</span>
          </button>
        ))}
      </div>, document.body,
    )}
  </>
}
