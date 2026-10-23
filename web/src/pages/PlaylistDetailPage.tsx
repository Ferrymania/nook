import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ArrowDown, ArrowUp, Loader2, Pencil, Play, X } from 'lucide-react'
import toast from 'react-hot-toast'
import { playbackAPI, type PlaylistDetail } from '../api/playback'
import { ARTWORK, imageURL } from '../api/client'
import { ArtworkImage } from '../components/ArtworkImage'
import { PageHeader } from '../components/PageHeader'
import { artworkSources } from '../utils/artworkSources'
import { playlistErrorMessage, playlistMediaMeta } from '../utils/playlists'

export function PlaylistDetailPage() {
  const { id = '' } = useParams()
  const [detail, setDetail] = useState<PlaylistDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [renaming, setRenaming] = useState(false)
  const [name, setName] = useState('')
  const [limit, setLimit] = useState(50)
  const refresh = useCallback(async () => {
    const next = await playbackAPI.getPlaylist(id)
    setDetail(next); setError('')
    return next
  }, [id])
  useEffect(() => {
    let canceled = false
    setLoading(true); setError(''); setDetail(null); setRenaming(false); setLimit(50)
    playbackAPI.getPlaylist(id).then(data => { if (!canceled) setDetail(data) })
      .catch(err => { if (!canceled) setError(playlistErrorMessage(err)) })
      .finally(() => { if (!canceled) setLoading(false) })
    return () => { canceled = true }
  }, [id])
  const change = async (action: () => Promise<unknown>, message: string) => {
    if (busy) return
    setBusy(true)
    try { await action(); await refresh(); toast.success(message) }
    catch (err) { toast.error(playlistErrorMessage(err)) } finally { setBusy(false) }
  }
  const rename = async (event: FormEvent) => {
    event.preventDefault()
    if (!name.trim()) return
    await change(async () => { await playbackAPI.renamePlaylist(id, name.trim()); setRenaming(false) }, '已保存名称')
  }
  const move = async (index: number, delta: number) => {
    if (!detail || index + delta < 0 || index + delta >= detail.items.length) return
    const order = detail.items.map(item => item.id)
    ;[order[index], order[index + delta]] = [order[index + delta], order[index]]
    await change(() => playbackAPI.reorderPlaylist(id, order), '已更新播放顺序')
  }
  const playURL = (mediaId: string) => `/play/${encodeURIComponent(mediaId)}?playlist=${encodeURIComponent(id)}`
  return <div className="space-y-6">
    <PageHeader backTo="/playlists" backLabel="播放列表" title={detail?.playlist.name || '播放列表'}
      description={detail ? `${detail.items.length} 部影片 · 按列表顺序播放` : undefined}
      actions={detail && <>
        {detail.items.length > 0 && <Link className="btn-primary min-h-11" to={playURL(detail.items[0].id)} state={{ from: `/playlist/${id}` }}><Play size={16} fill="currentColor" />按顺序播放</Link>}
        {detail.can_edit && <button type="button" className="btn-outline min-h-11" disabled={busy} onClick={() => { setName(detail.playlist.name); setRenaming(value => !value) }}><Pencil size={16} />重命名</button>}
      </>} />
    {renaming && <form onSubmit={rename} className="space-y-3 rounded-2xl border border-(--app-border) p-4">
      <label className="block text-sm font-medium" htmlFor="rename-playlist">列表名称</label>
      <input autoFocus id="rename-playlist" maxLength={128} required value={name} onChange={event => setName(event.target.value)} disabled={busy} className="input-base w-full" />
      <div className="flex gap-2"><button className="btn-primary min-h-11" disabled={busy || !name.trim()} type="submit">保存名称</button><button className="btn-outline min-h-11" disabled={busy} type="button" onClick={() => setRenaming(false)}>取消</button></div>
    </form>}
    {loading ? <p role="status" className="flex items-center gap-2 py-8 text-sm text-(--app-muted)"><Loader2 size={16} className="animate-spin" />正在加载列表…</p>
      : error ? <section role="alert" className="space-y-3 rounded-2xl border border-(--app-border) p-5"><p>无法打开播放列表：{error}</p><button type="button" className="btn-outline min-h-11" onClick={() => void refresh().catch(err => setError(playlistErrorMessage(err)))}>重试</button></section>
      : detail?.items.length === 0 ? <section className="space-y-3 rounded-2xl border border-(--app-border) p-6"><h2 className="font-medium">列表中还没有可观看的影片</h2><p className="text-sm text-(--app-muted)">打开媒体详情页，点击「加入播放列表」。已移除或无权访问的影片不会显示。</p><Link to="/libraries" className="btn-outline min-h-11">浏览媒体库</Link></section>
      : detail && <>
        {!detail.can_edit && <p className="text-sm text-(--app-muted)">公开播放列表，仅创建者或管理员可以编辑。</p>}
        <ol aria-label="影片播放顺序" aria-busy={busy} className="divide-y divide-(--app-border) rounded-2xl border border-(--app-border) bg-(--app-panel) px-3 sm:px-5">
          {detail.entries.slice(0, limit).map((entry, index) => <li key={entry.id} className="grid grid-cols-[1.5rem_2.75rem_minmax(0,1fr)] items-center gap-x-3 gap-y-1 py-3 sm:flex sm:gap-3">
            <span className="w-6 shrink-0 text-right text-sm tabular-nums text-(--app-muted)">{index + 1}</span>
            <Link to={`/media/${encodeURIComponent(entry.media.id)}`} state={{ from: `/playlist/${id}` }} className="shrink-0"><ArtworkImage sources={artworkSources(entry.media, 'poster').map(source => imageURL(source, entry.media.updated_at, ARTWORK.posterTiny))} alt={entry.media.title} compact className="h-16 w-11 rounded-md object-cover" fallbackClassName="h-16 w-11 rounded-md" /></Link>
            <Link to={`/media/${encodeURIComponent(entry.media.id)}`} state={{ from: `/playlist/${id}` }} className="min-w-0 flex-1"><span className="block break-words font-medium">{entry.media.title}</span><span className="mt-1 block text-xs text-(--app-muted)">{playlistMediaMeta(entry.media) || '影片'}</span></Link>
            <div className="col-start-3 ml-auto flex items-center gap-1">
              <Link to={playURL(entry.media.id)} state={{ from: `/playlist/${id}` }} className="btn-ghost min-h-11 min-w-11 p-2" aria-label={`播放「${entry.media.title}」`} title="从这里播放"><Play size={16} /></Link>
              {detail.can_edit && <>
                <button type="button" disabled={busy || index === 0} className="btn-ghost min-h-11 min-w-11 p-2 disabled:opacity-40" onClick={() => void move(index, -1)} aria-label={`上移「${entry.media.title}」`} title="上移"><ArrowUp size={16} /></button>
                <button type="button" disabled={busy || index === detail.items.length - 1} className="btn-ghost min-h-11 min-w-11 p-2 disabled:opacity-40" onClick={() => void move(index, 1)} aria-label={`下移「${entry.media.title}」`} title="下移"><ArrowDown size={16} /></button>
                <button type="button" disabled={busy} className="btn-ghost min-h-11 min-w-11 p-2" onClick={() => void change(() => playbackAPI.removePlaylistEntry(id, entry.id), '已从列表移除')} aria-label={`移除「${entry.media.title}」`} title="从列表移除"><X size={16} /></button>
              </>}
            </div>
          </li>)}
        </ol>
        {detail.items.length > limit && <button type="button" className="btn-outline min-h-11" onClick={() => setLimit(value => value + 50)}>显示更多（已显示 {limit} / {detail.items.length}）</button>}
      </>}
  </div>
}
