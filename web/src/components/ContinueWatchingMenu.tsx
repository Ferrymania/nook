import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react'
import { createPortal } from 'react-dom'
import { Link } from 'react-router-dom'
import { CheckCircle2, Circle, Heart, Loader2, MoreHorizontal, Play, Trash2 } from 'lucide-react'
import toast from 'react-hot-toast'

import { historyAPI } from '../api/history'
import { playbackAPI } from '../api/playback'
import { confirmAction } from './confirmAction'
import type { Media } from '../types'

type Action = 'favourite' | 'watched' | 'unwatched' | 'remove'

export function ContinueWatchingMenu({ history, media, onHistoryChanged }: {
  history: { id: string }
  media: Media
  onHistoryChanged: (id: string) => void
}) {
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const busyRef = useRef(false)
  const focusedRef = useRef(false)
  const [favourite, setFavourite] = useState<boolean | null>(null)
  const [favouriteError, setFavouriteError] = useState(false)
  const [retryKey, setRetryKey] = useState(0)
  const [position, setPosition] = useState<{ top: number; left: number } | null>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const menuID = useId()
  const close = useCallback(() => {
    focusedRef.current = false
    setOpen(false)
    triggerRef.current?.focus({ preventScroll: true })
  }, [])

  useEffect(() => {
    if (!open) return
    let cancelled = false
    setFavourite(null)
    setFavouriteError(false)
    playbackAPI.favouriteStatus(media.id)
      .then((value) => { if (!cancelled) setFavourite(value) })
      .catch(() => { if (!cancelled) setFavouriteError(true) })
    return () => { cancelled = true }
  }, [open, media.id, retryKey])

  useLayoutEffect(() => {
    if (!open) return
    const update = () => {
      const rect = triggerRef.current?.getBoundingClientRect()
      if (!rect) return
      const menu = menuRef.current?.getBoundingClientRect()
      const width = menu?.width ?? Math.min(240, window.innerWidth - 16)
      const height = menu?.height ?? 240
      setPosition({
        left: Math.max(8, Math.min(rect.right - width, window.innerWidth - width - 8)),
        top: Math.max(8, Math.min(rect.bottom + 8, window.innerHeight - height - 8)),
      })
    }
    update()
    window.addEventListener('resize', update)
    window.addEventListener('scroll', update, true)
    return () => {
      window.removeEventListener('resize', update)
      window.removeEventListener('scroll', update, true)
    }
  }, [open])

  useLayoutEffect(() => {
    if (!open || !position || focusedRef.current) return
    menuRef.current?.querySelector<HTMLElement>('[role="menuitem"]')?.focus({ preventScroll: true })
    focusedRef.current = true
  }, [open, position])

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'Escape' || event.key === 'Tab') {
      event.preventDefault()
      close()
      return
    }
    const directions = ['ArrowDown', 'ArrowUp', 'Home', 'End']
    if (!directions.includes(event.key)) return
    event.preventDefault()
    const options = [...(menuRef.current?.querySelectorAll<HTMLElement>('[role="menuitem"]:not(:disabled)') ?? [])]
    const current = options.indexOf(document.activeElement as HTMLElement)
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? options.length - 1
      : (current + (event.key === 'ArrowDown' ? 1 : -1) + options.length) % options.length
    options[next]?.focus()
  }

  const perform = async (action: Action) => {
    if (busyRef.current) return
    if (action === 'favourite' && favourite === null) return
    busyRef.current = true
    setBusy(true)
    try {
      if (action === 'favourite') {
        const next = await playbackAPI.setFavourite(media.id, !favourite)
        setFavourite(next)
        toast.success(next ? '已加入我的收藏' : '已取消收藏')
        close()
        return
      }
      close()
      if (action === 'remove' && !await confirmAction({
        title: '移除观看记录', message: `移除「${media.title}」的观看记录？不会删除媒体文件。`, confirmText: '移除',
      })) return
      if (action === 'unwatched' && !await confirmAction({
        title: '标记为未看', message: `将「${media.title}」标记为未看并清除播放进度？`, confirmText: '标记为未看',
      })) return
      if (action === 'remove') await historyAPI.remove(history.id)
      else await historyAPI.setPlayed(history.id, action === 'watched')
      onHistoryChanged(history.id)
      toast.success(action === 'remove' ? '已移除观看记录' : action === 'watched' ? '已标记为已看' : '已标记为未看')
    } catch (err) {
      toast.error((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '操作失败，请重试')
    } finally {
      busyRef.current = false
      setBusy(false)
    }
  }

  const optionClass = 'flex min-h-11 w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-sm text-(--app-text) outline-none hover:bg-(--app-hover) focus-visible:bg-(--app-hover) disabled:opacity-50'
  return <>
    <button ref={triggerRef} type="button" disabled={busy} aria-label={`操作：${media.title}`} aria-haspopup="menu" aria-expanded={open} aria-controls={open ? menuID : undefined}
      onClick={() => { setPosition(null); setOpen((value) => !value) }}
      onKeyDown={(event) => { if (event.key === 'ArrowDown' || event.key === 'ArrowUp') { event.preventDefault(); setOpen(true) } }}
      className="absolute right-2 top-2 flex h-11 w-11 items-center justify-center rounded-full bg-black/65 text-white transition-colors hover:bg-black/85 focus-visible:outline-2 focus-visible:outline-offset-2 disabled:opacity-60">
      {busy ? <Loader2 size={18} className="animate-spin" /> : <MoreHorizontal size={20} />}
    </button>
    {open && createPortal(<>
      <div className="fixed inset-0 z-80" aria-hidden="true" onPointerDown={close} />
      <div ref={menuRef} id={menuID} role="menu" aria-label={`${media.title}的观看操作`} aria-busy={busy} onKeyDown={onKeyDown}
        className="fixed z-90 w-60 max-w-[calc(100vw-1rem)] overflow-y-auto rounded-xl border border-(--app-border) bg-(--app-panel) p-1.5 shadow-xl"
        style={{ top: position?.top ?? 0, left: position?.left ?? 0, visibility: position ? 'visible' : 'hidden', maxHeight: 'calc(100dvh - 16px)' }}>
        <Link role="menuitem" to={`/play/${media.id}`} state={{ from: '/' }} onClick={close} className={optionClass}><Play size={16} />继续播放</Link>
        <button type="button" role="menuitem" disabled={busy || (!favouriteError && favourite === null)} className={optionClass}
          onClick={() => favouriteError ? setRetryKey((key) => key + 1) : void perform('favourite')}>
          <Heart size={16} fill={favourite ? 'currentColor' : 'none'} />{favouriteError ? '重试加载收藏状态' : favourite === null ? '加载收藏状态…' : favourite ? '取消收藏' : '加入收藏'}
        </button>
        <div className="my-1 border-t border-(--app-border)" />
        <button type="button" role="menuitem" disabled={busy} onClick={() => void perform('watched')} className={optionClass}><CheckCircle2 size={16} />标记为已看</button>
        <button type="button" role="menuitem" disabled={busy} onClick={() => void perform('unwatched')} className={optionClass}><Circle size={16} />标记为未看</button>
        <div className="my-1 border-t border-(--app-border)" />
        <button type="button" role="menuitem" disabled={busy} onClick={() => void perform('remove')} className={`${optionClass} text-red-500!`}><Trash2 size={16} />移除观看记录</button>
      </div>
    </>, document.body)}
  </>
}
