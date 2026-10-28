import { useEffect, useState } from 'react'
import { AlertCircle, ArrowUpCircle, Check, CheckCircle2, Copy, ExternalLink, Loader2, RefreshCw } from 'lucide-react'
import toast from 'react-hot-toast'

import { systemAPI, type ReleaseCheckStatus } from '../api/system'
import { ReleaseNotes } from '../components/ReleaseNotes'
import { formatAppVersion } from '../utils/formatAppVersion'

const RELEASES_URL = 'https://github.com/Ferrymania/nook/releases'
const UPDATE_COMMAND = 'docker compose pull nook\ndocker compose up -d --no-deps nook'
const UPDATE_COMMAND_LINES = UPDATE_COMMAND.split('\n')

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
  const title = checking ? '正在检查更新…' : failed ? '暂时无法检查更新'
    : available && result?.latest_version ? `发现新版本 ${formatAppVersion(result.latest_version)}`
      : result?.message || '版本状态未知'
  const showLatest = !available && result?.latest_version && result.status !== 'up_to_date'

  return (
    <section className="about-panel about-update" aria-label="版本更新">
      <div className="about-update-heading">
        <div className={`about-release-state${available ? ' about-release-available' : ''}`} role="status" aria-live="polite" aria-busy={checking}>
          <div className="about-release-title">
            <Icon size={20} className={`shrink-0 ${checking ? 'animate-spin' : ''}`} aria-hidden="true" />
            <h3>{title}</h3>
          </div>
          {failed && !checking && (
            <p className="about-release-note">
              {error ? '检查失败，请稍后重试。也可以前往 GitHub 发布页查看。' : result?.message}
            </p>
          )}
          {result && !error && !checking && !failed && (
            <div className="about-release-note">
              {showLatest && <span>最新正式版 {formatAppVersion(result.latest_version)}</span>}
              {formatDate(result.published_at) && <span>发布于 {formatDate(result.published_at)}</span>}
            </div>
          )}
        </div>
        <button type="button" onClick={check} disabled={checking} className="about-button about-button-primary">
          <RefreshCw size={16} className={checking ? 'animate-spin' : ''} aria-hidden="true" />
          {checking ? '检查中…' : '检查更新'}
        </button>
      </div>

      {available && result?.release_notes && (
        <details className="about-release-notes" open={available || undefined}>
          <summary>
            更新说明
          </summary>
          <ReleaseNotes>{result.release_notes}</ReleaseNotes>
        </details>
      )}

      <div className="about-command">
        <button type="button" onClick={copyCommand} className="about-button about-command-copy"
          aria-label={copied ? '已复制命令' : '复制命令'} title={copied ? '已复制命令' : '复制命令'}>
          {copied ? <Check size={16} aria-hidden="true" /> : <Copy size={16} aria-hidden="true" />}
        </button>
        <pre><code><span>{UPDATE_COMMAND_LINES[0]}</span>{'\n'}{UPDATE_COMMAND_LINES.slice(1).join('\n')}</code></pre>
      </div>

      <div className="about-update-footer">
        <p>{result ? `检查于 ${formatDate(result.checked_at)}` : error ? '检查失败，可重新检查' : '正在读取版本信息'}</p>
        <a href={result?.release_url || RELEASES_URL} target="_blank" rel="noreferrer" className="about-text-link">
          发布记录 <ExternalLink size={13} aria-hidden="true" />
        </a>
      </div>
    </section>
  )
}
