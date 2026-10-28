import { useEffect, useState } from 'react'
import { Check, Copy, ExternalLink, Github, RefreshCw } from 'lucide-react'
import toast from 'react-hot-toast'

import { systemAPI } from '../api/system'
import { formatAppVersion } from '../utils/formatAppVersion'
import { VersionUpdatePanel } from './VersionUpdatePanel'
import { DiagnosticsPanel } from './DiagnosticsPanel'
import './AboutSettingsPanel.css'

type SystemInfo = Awaited<ReturnType<typeof systemAPI.info>>
const GITHUB_REPO_URL = 'https://github.com/Ferrymania/nook'

export function AboutSettingsPanel() {
  const [info, setInfo] = useState<SystemInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [revision, setRevision] = useState(0)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    let disposed = false
    setLoading(true)
    setError(false)
    systemAPI.info()
      .then((value) => { if (!disposed) setInfo(value) })
      .catch(() => { if (!disposed) setError(true) })
      .finally(() => { if (!disposed) setLoading(false) })
    return () => { disposed = true }
  }, [revision])

  useEffect(() => {
    if (!copied) return
    const timer = window.setTimeout(() => setCopied(false), 2000)
    return () => window.clearTimeout(timer)
  }, [copied])

  const environment = [
    { label: '系统', value: [info?.os, info?.arch].filter(Boolean).join(' / ') },
    { label: 'Go', value: info?.go },
    { label: '数据目录', value: info?.data_dir, path: true },
    { label: '缓存目录', value: info?.cache_dir, path: true },
  ]
  const copyInfo = async () => {
    if (!info) return
    try {
      await navigator.clipboard.writeText([
        `${info.name || 'Nook'} ${formatAppVersion(info.version)}`,
        ...environment.map((row) => `${row.label}: ${row.value || '—'}`),
      ].join('\n'))
      setCopied(true)
      toast.success('运行信息已复制')
    } catch {
      toast.error('复制失败，请手动选择运行信息复制')
    }
  }

  return (
    <div className="about-settings">
      <header className="about-identity">
        <div className="about-product">
          <img src="/brand/logo-64.png" alt="" width={48} height={48} />
          <div className="about-product-name">
            <h2>{info?.name || 'Nook'}</h2>
            <span className={`about-version${loading && !info ? ' about-placeholder' : ''}`}>
              {loading && !info ? '读取版本…' : formatAppVersion(info?.version)}
            </span>
          </div>
        </div>
        <a href={GITHUB_REPO_URL} target="_blank" rel="noreferrer" className="about-button">
          <Github size={16} aria-hidden="true" />GitHub<ExternalLink size={14} aria-hidden="true" />
        </a>
      </header>

      <div className="about-overview">
        <VersionUpdatePanel />
        <section className="about-panel about-environment" aria-labelledby="about-environment-heading" aria-busy={loading}>
          <div className="about-panel-heading">
            <h3 id="about-environment-heading">运行环境</h3>
            <button type="button" className="about-button about-button-quiet" disabled={!info || loading} onClick={() => { void copyInfo() }} aria-label={copied ? '运行信息已复制' : '复制运行信息'}>
              {copied ? <Check size={16} aria-hidden="true" /> : <Copy size={16} aria-hidden="true" />}
              {copied ? '已复制' : '复制信息'}
            </button>
          </div>
          <dl className="about-environment-list">
            {environment.map((row) => <div key={row.label}>
              <dt>{row.label}</dt>
              <dd title={row.value || undefined} className={`${row.path ? 'about-path ' : ''}${loading && !info ? 'about-placeholder' : ''}`}>
                {loading && !info ? '读取中…' : row.value || '—'}
              </dd>
            </div>)}
          </dl>
          {error && <div className="about-error" role="alert">
            <span>无法加载运行信息</span>
            <button type="button" className="about-button about-button-quiet" onClick={() => setRevision((value) => value + 1)}><RefreshCw size={14} aria-hidden="true" />重试</button>
          </div>}
        </section>
      </div>
      <DiagnosticsPanel />
    </div>
  )
}
