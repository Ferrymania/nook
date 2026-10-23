import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { Download, Loader2, ScanSearch, Wrench } from 'lucide-react'
import toast from 'react-hot-toast'

import { toolsAPI, type FFmpegToolInfo, type FFmpegToolPair, type FFmpegToolsStatus } from '../api/tools'
import { SettingsSection } from './SettingsSection'

const TOOLS = ['ffmpeg', 'ffprobe'] as const

type Tool = typeof TOOLS[number]
interface FFToolsPanelProps {
  values: Record<string, string>
  savedValues: Record<string, string>
  dirty: Set<string>
  saving: boolean
  statusRevision: number
  onChange: (key: string, value: string) => void
  onBusyChange: (busy: boolean) => void
  children?: ReactNode
}

function errorMessage(err: unknown, fallback: string) {
  return (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
    (err as { message?: string })?.message ?? fallback
}

export function FFToolsPanel({ values, savedValues, dirty, saving, statusRevision, onChange, onBusyChange, children }: FFToolsPanelProps) {
  const [status, setStatus] = useState<FFmpegToolsStatus | null>(null)
  const [checking, setChecking] = useState(true)
  const [statusError, setStatusError] = useState(false)
  const [reload, setReload] = useState(0)
  const [operation, setOperation] = useState<'detect' | 'install' | null>(null)
  const [candidates, setCandidates] = useState<FFmpegToolPair>({})
  const [feedback, setFeedback] = useState<{ text: string; error?: boolean } | null>(null)
  const action = useRef<AbortController | null>(null)
  const handledInstall = useRef('')
  const installing = Boolean(status?.installing)
  const busy = checking || installing || operation !== null

  useEffect(() => { onBusyChange(busy) }, [busy, onBusyChange])
  useEffect(() => () => { action.current?.abort(); onBusyChange(false) }, [onBusyChange])
  useEffect(() => { setFeedback(null); setCandidates({}) }, [statusRevision])

  useEffect(() => {
    const controller = new AbortController()
    setChecking(true)
    toolsAPI.ffToolsStatus(controller.signal).then((next) => {
      if (controller.signal.aborted) return
      setStatus(next)
      setStatusError(false)
    }).catch(() => {
      if (!controller.signal.aborted) setStatusError(true)
    }).finally(() => {
      if (!controller.signal.aborted) setChecking(false)
    })
    return () => controller.abort()
  }, [statusRevision, reload])

  const fillPaths = useCallback((pair: FFmpegToolPair, source: 'detect' | 'install') => {
    const found = TOOLS.filter((tool) => pair[tool]?.installed && pair[tool]?.path)
    const changed = found.some((tool) => pair[tool]?.path !== (savedValues[`${tool}.path`] ?? ''))
    for (const tool of found) onChange(`${tool}.path`, pair[tool]!.path!)
    setCandidates(pair)
    setFeedback({
      error: found.length !== 2,
      text: found.length === 0
        ? '未找到可用工具，原有路径已保留。可手动填写或尝试下载安装。'
        : found.length !== 2
          ? '只找到一个工具，已填入可用路径，其他路径保持不变。'
          : !changed
            ? '路径与当前配置一致，无需重复保存。'
            : source === 'install'
            ? '安装完成，路径已填入。点击页面底部「保存」后生效。'
            : '检测完成，路径已填入；如有更改，点击页面底部「保存」后生效。',
    })
  }, [onChange, savedValues])

  const completeInstall = useCallback((next: FFmpegToolsStatus) => {
    const id = `${next.started_at ?? ''}/${next.finished_at ?? ''}`
    if (handledInstall.current === id) return
    handledInstall.current = id
    if (next.error) {
      setFeedback({ error: true, text: `下载安装失败：${next.error}。原有路径未更改。` })
    } else if (next.downloaded?.ffmpeg?.installed && next.downloaded?.ffprobe?.installed) {
      fillPaths(next.downloaded, 'install')
    } else {
      setFeedback({ error: true, text: '未检测到完整的下载工具，原有路径已保留。请重试下载安装。' })
    }
  }, [fillPaths])

  useEffect(() => {
    if (!installing) return
    const controller = new AbortController()
    let timer: number
    const poll = async () => {
      try {
        const next = await toolsAPI.ffToolsStatus(controller.signal)
        if (controller.signal.aborted) return
        setStatus(next)
        if (!next.installing) {
          completeInstall(next)
          return
        }
        setFeedback(null)
      } catch {
        if (controller.signal.aborted) return
        setFeedback({ text: '暂时无法获取安装进度，正在重试；当前配置继续使用。' })
      }
      timer = window.setTimeout(poll, 1500)
    }
    timer = window.setTimeout(poll, 1500)
    return () => { controller.abort(); window.clearTimeout(timer) }
  }, [installing, completeInstall])

  const start = async (kind: 'detect' | 'install') => {
    const controller = new AbortController()
    action.current = controller
    setOperation(kind)
    setFeedback(null)
    try {
      if (kind === 'detect') {
        const pair = await toolsAPI.detectFFTools(controller.signal)
        if (!controller.signal.aborted) fillPaths(pair, 'detect')
      } else {
        const next = await toolsAPI.installFFTools(controller.signal)
        if (controller.signal.aborted) return
        setStatus(next)
        if (!next.installing) completeInstall(next)
      }
    } catch (err) {
      if (!controller.signal.aborted) {
        const message = errorMessage(err, kind === 'detect' ? '自动检测失败' : '启动下载失败')
        setFeedback({ error: true, text: `${message}。原有路径未更改。` })
        toast.error(message)
      }
    } finally {
      if (!controller.signal.aborted) setOperation(null)
    }
  }

  const downloadedUnused = status?.downloaded?.ffmpeg?.installed && status?.downloaded?.ffprobe?.installed &&
    (status.downloaded.ffmpeg.path !== status.ffmpeg?.path || status.downloaded.ffprobe.path !== status.ffprobe?.path)

  return (
    <SettingsSection title="媒体工具" icon={Wrench} actions={
        <div className="flex flex-wrap gap-2">
          <button type="button" onClick={() => start('detect')} disabled={busy || saving} className="library-toolbar-button">
            {operation === 'detect' ? <Loader2 size={16} className="animate-spin" /> : <ScanSearch size={16} />}
            {operation === 'detect' ? '检测中…' : '自动检测'}
          </button>
          <button type="button" onClick={() => start('install')} disabled={busy || saving} className="library-toolbar-button">
            {installing || operation === 'install' ? <Loader2 size={16} className="animate-spin" /> : <Download size={16} />}
            {installing || operation === 'install' ? '下载安装中…' : '下载安装'}
          </button>
        </div>
      }>
      <div className="settings-tool-paths">
        {TOOLS.map((tool) => (
          <div key={tool} className={tool === 'ffprobe' ? 'settings-probe-row' : undefined}>
            <ToolPath
              tool={tool} value={values[`${tool}.path`] ?? ''} active={status?.[tool]}
              candidate={candidates[tool]} pending={dirty.has(`${tool}.path`)}
              checking={checking} statusError={statusError} disabled={busy || saving}
              onChange={(value) => { onChange(`${tool}.path`, value); setFeedback(null) }}
            />
            {tool === 'ffprobe' && children}
          </div>
        ))}
      </div>
      <p className="settings-help-text">留空自动查找系统工具。FFprobe 并发建议为 1。</p>
      <details className="settings-disclosure mt-2">
        <summary>路径说明与工具版本</summary>
        <p className="settings-help-text">可手动填写或自动检测；Docker 部署请使用容器内可见的路径。检测或安装后，点击下方保存生效。</p>
        {TOOLS.map((tool) => status?.[tool]?.version && <p key={tool} className="settings-help-text wrap-break-word font-mono">{status[tool]!.version!.split(' Copyright')[0]}</p>)}
      </details>
      {statusError && <div role="alert" className="flex flex-wrap items-center gap-2 text-xs text-red-600">
        当前工具状态获取失败，路径修改已保留。
        <button type="button" className="underline" disabled={checking} onClick={() => setReload((v) => v + 1)}>重新检测状态</button>
      </div>}
      {(installing || feedback || downloadedUnused) && <div
        role={feedback?.error ? 'alert' : 'status'}
        className={`rounded-xl px-3 py-2 text-xs ${feedback?.error ? 'bg-red-50 text-red-600' : 'bg-(--app-panel-soft) text-(--app-muted)'}`}
      >
        {feedback?.text ?? (installing
          ? `${status?.message || '正在下载安装…'} 当前配置继续使用；安装完成只填入路径，保存后才生效。`
          : '已下载的工具可复用。点击「下载安装」填入路径，再保存即可使用。')}
      </div>}
    </SettingsSection>
  )
}

function ToolPath({ tool, value, active, candidate, pending, checking, statusError, disabled, onChange }: {
  tool: Tool; value: string; active?: FFmpegToolInfo; candidate?: FFmpegToolInfo
  pending: boolean; checking: boolean; statusError: boolean; disabled: boolean; onChange: (value: string) => void
}) {
  const id = useId()
  const name = tool === 'ffmpeg' ? 'FFmpeg' : 'FFprobe'
  const info = candidate?.path === value ? candidate : pending ? undefined : active
  const confirmed = !checking && !statusError
  const showDetail = pending || !confirmed || !active?.installed
  return <div className="min-w-0 space-y-2">
    <div className="flex items-center justify-between gap-3">
      <label htmlFor={id} className="text-sm font-medium text-ink-600">{name} 程序路径</label>
      <span className={`shrink-0 rounded-full px-2 py-0.5 text-xs ${pending ? 'bg-(--app-brand-soft) text-(--app-brand-text)' : 'bg-(--app-panel-soft) text-(--app-muted)'}`}>
        {pending ? '待保存' : checking ? '检测中' : statusError ? '状态未知' : active?.installed ? '可用' : '未检测到'}
      </span>
    </div>
    <input id={id} className="input-base font-mono text-sm" value={value}
      placeholder={confirmed && active?.path ? active.path : `输入 ${name} 程序路径，或点击自动检测`}
      onChange={(event) => onChange(event.target.value)} disabled={disabled} spellCheck={false}
      aria-describedby={showDetail ? `${id}-detail` : undefined} />
    {showDetail && <p id={`${id}-detail`} className="break-all text-xs text-(--app-muted)">
      {info?.version && (pending || confirmed) ? `${info.version.split(' Copyright')[0]}${pending ? ' · 保存后使用' : ''}`
        : pending ? '路径尚未保存，当前配置继续使用。'
          : confirmed && active?.installed ? '当前工具可用。' : '填写完整程序路径；Docker 部署请使用容器内可见的路径。'}
    </p>}
  </div>
}
