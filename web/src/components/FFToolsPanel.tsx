import { useCallback, useEffect, useId, useState } from 'react'
import toast from 'react-hot-toast'
import { CheckCircle2, Download, Loader2, XCircle } from 'lucide-react'

import { toolsAPI, type FFmpegToolInfo, type FFmpegToolsStatus } from '../api/tools'

interface FFToolsPanelProps {
  values: Record<string, string>
  dirty: Set<string>
  saving: boolean
  statusRevision: number
  onChange: (key: string, value: string) => void
  onInstalled: () => Promise<void>
}

export function FFToolsPanel({ values, dirty, saving, statusRevision, onChange, onInstalled }: FFToolsPanelProps) {
  const [status, setStatus] = useState<FFmpegToolsStatus | null>(null)
  const [checking, setChecking] = useState(true)
  const [statusError, setStatusError] = useState(false)
  const [working, setWorking] = useState(false)

  const load = useCallback(async () => {
    setChecking(true)
    try {
      setStatus(await toolsAPI.ffToolsStatus())
      setStatusError(false)
    } catch {
      setStatusError(true)
    } finally {
      setChecking(false)
    }
  }, [])

  useEffect(() => {
    load().catch(() => undefined)
  }, [load, statusRevision])

  const installing = Boolean(status?.installing)

  const syncInstalledPaths = useCallback(async () => {
    try {
      await onInstalled()
      toast.success('FFmpeg / FFprobe 已安装，路径已自动保存并生效')
    } catch {
      toast.error('工具已安装，但路径同步失败；请刷新页面后查看')
    }
  }, [onInstalled])

  // 只同步安装后的两个路径，保留页面中其他尚未保存的修改。
  useEffect(() => {
    if (!installing) return
    let cancelled = false
    let timer: number
    const poll = async () => {
      try {
        const next = await toolsAPI.ffToolsStatus()
        if (cancelled) return
        if (!next.installing) {
          if (next.error) {
            toast.error(`FFmpeg 下载安装失败：${next.error}`)
          } else {
            setWorking(true)
            await syncInstalledPaths()
            if (cancelled) return
            setWorking(false)
          }
          setStatus(next)
          return
        }
        setStatus(next)
      } catch {
        // 轮询失败继续等下一轮
      }
      if (!cancelled) timer = window.setTimeout(poll, 1500)
    }
    timer = window.setTimeout(poll, 1500)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [installing, syncInstalledPaths])

  const install = async () => {
    setWorking(true)
    try {
      const next = await toolsAPI.installFFTools()
      setStatus(next)
      if (next.error && !next.installing) {
        toast.error(next.error)
      } else if (next.installing) {
        toast('开始下载，完成后自动生效')
      } else {
        await syncInstalledPaths()
      }
    } catch (err) {
      toast.error(
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
          (err as { message?: string })?.message ??
          '启动下载失败',
      )
    } finally {
      setWorking(false)
    }
  }

  const busy = working || installing
  const pathsDirty = dirty.has('ffmpeg.path') || dirty.has('ffprobe.path')
  const targetLabel = status?.target?.label ?? (statusError ? '未知' : '检测中…')
  const ffmpegInstalled = Boolean(status?.ffmpeg?.installed)
  const ffprobeInstalled = Boolean(status?.ffprobe?.installed)

  return (
    <section className="glass-panel space-y-4" aria-label="FFmpeg / FFprobe 工具">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="font-display text-lg font-bold text-ink-600">FFmpeg / FFprobe 工具</div>
          <div className="text-xs text-sand-500">当前平台：{targetLabel}</div>
        </div>
        <button
          type="button"
          onClick={install}
          disabled={busy || saving || checking || statusError || pathsDirty}
          className="neon-button shrink-0 disabled:opacity-50"
        >
          {busy ? <Loader2 size={16} className="animate-spin" /> : <Download size={16} />}
          {busy ? '下载安装中…' : '下载并安装'}
        </button>
      </div>

      <p className="text-xs text-sand-500">
        路径留空时自动查找系统工具；手动修改后点击页面底部「保存」生效。
      </p>
      <div className="grid gap-3 md:grid-cols-2">
        {(['ffmpeg', 'ffprobe'] as const).map((tool) => (
          <ToolRow
            key={tool}
            name={tool === 'ffmpeg' ? 'FFmpeg' : 'FFprobe'}
            tool={tool}
            info={status?.[tool]}
            value={values[`${tool}.path`] ?? ''}
            pending={dirty.has(`${tool}.path`)}
            checking={checking}
            statusError={statusError}
            disabled={busy || saving}
            onChange={(value) => onChange(`${tool}.path`, value)}
          />
        ))}
      </div>

      {statusError && (
        <div className="flex items-center gap-3 text-xs text-red-600" role="alert">
          工具状态获取失败，当前使用路径尚未确认。
          <button type="button" onClick={load} disabled={checking} className="underline disabled:opacity-50">重新检测</button>
        </div>
      )}

      {(status?.message || status?.error) && (
        <div
          className={
            status?.error
              ? 'rounded-xl bg-red-50 px-3 py-2 text-xs text-red-600'
              : 'rounded-xl bg-sand-100/70 px-3 py-2 text-xs text-sand-500'
          }
        >
          {status?.error || status?.message}
        </div>
      )}

      <div className="space-y-1 border-t border-gray-200 pt-3 text-xs text-sand-500">
        {!checking && !statusError && <p>
          {pathsDirty
            ? '请先保存路径修改，再使用「下载并安装」。'
            : ffmpegInstalled && ffprobeInstalled
              ? '已检测到可用工具，可直接使用，无需另行下载。'
              : '如需安装，可点击「下载并安装」，完成后自动保存路径并生效。'}
        </p>}
        <p className="break-all">
          下载保存目录：<span className="font-mono">{status?.install_dir ?? '…'}</span>（仅下载安装时使用）
        </p>
      </div>
    </section>
  )
}

interface ToolRowProps {
  name: string
  tool: string
  info?: FFmpegToolInfo
  value: string
  pending: boolean
  checking: boolean
  statusError: boolean
  disabled: boolean
  onChange: (value: string) => void
}

function ToolRow({ name, tool, info, value, pending, checking, statusError, disabled, onChange }: ToolRowProps) {
  const id = useId()
  const confirmed = !checking && !statusError
  return (
    <div className="min-w-0 space-y-3 rounded-xl border border-gray-200 bg-white/70 p-4">
      <div className="flex items-center justify-between gap-2">
        <div className="font-medium text-ink-600">{name}</div>
        <span className="flex items-center gap-1 text-xs text-ink-50" aria-live="polite">
          {checking ? <Loader2 size={14} className="animate-spin" /> : confirmed && (info?.installed
            ? <CheckCircle2 size={14} className="text-green-500" />
            : <XCircle size={14} className="text-red-400" />)}
          {checking ? '检测中' : statusError ? '状态未知' : info?.installed ? '可用' : '未检测到'}
        </span>
      </div>
      <div>
        <label htmlFor={id} className="mb-1 block text-xs text-ink-100">{name} 路径</label>
        <input
          id={id}
          className="input-base font-mono text-xs"
          placeholder={`留空自动查找 ${tool}`}
          value={value}
          disabled={disabled}
          onChange={(event) => onChange(event.target.value)}
          aria-describedby={`${id}-status`}
          spellCheck={false}
        />
      </div>
      <div id={`${id}-status`} className="space-y-1 text-xs text-sand-500">
        <p className="break-all">
          当前使用：<span className="font-mono">{confirmed ? info?.path || '未检测到可用工具' : '尚未确认'}</span>
        </p>
        {confirmed && info?.version && <p className="truncate" title={info.version}>{info.version}</p>}
        {pending && <p className="text-brand-500">路径尚未保存，保存后重新检测。</p>}
      </div>
    </div>
  )
}
