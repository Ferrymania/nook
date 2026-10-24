import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { ChevronRight, ListMusic, Loader2, Plus, Trash2 } from 'lucide-react'
import toast from 'react-hot-toast'
import { playbackAPI } from '../api/playback'
import { confirmAction } from '../components/confirmAction'
import { PageHeader } from '../components/PageHeader'
import { playlistErrorMessage } from '../utils/playlists'
import type { Playlist } from '../types'

export function PlaylistsPage() {
  const [items, setItems] = useState<Playlist[]>([])
  const [name, setName] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState('')
  const refresh = useCallback(async () => {
    const next = await playbackAPI.listPlaylists()
    setItems(next); setError('')
  }, [])
  useEffect(() => {
    let canceled = false
    playbackAPI.listPlaylists().then(data => { if (!canceled) setItems(data) })
      .catch(err => { if (!canceled) setError(playlistErrorMessage(err)) })
      .finally(() => { if (!canceled) setLoading(false) })
    return () => { canceled = true }
  }, [])
  const create = async (event: FormEvent) => {
    event.preventDefault()
    if (busy || !name.trim()) return
    setBusy('create')
    try { await playbackAPI.createPlaylist(name.trim()); setName(''); await refresh(); toast.success('已创建播放列表') }
    catch (err) { toast.error(playlistErrorMessage(err)) } finally { setBusy('') }
  }
  const remove = async (playlist: Playlist) => {
    if (busy || !await confirmAction({ title: '删除播放列表', message: `删除「${playlist.name}」及其列表记录？影片文件和观看进度会保留。`, confirmText: '删除' })) return
    setBusy(playlist.id)
    try { await playbackAPI.deletePlaylist(playlist.id); await refresh(); toast.success('已删除播放列表') }
    catch (err) { toast.error(playlistErrorMessage(err)) } finally { setBusy('') }
  }
  return <div className="space-y-6">
    <PageHeader title="播放列表" actions={
      <form onSubmit={create} className="flex w-full min-w-0 items-center gap-2">
        <label htmlFor="playlist-name" className="shrink-0 text-sm">名称</label>
        <input id="playlist-name" required maxLength={128} className="input-base min-w-0 flex-1 sm:w-56" placeholder="新播放列表" value={name} onChange={event => setName(event.target.value)} disabled={Boolean(busy)} />
        <button type="submit" className="btn-primary min-h-11 shrink-0" disabled={Boolean(busy) || !name.trim()}>{busy === 'create' ? <Loader2 size={16} className="animate-spin" /> : <Plus size={16} />}新建</button>
      </form>
    } />
    {loading ? <p role="status" className="flex items-center gap-2 py-8 text-sm text-(--app-muted)"><Loader2 size={16} className="animate-spin" />正在加载播放列表…</p>
      : error ? <div role="alert" className="space-y-3 rounded-2xl border border-(--app-border) p-5"><p>{error}</p><button className="btn-outline min-h-11" onClick={() => void refresh().catch(err => setError(playlistErrorMessage(err)))}>重新加载</button></div>
      : items.length === 0 ? <p role="status" className="rounded-xl border border-(--app-border) px-5 py-8 text-center text-sm text-(--app-muted)">暂无播放列表</p> : <section aria-label="我的播放列表">
        <p className="mb-3 text-sm text-(--app-muted)">{items.length} 个列表</p>
        <ul className="divide-y divide-(--app-border) rounded-2xl border border-(--app-border) bg-(--app-panel) px-4 sm:px-5">
          {items.map(item => <li key={item.id} className="flex items-center gap-3 py-4">
            <ListMusic size={20} className="shrink-0 text-(--app-muted)" aria-hidden="true" />
            <Link to={`/playlist/${item.id}`} className="min-w-0 flex-1 space-y-1"><span className="block break-words font-medium">{item.name}</span><span className="block text-sm text-(--app-muted)">{item.item_count ?? 0} 部影片{item.is_public ? ' · 公开列表' : ''}</span></Link>
            <button type="button" aria-label={`删除播放列表「${item.name}」`} title="删除播放列表" disabled={Boolean(busy)} onClick={() => void remove(item)} className="btn-ghost min-h-11 min-w-11 p-2 text-(--app-muted)">{busy === item.id ? <Loader2 size={16} className="animate-spin" /> : <Trash2 size={16} />}</button>
            <Link to={`/playlist/${item.id}`} aria-label={`打开播放列表「${item.name}」`} className="btn-ghost min-h-11 min-w-11 p-2"><ChevronRight size={18} /></Link>
          </li>)}
        </ul>
      </section>}
  </div>
}
