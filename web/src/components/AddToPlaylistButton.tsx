import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { createPortal } from 'react-dom'
import { Check, ListPlus, Loader2, Plus, X } from 'lucide-react'
import toast from 'react-hot-toast'
import { playbackAPI } from '../api/playback'
import type { Media, Playlist } from '../types'
import { playlistErrorMessage } from '../utils/playlists'

type Props = { media: Media; episodes?: Media[]; disabled?: boolean }

export function AddToPlaylistButton({ media, episodes, disabled = false }: Props) {
  const [open, setOpen] = useState(false)
  return <>
    <button type="button" className="btn-outline min-h-11 gap-2" disabled={disabled} onClick={() => setOpen(true)} aria-haspopup="dialog">
      <ListPlus size={16} aria-hidden="true" />加入播放列表
    </button>
    {open && <AddToPlaylistDialog media={media} episodes={episodes} onClose={() => setOpen(false)} />}
  </>
}

function AddToPlaylistDialog({ media, episodes, onClose }: Props & { onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const titleId = useId()
  const [targets] = useState(() => [...new Set((episodes?.length ? episodes : [media]).map(item => item.id).filter(Boolean))])
  const [items, setItems] = useState<Playlist[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState('')
  const [keyword, setKeyword] = useState('')
  const [name, setName] = useState('')
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    const overflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    dialog.current?.showModal()
    return () => { document.body.style.overflow = overflow; previous?.focus() }
  }, [])
  useEffect(() => {
    let canceled = false
    setLoading(true)
    setLoadError('')
    playbackAPI.listPlaylists(targets.length === 1 ? targets[0] : undefined)
      .then(data => { if (!canceled) setItems(data) })
      .catch(err => { if (!canceled) setLoadError(playlistErrorMessage(err)) })
      .finally(() => { if (!canceled) setLoading(false) })
    return () => { canceled = true }
  }, [attempt, targets])

  const add = async (playlist: Playlist) => {
    if (busy || targets.length === 0) return
    setBusy(playlist.id); setError('')
    try {
      const result = await playbackAPI.addToPlaylist(playlist.id, targets)
      toast.success(result.added > 0 ? `已加入「${playlist.name}」` : '影片已在此列表中')
      onClose()
    } catch (err) { setError(playlistErrorMessage(err)) } finally { setBusy('') }
  }
  const create = async (event: FormEvent) => {
    event.preventDefault()
    if (busy || !name.trim() || targets.length === 0) return
    setBusy('create'); setError('')
    try {
      const result = await playbackAPI.createPlaylist(name.trim(), false, targets)
      toast.success(`已创建并加入「${result.name}」`); onClose()
    } catch (err) { setError(playlistErrorMessage(err)) } finally { setBusy('') }
  }
  const visible = items.filter(item => item.name.toLocaleLowerCase().includes(keyword.toLocaleLowerCase()))

  return createPortal(<dialog ref={dialog} aria-labelledby={titleId}
    onCancel={event => { event.preventDefault(); if (!busy) onClose() }}
    className="m-auto w-[calc(100%-2rem)] max-w-lg max-h-[85dvh] overflow-hidden rounded-2xl border border-(--app-border) bg-(--app-panel) p-0 text-(--app-text) shadow-xl backdrop:bg-black/50">
    <div className="flex max-h-[85dvh] flex-col">
      <header className="flex items-start justify-between gap-4 border-b border-(--app-border) p-5">
        <div className="min-w-0"><h2 id={titleId} className="text-lg font-semibold">加入播放列表</h2>
          <p className="mt-1 break-words text-sm text-(--app-muted)">{media.title}{targets.length > 1 ? ` · 全部 ${targets.length} 集` : ''}</p>
        </div>
        <button type="button" aria-label="关闭加入播放列表" disabled={Boolean(busy)} onClick={onClose} className="btn-ghost min-h-11 min-w-11 p-2"><X size={18} /></button>
      </header>
      <div className="min-h-0 space-y-4 overflow-y-auto p-5">
        <p className="text-sm text-(--app-muted)">{targets.length > 1 ? '按季集顺序加入，列表中已有的影片会自动跳过。' : '选择一个列表，影片将加入列表末尾。'}</p>
        {loading ? <p role="status" className="flex items-center gap-2 py-4 text-sm"><Loader2 size={16} className="animate-spin" />正在加载列表…</p>
          : loadError ? <div role="alert" className="space-y-2 text-sm"><p>{loadError}</p><button type="button" className="btn-outline min-h-11" onClick={() => setAttempt(value => value + 1)}>重新加载</button></div>
          : items.length === 0 ? <p className="rounded-xl bg-(--app-panel-soft) p-4 text-sm text-(--app-muted)">还没有播放列表，在下方新建后即可加入。</p>
          : <section className="space-y-2" aria-label="选择已有播放列表">
            <label className="sr-only" htmlFor={`${titleId}-search`}>搜索播放列表</label>
            <input id={`${titleId}-search`} type="search" className="input-base w-full" value={keyword} onChange={event => setKeyword(event.target.value)} placeholder="搜索已有列表" disabled={Boolean(busy)} />
            <ul className="divide-y divide-(--app-border)">{visible.map(item => <li key={item.id}>
              <button type="button" disabled={Boolean(busy) || item.contains_media} onClick={() => void add(item)} className="flex min-h-16 w-full items-center justify-between gap-3 py-3 text-left disabled:cursor-default">
                <span className="min-w-0"><span className="block break-words font-medium">{item.name}</span><span className="mt-1 block text-xs text-(--app-muted)">{item.item_count ?? 0} 部影片{item.is_public ? ' · 公开列表' : ''}</span></span>
                <span className="flex shrink-0 items-center gap-1 text-sm text-(--app-muted)">{busy === item.id ? <Loader2 size={16} className="animate-spin" /> : item.contains_media ? <><Check size={16} />已加入</> : <><Plus size={16} />加入</>}</span>
              </button>
            </li>)}</ul>
            {visible.length === 0 && <p className="py-3 text-sm text-(--app-muted)">没有匹配的列表。</p>}
          </section>}
        <form onSubmit={create} className="space-y-3 border-t border-(--app-border) pt-4">
          <label htmlFor={`${titleId}-name`} className="block text-sm font-medium">新建播放列表</label>
          <input id={`${titleId}-name`} required maxLength={128} className="input-base w-full" value={name} onChange={event => setName(event.target.value)} placeholder="例如：周末电影" disabled={Boolean(busy)} />
          <button type="submit" className="btn-primary min-h-11 w-full" disabled={Boolean(busy) || !name.trim() || targets.length === 0}>{busy === 'create' ? <Loader2 size={16} className="animate-spin" /> : <Plus size={16} />}新建并加入</button>
        </form>
        {error && <p role="alert" className="text-sm text-(--app-danger-text)">{error}</p>}
      </div>
    </div>
  </dialog>, document.body)
}
