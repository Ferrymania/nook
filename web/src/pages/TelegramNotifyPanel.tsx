import { useState } from 'react'
import { Loader2, Send } from 'lucide-react'

import { telegramAPI } from '../api/telegram'

// TelegramNotifyPanel 提供「发送测试消息」按钮。
//
// 通知通道最容易出的问题是 Token 或 Chat ID 填错，而这类错误平时是完全静默的
// （发送失败只写服务端日志）。这个按钮把失败原因直接带回界面。
export function TelegramNotifyPanel() {
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
    <div className="glass-panel space-y-3">
      <div>
        <h3 className="font-display text-sm font-bold text-ink-100">通知通道自检</h3>
        <p className="mt-0.5 text-xs text-sand-500">
          保存上方设置后点击发送。测试消息会发到「管理员 Chat ID」，用于确认 Token
          与会话 ID 都正确。
        </p>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <button
          type="button"
          onClick={() => void runTest()}
          disabled={busy}
          className="btn-secondary inline-flex items-center gap-2"
        >
          {busy ? <Loader2 size={14} className="animate-spin" /> : <Send size={14} />}
          <span>发送测试消息</span>
        </button>

        {result && (
          <span
            className={
              'text-xs font-semibold ' + (result.ok ? 'text-emerald-400' : 'text-rose-400')
            }
          >
            {result.message}
          </span>
        )}
      </div>

      <div className="border-t border-sand-200/60 pt-3 text-xs text-ink-50">
        <h4 className="font-semibold text-ink-100">Telegram 同步控制</h4>
        <p className="mt-1">
          启用 Telegram 并保存 Bot Token 与管理员的私聊 Chat ID 后，会自动注册命令菜单。
          使用这里配置的同一个 Bot，无需在个人资料中绑定。群组 Chat ID 仅接收通知，不能控制同步。
        </p>
        <dl className="mt-2 grid gap-1 sm:grid-cols-2">
          <div><dt className="inline font-mono">/strm_inc</dt><dd className="ml-2 inline">全部启用目录依次增量同步</dd></div>
          <div><dt className="inline font-mono">/sync</dt><dd className="ml-2 inline">选择目录增量同步</dd></div>
          <div><dt className="inline font-mono">/status</dt><dd className="ml-2 inline">查看同步状态及最近结果</dd></div>
          <div><dt className="inline font-mono">/cancel</dt><dd className="ml-2 inline">取消单个目录或本次整批同步</dd></div>
        </dl>
        <p className="mt-2">首次使用请先与 Bot 私聊发送 /start。同步结束后会返回目录结果和汇总，元数据下载、上传数量表示已入队。</p>
      </div>
    </div>
  )
}
