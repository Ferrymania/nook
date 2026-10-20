import { useEffect, useState } from 'react'
import { AlertCircle, ArrowUpCircle, Check, CheckCircle2, Copy, ExternalLink, Loader2, RefreshCw } from 'lucide-react'
import toast from 'react-hot-toast'

import { systemAPI, type ReleaseCheckStatus } from '../api/system'
import { ReleaseNotes } from '../components/ReleaseNotes'
import { formatAppVersion } from '../utils/formatAppVersion'

const RELEASES_URL = 'https://github.com/Ferrymania/nook/releases'
const UPDATE_COMMAND = 'docker compose pull nook\ndocker compose up -d --no-deps nook'

function formatDate(value?: string): string {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleString('zh-CN', { hour12: false })
}

export function VersionUpdatePanel() {
  const [result, setResult] = useState<ReleaseCheckStatus | null>(null)
  const [checking, setChecking] = useState(true)
  const [error, setError] = useState(false)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    systemAPI.release(false, controller.signal)
      .then((value) => { if (!controller.signal.aborted) setResult(value) })
      .catch(() => { if (!controller.signal.aborted) setError(true) })
      .finally(() => { if (!controller.signal.aborted) setChecking(false) })
    return () => controller.abort()
  }, [])

  useEffect(() => {
    if (!copied) return
    const timer = window.setTimeout(() => setCopied(false), 2000)
    return () => window.clearTimeout(timer)
  }, [copied])

  const check = async () => {
    setChecking(true)
    setError(false)
    try {
      setResult(await systemAPI.release(true))
    } catch {
      setResult(null)
      setError(true)
    } finally {
      setChecking(false)
    }
  }

  const copyCommand = async () => {
    try {
      await navigator.clipboard.writeText(UPDATE_COMMAND)
      setCopied(true)
      toast.success('更新命令已复制，请在服务器的 Compose 目录执行')
    } catch {
      toast.error('复制失败，请手动选择下方命令复制')
    }
  }

  const available = !error && result?.status === 'update_available'
  const failed = error || result?.status === 'unavailable'
  const Icon = checking ? Loader2 : failed ? AlertCircle : available ? ArrowUpCircle : result?.status === 'up_to_date' ? CheckCircle2 : RefreshCw
  const title = checking ? '正在检查更新…' : failed ? '暂时无法检查更新' : result?.message || '版本更新'

  return (
    <section className="glass-panel space-y-5 p-6 sm:p-8" aria-labelledby="version-update-heading">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="space-y-1">
          <h3 id="version-update-heading" className="font-display text-lg font-bold text-ink-600">版本更新</h3>
          <p className="text-sm text-sand-500">进入此页自动检查 GitHub 最新正式版本</p>
        </div>
        <button type="button" onClick={check} disabled={checking} className="library-toolbar-button">
          <RefreshCw size={16} className={checking ? 'animate-spin' : ''} aria-hidden="true" />
          {checking ? '检查中…' : '检查更新'}
        </button>
      </div>

      <div
        className={`rounded-2xl border p-4 sm:p-5 ${available ? 'border-brand-500/30 bg-brand-500/5' : 'border-[var(--app-border)] bg-[var(--app-panel-soft)]'}`}
        role="status" aria-live="polite" aria-busy={checking}
      >
        <div className="flex items-center gap-2 text-ink-600">
          <Icon size={20} className={`shrink-0 ${checking ? 'animate-spin' : ''} ${available ? 'text-brand-500' : ''}`} aria-hidden="true" />
          <p className="font-semibold">{title}</p>
          {available && !checking && <span className="shrink-0 whitespace-nowrap rounded-full bg-brand-500/10 px-2 py-0.5 text-xs font-semibold text-brand-600">可更新</span>}
        </div>
        {failed && !checking && (
          <p className="mt-2 text-sm leading-relaxed text-sand-500">
            {error ? '检查失败，请稍后重试。也可以前往 GitHub 发布页查看。' : result?.message}
          </p>
        )}
        {result && !error && (
          <div className="mt-4 flex flex-wrap gap-x-8 gap-y-3 text-sm">
            <div><span className="text-sand-500">当前版本</span><p className="mt-1 font-mono font-semibold text-ink-600">{formatAppVersion(result.current_version)}</p></div>
            {result.latest_version && <div><span className="text-sand-500">最新正式版</span><p className="mt-1 font-mono font-semibold text-ink-600">{formatAppVersion(result.latest_version)}</p></div>}
            {formatDate(result.published_at) && <div><span className="text-sand-500">发布时间</span><p className="mt-1 text-ink-600">{formatDate(result.published_at)}</p></div>}
          </div>
        )}
      </div>

      {available && result?.release_notes && (
        <details className="rounded-xl border border-[var(--app-border)] px-4 py-3" open={available || undefined}>
          <summary className="cursor-pointer text-sm font-semibold text-ink-600">
            最新正式版{result.latest_version && ` ${formatAppVersion(result.latest_version)}`} 更新说明
          </summary>
          <ReleaseNotes>{result.release_notes}</ReleaseNotes>
        </details>
      )}

      <div className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h4 className="text-sm font-semibold text-ink-600">Docker Compose 更新</h4>
          <button type="button" onClick={copyCommand} className="library-toolbar-button">
            {copied ? <Check size={15} aria-hidden="true" /> : <Copy size={15} aria-hidden="true" />}
            {copied ? '已复制更新命令' : '复制更新命令'}
          </button>
        </div>
        <pre className="overflow-x-auto rounded-xl border border-[var(--app-border)] bg-[var(--app-panel-soft)] p-4 text-xs leading-7 text-ink-600 sm:text-sm"><code>{UPDATE_COMMAND}</code></pre>
        <p className="text-xs leading-relaxed text-sand-500">在服务器的 Compose 文件所在目录执行。通过容器管理面板部署时，可在面板中更新；二进制部署请前往发布页下载。</p>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--app-border)] pt-4 text-xs text-sand-500">
        <p>{result ? `检查于 ${formatDate(result.checked_at)} · 手动检查间隔至少 1 分钟` : error ? '可稍后重试或前往发布页查看' : '检查结果缓存 30 分钟'}</p>
        <a href={result?.release_url || RELEASES_URL} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1.5 font-medium text-brand-600 hover:underline">
          查看 GitHub 发布页 <ExternalLink size={13} aria-hidden="true" />
        </a>
      </div>
    </section>
  )
}
