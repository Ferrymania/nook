import { useState } from 'react'
import { Loader2, Send } from 'lucide-react'

import { telegramAPI } from '../api/telegram'

// TelegramNotifyPanel 提供「发送测试消息」按钮。
//
// 通知通道最容易出的问题是 Token 或 Chat ID 填错，而这类错误平时是完全静默的
// （发送失败只写服务端日志）。这个按钮把失败原因直接带回界面。
export function TelegramNotifyPanel({ disabled = false }: { disabled?: boolean }) {
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<{ ok: boolean; message: string } | null>(null)

  const runTest = async () => {
    setBusy(true)
    setResult(null)
    try {
      const res = await telegramAPI.test()
      setResult(
        res.success
          ? { ok: true, message: '测试消息已发送，请检查 Telegram。' }
          : { ok: false, message: res.error || '发送失败' },
      )
    } catch (err) {
      setResult({
        ok: false,
        message: err instanceof Error ? err.message : '发送失败',
      })
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-3">
      <button
        type="button"
        onClick={() => void runTest()}
        disabled={busy || disabled}
        title={disabled ? '请先保存 Telegram 设置' : undefined}
        className="btn-secondary inline-flex items-center gap-2"
      >
        {busy ? <Loader2 size={14} className="animate-spin" /> : <Send size={14} />}
        <span>发送测试消息</span>
      </button>

      {result && (
        <span role="status"
          className={
            'text-xs font-semibold ' + (result.ok ? 'text-emerald-400' : 'text-rose-400')
          }
        >
          {result.message}
        </span>
      )}
    </div>
  )
}
