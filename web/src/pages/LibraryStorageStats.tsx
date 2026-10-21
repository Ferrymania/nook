import { useCallback, useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'

import { storageAPI, type StorageBreakdown } from '../api/storage'
import { formatKnownDuration, formatKnownVideoSize } from '../utils/mediaStatistics'
import './LibraryStorageStats.css'

export function LibraryStorageStats() {
  const [data, setData] = useState<StorageBreakdown | null>(null)
  const [refreshing, setRefreshing] = useState(true)
  const [error, setError] = useState(false)
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null)

  const loadData = useCallback(async (signal?: AbortSignal) => {
    setRefreshing(true)
    setError(false)
    try {
      const next = await storageAPI.breakdown(signal)
      if (signal?.aborted) return
      setData(next)
      setUpdatedAt(new Date())
    } catch {
      if (!signal?.aborted) setError(true)
    } finally {
      if (!signal?.aborted) setRefreshing(false)
    }
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    void loadData(controller.signal)
    return () => controller.abort()
  }, [loadData])

  const loading = refreshing && !data
  const count = data?.media_count ?? 0
  const known = data?.known_size_count ?? 0
  const unknown = data?.unknown_size_count ?? 0
  const number = (n: number) => n.toLocaleString('zh-CN')
  const value = (text: string) => loading ? '…' : data ? text : '—'

  return (
    <section className="nook-media-statistics" aria-labelledby="media-statistics-heading" aria-busy={refreshing}>
      <div className="nook-statistics-heading">
        <div>
          <h2 id="media-statistics-heading">媒体统计</h2>
          <p>{data ? `${number(data.by_library.length)} 个媒体库` : loading ? '正在读取已保存的媒体信息…' : '统计暂不可用'}</p>
        </div>
        <div className="nook-statistics-refresh">
          {updatedAt && <span>更新于 {updatedAt.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}</span>}
          <button type="button" onClick={() => void loadData()} disabled={refreshing}>
            <RefreshCw size={15} className={refreshing ? 'animate-spin motion-reduce:animate-none' : ''} aria-hidden="true" />
            {refreshing ? '刷新中' : error ? '重试' : '刷新'}
          </button>
        </div>
      </div>

      {error && <p className="nook-statistics-error" role="alert">{data ? '刷新失败，仍显示上次统计。请重试。' : '无法加载媒体统计，请重试。'}</p>}

      <dl className="nook-statistics-summary">
        <div><dt>已知视频大小</dt><dd>{value(formatKnownVideoSize(data?.total_bytes ?? 0, known, count))}</dd></div>
        <div><dt>媒体条目</dt><dd>{value(number(count))}</dd></div>
        <div><dt>已知总时长</dt><dd>{value(formatKnownDuration(data?.total_seconds ?? 0, count))}</dd></div>
      </dl>

      {data && <p className="nook-statistics-coverage">大小已知 <span>{number(known)} / {number(count)}</span> 条<span className="nook-statistics-unknown">{number(unknown)} 条未获取</span></p>}

      <table className="nook-statistics-table">
        <caption className="sr-only">按媒体库统计</caption>
        <thead><tr><th scope="col">媒体库</th><th scope="col">条目</th><th scope="col">视频大小</th><th scope="col">大小已知</th></tr></thead>
        <tbody>
          {!data || data.by_library.length === 0 ? (
            <tr><td colSpan={4} className="nook-statistics-empty">{loading ? '正在加载统计…' : !data ? '统计暂不可用' : '暂无媒体库，添加后可在这里查看统计。'}</td></tr>
          ) : data.by_library.map((library) => (
            <tr key={library.library_id}>
              <th scope="row">{library.name}</th>
              <td data-label="条目">{number(library.media_count)}</td>
              <td data-label="视频大小">{formatKnownVideoSize(library.total_bytes, library.known_size_count, library.media_count)}</td>
              <td data-label="大小已知">{number(library.known_size_count)} / {number(library.media_count)}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <details className="nook-statistics-definition">
        <summary>统计口径</summary>
        <p>视频大小仅累计已保存的源视频大小，不包含 STRM 文本文件。未获取的大小显示为“—”，不计入合计；这些数字不代表服务器磁盘占用。</p>
        <p>条目按媒体文件记录计数，剧集按单集统计，不同版本分别计数。总时长仅累计已知时长。刷新只读取已保存的信息，不会扫描文件或发起媒体探测。</p>
      </details>
    </section>
  )
}
