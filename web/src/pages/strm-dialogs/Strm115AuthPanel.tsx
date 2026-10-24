import { useEffect, useMemo, useRef, useState } from 'react'
import toast from 'react-hot-toast'
import { Loader2, QrCode, RefreshCw, Search } from 'lucide-react'
import QRCode from 'qrcode'

import { strmAPI, type Strm115AuthSource, type Strm115Sources } from '../../api/strm'
import type { StrmAccount } from '../../types/strm'
import { apiErrorMessage } from '../StrmManagePage'

const QR_SIZE = 176

function QrCanvas({ content }: { content: string }) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas || !content) return
    QRCode.toCanvas(canvas, content, {
      width: QR_SIZE,
      margin: 2,
      errorCorrectionLevel: 'M',
      color: { dark: '#1f2937', light: '#ffffff' },
    })
      .then(() => setError(''))
      .catch((err: unknown) => setError(err instanceof Error ? err.message : '二维码生成失败'))
  }, [content])

  return (
    <div className="relative h-44 w-44 shrink-0 rounded-xl border border-gray-200 bg-white">
      <canvas
        ref={canvasRef}
        width={QR_SIZE}
        height={QR_SIZE}
        role="img"
        aria-label="115 授权二维码"
        style={{ width: QR_SIZE, height: QR_SIZE }}
      />
      {error && (
        <div className="absolute inset-0 grid place-items-center rounded-xl bg-gray-50 p-3 text-center text-xs text-rose-500">
          {error}
        </div>
      )}
    </div>
  )
}

const AUTH_SOURCE_OPTIONS: { key: Strm115AuthSource; label: string; desc: string }[] = [
  { key: 'built_in_appid', label: '官方应用目录', desc: '设备码扫码授权，无需回跳' },
  { key: 'custom_appid', label: '自定义 APP ID', desc: '使用自己申请的开放平台应用' },
]

const inputCls = 'input-base w-full'

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <label className="block space-y-1">
      <span className="text-sm font-medium text-ink-100">{label}</span>
      {children}
      {hint && <span className="block text-xs text-sand-500">{hint}</span>}
    </label>
  )
}

