import { FormEvent, Fragment, useEffect, useState } from 'react'
import toast from 'react-hot-toast'
import { Pencil, Save, Trash2, X } from 'lucide-react'

import { apiConfigsAPI, type APIConfig, type APIConfigPatch } from '../api/api_configs'
import { confirmAction } from './confirmAction'
import { SettingsSwitch } from './SettingsSection'

const PROVIDER_NAMES: Record<string, string> = {
  tmdb: 'TMDb', bangumi: 'Bangumi', fanart: 'Fanart.tv',
  omdb: 'OMDb', openai: 'OpenAI 兼容', thetvdb: 'TheTVDB',
}

// Compact inline-editable provider table for use inside AdminPage's "外部API" tab.
export function APIConfigsPanel() {
  const [items, setItems] = useState<APIConfig[]>([])
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState<string | null>(null)
  const [switching, setSwitching] = useState<string[]>([])

  const refresh = () =>
    apiConfigsAPI
      .list()
      .then(rows => setItems(rows.filter(item => Object.prototype.hasOwnProperty.call(PROVIDER_NAMES, item.provider))))
      .finally(() => setLoading(false))

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  const updateItem = (id: string, changes: Partial<APIConfig>) => {
    setItems(rows => rows.map(row => row.id === id ? { ...row, ...changes } : row))
  }

  const setEnabled = async (item: APIConfig, enabled: boolean) => {
    setSwitching(current => [...current, item.id])
    try {
      const saved = await apiConfigsAPI.update(item.provider, { enabled })
      updateItem(item.id, { enabled: saved.enabled })
    } catch (err: unknown) {
      toast.error(apiConfigError(err, '更新启用状态失败'))
    } finally {
      setSwitching(current => current.filter(id => id !== item.id))
    }
  }

  return (
    <div className="space-y-4">
      {loading && (
        <p className="py-6 text-center text-sm text-sand-500">加载中…</p>
      )}

      {!loading && (
        <div className="glass-panel table-scroll max-sm:px-2">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-200 text-xs text-sand-500">
              <tr>
                <th className="px-2 py-3 sm:px-4">服务</th>
                <th className="hidden px-4 py-3 sm:table-cell">密钥</th>
                <th className="px-1 py-3 sm:px-4">配置状态</th>
                <th className="whitespace-nowrap px-1 py-3 text-center sm:px-4">全局启用</th>
                <th className="px-2 py-3 text-right sm:px-4">操作</th>
              </tr>
            </thead>
            <tbody>
              {items.map(item => (
                <Fragment key={item.id}>
                  <tr
                    className="border-t border-gray-200 transition hover:bg-gray-50"
                  >
                    <td className="px-2 py-3 sm:px-4">
                      <span className="font-medium text-ink-600">{PROVIDER_NAMES[item.provider] ?? item.provider}</span>
                      {item.has_key && (
                        <span className="block font-mono text-xs text-brand-500 sm:hidden">{item.masked_key}</span>
                      )}
                    </td>
                    <td className="hidden px-4 py-3 font-mono text-xs sm:table-cell">
                      {item.has_key ? (
                        <span className="text-brand-500">{item.masked_key}</span>
                      ) : (
                        <span className="text-(--app-muted)" aria-label="未填写密钥">—</span>
                      )}
                    </td>
                    <td className="whitespace-nowrap px-1 py-3 sm:px-4">
                      {!item.has_key && item.provider === 'bangumi' ? (
                        <span className="rounded-full bg-emerald-400/10 px-2 py-0.5 text-xs text-emerald-600">无需密钥</span>
                      ) : item.has_key ? (
                        <span className="inline-flex items-center gap-1 rounded-full bg-emerald-400/10 px-2 py-0.5 text-xs text-emerald-400">
                          已配置
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1 rounded-full bg-sand-300/40 px-2 py-0.5 text-xs text-sand-500">
                          未配置
                        </span>
                      )}
                    </td>
                    <td className="px-1 py-3 sm:px-4">
                      <div className="flex justify-center">
                        <SettingsSwitch
                          label={`${PROVIDER_NAMES[item.provider] ?? item.provider} 全局启用`}
                          checked={item.enabled}
                          disabled={switching.includes(item.id)}
                          onChange={enabled => void setEnabled(item, enabled)}
                        />
                      </div>
                    </td>
                    <td className="px-1 py-3 text-right sm:px-4">
                      <div className="flex items-center justify-end sm:gap-1">
                        <button
                          onClick={() => setEditing(item.provider)}
                          aria-expanded={editing === item.provider}
                          className="inline-flex h-11 w-10 items-center justify-center rounded-lg text-ink-50 transition hover:bg-(--app-hover) hover:text-(--app-text) sm:h-8 sm:w-8"
                          title="编辑"
                          aria-label={`编辑 ${PROVIDER_NAMES[item.provider] ?? item.provider}`}
                        >
                          <Pencil size={14} />
                        </button>
                        {item.has_key && (
                          <button
                            onClick={async () => {
                              if (!(await confirmAction({ title: '清除 API Key', message: `确定清除 ${item.provider} 的 API Key?`, confirmText: '清除' }))) return
                              try {
                                await apiConfigsAPI.remove(item.provider)
                                updateItem(item.id, { has_key: false, masked_key: undefined })
                                toast.success('已清除')
                              } catch (err: unknown) {
                                toast.error(apiConfigError(err, '清除密钥失败'))
                              }
                            }}
                            className={`inline-flex h-11 w-10 items-center justify-center rounded-lg text-ink-50 transition hover:bg-red-400/10 hover:text-red-400 sm:h-8 sm:w-8 ${editing === item.provider ? 'invisible' : ''}`}
                            title="清除密钥"
                            aria-label={`清除 ${PROVIDER_NAMES[item.provider] ?? item.provider} 密钥`}
                          >
                            <Trash2 size={14} />
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                  {editing === item.provider && (
                    <EditingRow
                      item={item}
                      onCancel={() => setEditing(null)}
                      onSaved={saved => {
                        updateItem(item.id, {
                          base_url: saved.base_url, extra: saved.extra,
                          has_key: saved.has_key, masked_key: saved.masked_key,
                        })
                        setEditing(current => current === item.provider ? null : current)
                      }}
                    />
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function apiConfigError(err: unknown, fallback: string) {
  return (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? fallback
}

function EditingRow({
  item,
  onCancel,
  onSaved,
}: {
  item: APIConfig
  onCancel: () => void
  onSaved: (saved: APIConfig) => void
}) {
  const [apiKey, setAPIKey] = useState('')
  const [baseURL, setBaseURL] = useState(item.base_url ?? '')
  const [dailyLimit, setDailyLimit] = useState(item.extra || '1000')
  const [saving, setSaving] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setSaving(true)
    try {
      const patch: APIConfigPatch = item.provider === 'omdb' ? { extra: dailyLimit } : { base_url: baseURL }
      if (apiKey.trim()) patch.api_key = apiKey.trim()
      const saved = await apiConfigsAPI.update(item.provider, patch)
      toast.success(`${item.provider} 已保存`)
      onSaved(saved)
    } catch (err: unknown) {
      toast.error(apiConfigError(err, '保存失败'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <tr className="border-t border-gray-200 bg-primary-400/5">
      <td colSpan={5} className="px-2 py-3 sm:px-4">
        <form onSubmit={submit} className="flex flex-wrap items-end gap-3">
          <label className="basis-full text-xs text-ink-50 sm:min-w-0 sm:flex-1">
            {item.provider === 'bangumi' ? 'Access Token（可选）' : 'API Key'}
            <input
              className="input-base mt-1"
              type="password"
              placeholder={item.has_key ? '•••••••••••• (留空保留原值)' : item.provider === 'bangumi' ? '留空匿名使用' : '输入密钥'}
              value={apiKey}
              onChange={(e) => setAPIKey(e.target.value)}
            />
          </label>
          {item.provider === 'omdb' ? (
            <label className="basis-full text-xs text-ink-50 sm:min-w-0 sm:flex-1">
              每日请求上限（UTC）
              <input className="input-base mt-1" type="number" required min={1} max={1_000_000}
                value={dailyLimit} onChange={event => setDailyLimit(event.target.value)} />
            </label>
          ) : (
            <label className="basis-full text-xs text-ink-50 sm:min-w-0 sm:flex-1">
              Base URL
              <input
                className="input-base mt-1"
                placeholder="https://api.themoviedb.org/3"
                value={baseURL}
                onChange={(e) => setBaseURL(e.target.value)}
              />
            </label>
          )}
          <button type="submit" disabled={saving} className="neon-button px-3! py-1.5! text-xs!">
            <Save size={12} /> 保存
          </button>
          <button
            type="button"
            onClick={onCancel}
            aria-label="取消编辑"
            className="inline-flex min-h-11 min-w-11 items-center justify-center rounded-lg border border-sand-400/30 text-xs text-ink-50 hover:bg-(--app-hover) hover:text-(--app-text) sm:min-h-8 sm:min-w-8"
          >
            <X size={12} />
          </button>
        </form>
      </td>
    </tr>
  )
}
