import { useEffect, useState } from 'react'
import {
  Check,
  Copy,
  Cpu,
  ExternalLink,
  Github,
  HardDrive,
  Server,
  Tag,
} from 'lucide-react'
import toast from 'react-hot-toast'

import { systemAPI } from '../api/system'
import { formatAppVersion } from '../utils/formatAppVersion'
import { VersionUpdatePanel } from './VersionUpdatePanel'

type SystemInfo = {
  name: string
  version: string
  go: string
  os: string
  arch: string
  data_dir: string
  cache_dir: string
}

const GITHUB_REPO_URL = 'https://github.com/Ferrymania/nook'

export function AboutSettingsPanel() {
  const [info, setInfo] = useState<SystemInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    systemAPI
      .info()
      .then((res) => {
        setInfo({
          name: res.name || 'Nook',
          version: res.version || 'dev',
          go: res.go || '',
          os: res.os || '',
          arch: res.arch || '',
          data_dir: res.data_dir || '',
          cache_dir: res.cache_dir || '',
        })
      })
      .catch(() => {
        setInfo({
          name: 'Nook',
          version: '未知',
          go: '',
          os: '',
          arch: '',
          data_dir: '',
          cache_dir: '',
        })
      })
      .finally(() => setLoading(false))
  }, [])

  const handleCopyGitURL = async () => {
    try {
      await navigator.clipboard.writeText(GITHUB_REPO_URL)
      setCopied(true)
      toast.success('GitHub 地址已复制到剪贴板')
      setTimeout(() => setCopied(false), 2000)
    } catch {
      toast.error('复制失败，请手动复制')
    }
  }

  return (
    <div className="space-y-6">
      {/* 顶部概览与项目卡片 */}
      <div className="glass-panel overflow-hidden p-6 sm:p-8">
        <div className="flex flex-col gap-6 lg:flex-row lg:items-center lg:justify-between">
          <div className="space-y-3">
            <div className="flex flex-wrap items-center gap-3">
              <h2 className="font-display text-2xl sm:text-3xl font-bold text-ink-600">
                Nook 影音管理系统
              </h2>
              <span className="inline-flex items-center gap-1.5 rounded-full bg-brand-500/10 px-3 py-1 text-xs font-semibold text-brand-600 border border-brand-500/20">
                <Tag size={13} />
                {loading ? '加载中...' : formatAppVersion(info?.version)}
              </span>
            </div>
            <p className="max-w-2xl text-sm leading-relaxed text-sand-600">
              Nook 是一款专为个人与家庭设计的现代化开源媒体库管理与流媒体点播系统。
              集媒体资源归纳、智能多源刮削、弹幕交互、海量云盘 STRM 转存与多端播放于一体，让观影体验更轻快自由。
            </p>
          </div>

          {/* 仓库链接操作区 */}
          <div className="flex flex-wrap items-center gap-3">
            <a
              href={GITHUB_REPO_URL}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-2 rounded-xl bg-ink-600 px-4 py-2.5 text-sm font-medium text-white shadow-sm transition hover:bg-ink-700 hover:shadow"
            >
              <Github size={16} />
              <span>访问 GitHub 仓库</span>
              <ExternalLink size={14} className="opacity-70" />
            </a>
            <button
              type="button"
              onClick={handleCopyGitURL}
              className="inline-flex items-center gap-2 rounded-xl border border-gray-200 bg-white/80 px-4 py-2.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white hover:text-brand-600"
            >
              {copied ? <Check size={16} className="text-emerald-500" /> : <Copy size={16} />}
              <span>{copied ? '已复制' : '复制地址'}</span>
            </button>
          </div>
        </div>

        {/* 运行环境规格栏 */}
        <div className="mt-6 grid grid-cols-2 gap-3 sm:grid-cols-4 border-t border-gray-200/60 pt-6">
          <div className="rounded-xl bg-sand-100/50 p-3.5">
            <div className="flex items-center gap-1.5 text-xs text-sand-500">
              <Tag size={13} />
              <span>系统版本</span>
            </div>
            <p className="mt-1 font-mono text-sm font-semibold text-ink-600">
              {loading ? '...' : formatAppVersion(info?.version)}
            </p>
          </div>
          <div className="rounded-xl bg-sand-100/50 p-3.5">
            <div className="flex items-center gap-1.5 text-xs text-sand-500">
              <Server size={13} />
              <span>运行环境</span>
            </div>
            <p className="mt-1 font-mono text-sm font-semibold text-ink-600">
              {loading ? '...' : `${info?.os || 'linux'} / ${info?.arch || 'amd64'}`}
            </p>
          </div>
          <div className="rounded-xl bg-sand-100/50 p-3.5">
            <div className="flex items-center gap-1.5 text-xs text-sand-500">
              <Cpu size={13} />
              <span>Go 运行时</span>
            </div>
            <p className="mt-1 font-mono text-sm font-semibold text-ink-600">
              {loading ? '...' : info?.go || 'go1.x'}
            </p>
          </div>
          <div className="rounded-xl bg-sand-100/50 p-3.5">
            <div className="flex items-center gap-1.5 text-xs text-sand-500">
              <HardDrive size={13} />
              <span>数据目录</span>
            </div>
            <p className="mt-1 truncate font-mono text-xs font-semibold text-ink-600" title={info?.data_dir}>
              {loading ? '...' : info?.data_dir || './data'}
            </p>
          </div>
        </div>
      </div>

      <VersionUpdatePanel />
    </div>
  )
}
