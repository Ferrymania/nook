import { FormEvent, useEffect, useState } from 'react'
import toast from 'react-hot-toast'
import { Pencil, Save, Trash2, X } from 'lucide-react'

import { apiConfigsAPI, type APIConfig } from '../api/api_configs'
import { confirmAction } from './confirmAction'

const PROVIDER_NAMES: Record<string, string> = {
  tmdb: 'TMDb', bangumi: 'Bangumi', fanart: 'Fanart.tv',
  openai: 'OpenAI 兼容', thetvdb: 'TheTVDB',
}

// Compact inline-editable provider table for use inside AdminPage's "外部API" tab.
export function APIConfigsPanel() {
  const [items, setItems] = useState<APIConfig[]>([])
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState<string | null>(null)

  const refresh = () =>
    apiConfigsAPI
      .list()
      .then(rows => setItems(rows.filter(item => Object.prototype.hasOwnProperty.call(PROVIDER_NAMES, item.provider))))
      .finally(() => setLoading(false))

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  return (
    <div className="space-y-4">
      {loading && (
        <p className="py-6 text-center text-sm text-sand-500">加载中…</p>
      )}

      {!loading && (
        <div className="glass-panel table-scroll">
          <table className="min-w-[640px] w-full text-left text-sm">
            <thead className="border-b border-gray-200 text-xs text-sand-500">
              <tr>
                <th className="px-4 py-3">服务</th>
                <th className="px-4 py-3">密钥</th>
                <th className="px-4 py-3">状态</th>
                <th className="px-4 py-3 text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) =>
                editing === item.provider ? (
                  <EditingRow
                    key={item.id}
                    item={item}
                    onCancel={() => setEditing(null)}
                    onSaved={() => {
                      setEditing(null)
                      refresh()
                    }}
                  />
                ) : (
                  <tr
                    key={item.id}
                    className="border-t border-gray-200 transition hover:bg-gray-50"
                  >
                    <td className="px-4 py-3">
                      <span className="font-medium text-ink-600">{PROVIDER_NAMES[item.provider] ?? item.provider}</span>
                    </td>
                    <td className="px-4 py-3 font-mono text-xs">
                      {item.has_key ? (
                        <span className="text-brand-500">{item.masked_key}</span>
                      ) : (
                        <span className="text-(--app-muted)" aria-label="未填写密钥">—</span>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      {!item.enabled ? (
                        <span className="rounded-full bg-sand-300/40 px-2 py-0.5 text-xs text-sand-500">全局已关闭</span>
                      ) : !item.has_key && item.provider === 'bangumi' ? (
                        <span className="rounded-full bg-emerald-400/10 px-2 py-0.5 text-xs text-emerald-600">可匿名使用</span>
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
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <button
                          onClick={() => setEditing(item.provider)}
                          className="inline-flex h-11 w-11 items-center justify-center rounded-lg text-ink-50 transition hover:bg-(--app-hover) hover:text-(--app-text) sm:h-8 sm:w-8"
                          title="编辑"
                          aria-label={`编辑 ${PROVIDER_NAMES[item.provider] ?? item.provider}`}
                        >
                          <Pencil size={14} />
                        </button>
                        {item.has_key && (
                          <button
                            onClick={async () => {
                              if (!(await confirmAction({ title: '清除 API Key', message: `确定清除 ${item.provider} 的 API Key?`, confirmText: '清除' }))) return
                              await apiConfigsAPI.remove(item.provider)
                              toast.success('已清除')
                              refresh()
                            }}
                            className="inline-flex h-11 w-11 items-center justify-center rounded-lg text-ink-50 transition hover:bg-red-400/10 hover:text-red-400 sm:h-8 sm:w-8"
                            title="清除密钥"
                            aria-label={`清除 ${PROVIDER_NAMES[item.provider] ?? item.provider} 密钥`}
                          >
                            <Trash2 size={14} />
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                ),
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function EditingRow({
  item,
  onCancel,
  onSaved,
}: {
  item: APIConfig
  onCancel: () => void
  onSaved: () => void
}) {
  const [apiKey, setAPIKey] = useState('')
  const [baseURL, setBaseURL] = useState(item.base_url ?? '')
  const [enabled, setEnabled] = useState(item.enabled)
  const [saving, setSaving] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setSaving(true)
    try {
      const patch: Record<string, unknown> = { base_url: baseURL, enabled }
      if (apiKey.trim()) patch.api_key = apiKey.trim()
      await apiConfigsAPI.update(item.provider, patch)
      toast.success(`${item.provider} 已保存`)
      onSaved()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '保存失败'
      toast.error(msg)
    } finally {
      setSaving(false)
    }
  }

  return (
    <tr className="border-t border-gray-200 bg-primary-400/5">
      <td colSpan={4} className="px-4 py-3">
        <form onSubmit={submit} className="flex flex-wrap items-end gap-3">
          <span className="text-sm font-medium text-ink-600">{PROVIDER_NAMES[item.provider] ?? item.provider}</span>
          <label className="flex-1 text-xs text-ink-50">
            {item.provider === 'bangumi' ? 'Access Token（可选）' : 'API Key'}
            <input
              className="input-base mt-1"
              type="password"
              placeholder={item.has_key ? '•••••••••••• (留空保留原值)' : item.provider === 'bangumi' ? '留空匿名使用' : '输入密钥'}
              value={apiKey}
              onChange={(e) => setAPIKey(e.target.value)}
            />
          </label>
          <label className="flex-1 text-xs text-ink-50">
            Base URL
            <input
              className="input-base mt-1"
              placeholder="https://api.themoviedb.org/3"
              value={baseURL}
              onChange={(e) => setBaseURL(e.target.value)}
            />
          </label>
          <label className="flex items-center gap-2 text-xs text-ink-50">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
            />
            全局启用
          </label>
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
