import { useEffect, useId, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { X } from 'lucide-react'

import { externalRatingsAPI, type LibraryRatingStatus } from '../api/library'
import type { Library } from '../types'
import { apiErrorMessage } from './adminLibraryPanelModel'

const pauseMessages = {
  unconfigured: '请先在「系统设置 → 外部 API」启用 OMDb 并填写 API Key。',
  invalid_key: 'OMDb 密钥无效，请在「外部 API」更新密钥。更换后会自动继续。',
  quota: '已达到今日请求上限，额度重置后会自动继续。',
}

export function LibraryRatingsDialog({ library, submission, onClose }: { library: Library; submission: Promise<string>; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const title = useId()
  const [status, setStatus] = useState<LibraryRatingStatus>()
  const [error, setError] = useState('')
  const [submitError, setSubmitError] = useState('')

  useEffect(() => {
    const node = dialog.current
    node?.showModal()
    return () => node?.close()
  }, [])

  useEffect(() => {
    let active = true
    let timer: ReturnType<typeof setTimeout>
    const refresh = async () => {
      let delay = 15000
      try {
        const next = await externalRatingsAPI.libraryStatus(library.id)
        if (active) {
          setStatus(next)
          setError('')
          if (next.pending && !next.pause_reason) delay = 5000
        }
      } catch (error) {
        if (active) setError(apiErrorMessage(error, '读取评分进度失败'))
      } finally {
        if (active) timer = setTimeout(refresh, delay)
      }
    }
    void submission.then(message => {
      if (!active) return
      setSubmitError(message)
      return refresh()
    })
    return () => { active = false; clearTimeout(timer) }
  }, [library.id, submission])

  const progress = status && [
    ['已补齐', status.found],
    ['部分有分', status.partial],
    ['暂无评分', status.missing],
    ['待处理', status.pending],
    ['未查询', status.unqueried],
    ['失败待重试', status.failed],
  ] as const
  const pauseMessage = status?.pause_reason ? pauseMessages[status.pause_reason] : undefined
  const visibleError = error || (status?.pause_reason === 'unconfigured' ? '' : submitError)

  return createPortal(
    <dialog ref={dialog} aria-labelledby={title}
      className="m-auto max-h-[90dvh] w-[calc(100%-1.5rem)] max-w-md overflow-y-auto rounded-2xl border border-(--app-border) bg-(--app-panel) p-0 text-(--app-text) backdrop:bg-black/40"
      onCancel={event => { event.preventDefault(); onClose() }}>
      <header className="flex items-start justify-between gap-4 border-b border-(--app-border) px-5 py-4">
        <div className="min-w-0">
          <h2 id={title} className="font-semibold">补齐评分</h2>
          <p className="mt-1 break-all text-sm text-(--app-subtle)">{library.name}</p>
        </div>
        <button type="button" className="nook-library-manager-button shrink-0" aria-label="关闭" onClick={onClose}><X size={16} /></button>
      </header>
      <div className="space-y-3 px-5 py-4" aria-live="polite">
        {status && progress ? (
          <>
            <p className="text-sm text-(--app-subtle)">{status.total} 部已匹配 TMDb 的作品</p>
            <dl className="grid grid-cols-2 gap-x-5 gap-y-2">
              {progress.map(([label, count]) => (
                <div key={label} className="flex items-baseline justify-between gap-2 text-sm">
                  <dt className="text-(--app-subtle)">{label}</dt><dd className="tabular-nums">{count}</dd>
                </div>
              ))}
            </dl>
          </>
        ) : !visibleError && <p className="text-sm text-(--app-subtle)">正在读取评分进度…</p>}
        {pauseMessage && <p className="text-sm text-(--app-subtle)">{pauseMessage}</p>}
        {visibleError && <p role="alert" className="text-sm text-(--app-danger-text)">{visibleError}</p>}
      </div>
    </dialog>,
    document.body,
  )
}