export function Strm115AuthPanel({
  existing,
  accountName,
  onAuthed,
}: {
  existing: StrmAccount | null
  accountName: string
  onAuthed: (account: StrmAccount) => void
}) {
  const [sources, setSources] = useState<Strm115Sources | null>(null)
  const [authSource, setAuthSource] = useState<Strm115AuthSource>('built_in_appid')
  const [appID, setAppID] = useState('100195125')
  const [customAppID, setCustomAppID] = useState('')
  const [appKeyword, setAppKeyword] = useState('')

  const [starting, setStarting] = useState(false)
  const [authUI, setAuthUI] = useState<{ qrcodeUrl: string } | null>(null)
  const [authStatus, setAuthStatus] = useState('')

  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const stopPolling = () => {
    if (pollRef.current) {
      clearInterval(pollRef.current)
      pollRef.current = null
    }
  }
  useEffect(() => stopPolling, [])

  useEffect(() => {
    strmAPI.list115Sources().then(setSources).catch((err) => toast.error(apiErrorMessage(err)))
  }, [])

  const filteredApps = useMemo(
    () =>
      (sources?.built_in ?? []).filter(
        (s) => !appKeyword || s.app_name.toLowerCase().includes(appKeyword.toLowerCase()) || s.app_id.includes(appKeyword),
      ),
    [appKeyword, sources],
  )

  useEffect(() => {
    if (filteredApps.length > 0 && !filteredApps.some((source) => source.app_id === appID)) {
      setAppID(filteredApps[0].app_id)
    }
  }, [appID, filteredApps])

  const startAuth = async () => {
    if (authSource === 'built_in_appid' && filteredApps.length === 0) {
      toast.error('未找到匹配的官方应用')
      return
    }
    const selectedAppID = authSource === 'custom_appid' ? customAppID.trim() : appID
    if (!selectedAppID) {
      toast.error('请输入自定义 APP ID')
      return
    }

    setStarting(true)
    setAuthStatus('')
    try {
      // 115 授权需要账号 ID：没有则先创建空凭据账号
      let account = existing
      if (!account) {
        account = await strmAPI.createAccount({ name: accountName || '115 网盘', provider: 'cloud115', config: {} })
      }
      const result = await strmAPI.start115OAuth(account.id, { auth_source: authSource, app_id: selectedAppID })
      if (!result.qrcode?.qrcode) throw new Error('授权响应缺少二维码')
      setAuthUI({ qrcodeUrl: result.qrcode.qrcode })
      stopPolling()
      // confirmed / expired 只处理一次，防止重叠的轮询回调重复触发
      let settled = false
      pollRef.current = setInterval(async () => {
        if (settled) return
        try {
          const status = await strmAPI.poll115OAuth(account.id, result.session_id)
          if (settled) return
          setAuthStatus(status.tip)
          if (status.status === 'confirmed') {
            settled = true
            stopPolling()
            setAuthStatus('授权成功，正在验证凭据…')
            try {
              const updated = await strmAPI.testAccount(account.id)
              onAuthed(updated)
            } catch (err) {
              // 授权已确认、令牌已保存；凭据验证失败不阻塞授权完成，
              // 避免轮询已停且 onAuthed 未回调导致弹窗卡死。
              toast.error(`凭据验证失败：${apiErrorMessage(err)}，授权已保存，可稍后在账号列表重试`)
              onAuthed(account)
            }
          }
          if (status.status === 'expired') {
            settled = true
            stopPolling()
            setAuthUI(null)
            toast.error('授权已过期，请重新发起')
          }
        } catch {
          /* 轮询失败等下一次 */
        }
      }, 3000)
    } catch (err) {
      toast.error(apiErrorMessage(err))
    } finally {
      setStarting(false)
    }
  }

  const resetAuth = () => {
    stopPolling()
    setAuthUI(null)
    setAuthStatus('')
  }

  return (
    <div className="space-y-4 rounded-2xl bg-gray-50 p-4">
      {existing?.has_credential && (
        <p className="rounded-xl bg-emerald-50 px-3 py-2 text-sm text-emerald-600">
          ✓ 该账号已授权；重新授权会替换现有令牌
        </p>
      )}

      {!authUI && (
        <div className="space-y-3">
          <div className="grid grid-cols-2 gap-2">
            {AUTH_SOURCE_OPTIONS.map((option) => {
              const active = authSource === option.key
              return (
                <button
                  key={option.key}
                  type="button"
                  onClick={() => setAuthSource(option.key)}
                  className={
                    'rounded-xl border-2 p-2.5 text-left transition ' +
                    (active ? 'border-brand-400 bg-brand-50' : 'border-gray-100 bg-white hover:border-gray-200')
                  }
                >
                  <p className="text-sm font-bold text-ink-600">{option.label}</p>
                  <p className="text-[11px] text-sand-500">{option.desc}</p>
                </button>
              )
            })}
          </div>

          {authSource === 'built_in_appid' && (
            <div className="space-y-2">
              <Field label="选择官方应用" hint="应用目录来自 QMediaSync 内置；媒体播放器 / 飞牛 / 恒星等">
                <div className="flex items-center gap-2">
                  <Search size={14} className="shrink-0 text-sand-400" />
                  <input className={inputCls} placeholder="搜索应用名称或 ID…" value={appKeyword} onChange={(e) => setAppKeyword(e.target.value)} />
                </div>
                <select className={inputCls} value={appID} onChange={(e) => setAppID(e.target.value)} size={5}>
                  {filteredApps.map((source) => (
                    <option key={source.app_id} value={source.app_id}>
                      {source.display_name}（{source.app_id}）
                    </option>
                  ))}
                </select>
                {filteredApps.length === 0 && <p className="text-xs text-sand-500">未找到匹配的官方应用</p>}
              </Field>
            </div>
          )}
          {authSource === 'custom_appid' && (
            <Field label="自定义 APP ID" hint="使用自己在 115 开放平台申请的应用 ID">
              <input className={inputCls} value={customAppID} placeholder="100195125" onChange={(e) => setCustomAppID(e.target.value)} />
            </Field>
          )}
          <button
            type="button"
            onClick={startAuth}
            disabled={starting || (authSource === 'built_in_appid' ? filteredApps.length === 0 : !customAppID.trim())}
            className="neon-button disabled:opacity-50"
          >
            {starting ? <Loader2 size={16} className="animate-spin" /> : <QrCode size={16} />}
            获取登录二维码
          </button>
        </div>
      )}

      {authUI && (
        <div className="space-y-3">
          <div className="flex flex-wrap items-center gap-4">
            <QrCanvas content={authUI.qrcodeUrl} />
            <div className="space-y-1.5 text-sm">
              <p className="flex items-center gap-2 font-medium text-ink-600"><Loader2 size={14} className="animate-spin" />{authStatus || '等待扫码…'}</p>
              <p className="text-xs text-sand-500">请使用 115 手机客户端扫码并确认授权，5 分钟内有效</p>
            </div>
          </div>
          <button
            type="button"
            onClick={resetAuth}
            className="inline-flex items-center gap-1 rounded-lg border border-gray-200 px-2.5 py-1.5 text-xs font-semibold text-ink-100 transition hover:bg-gray-50"
          >
            <RefreshCw size={14} />
            重新发起授权
          </button>
        </div>
      )}
    </div>
  )
}
