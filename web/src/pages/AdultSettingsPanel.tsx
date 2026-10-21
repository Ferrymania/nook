import { FormEvent, useEffect, useState } from 'react'
import { Loader2, Lock, Save, XCircle } from 'lucide-react'
import toast from 'react-hot-toast'

import { adminAPI } from '../api/admin'
import { libraryAPI } from '../api/library'
import type { Library, Setting } from '../types'
import { adultSettingsGroup } from './settingsGroupAccess'

export function AdultSettingsPanel() {
  const [values, setValues] = useState<Record<string, string>>({})
  const [dirty, setDirty] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [saving, setSaving] = useState(false)
  const [libraries, setLibraries] = useState<Library[]>([])

  const refresh = async () => {
    setLoading(true)
    try {
      const [all, libs] = await Promise.all([
        adminAPI.listSettings(),
        libraryAPI.list({ includeHidden: true }).catch(() => [] as Library[]),
      ])
      const idx: Record<string, string> = {}
      for (const item of adultSettingsGroup.items) {
        if (item.defaultValue !== undefined) {
          idx[item.key] = item.defaultValue
        }
      }
      for (const s of all as Setting[]) {
        if (adultSettingsGroup.items.some((item) => item.key === s.key)) {
          idx[s.key] = s.value
        }
      }
      setValues(idx)
      setLibraries(libs as Library[])
      setDirty(new Set())
      setLoadError('')
    } catch (err: unknown) {
      // 加载失败时保留错误态，避免把表单默认值误当成已保存的配置
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '加载成人设置失败'
      setLoadError(msg)
      toast.error(msg)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  const setVal = (key: string, value: string) => {
    setValues((prev) => ({ ...prev, [key]: value }))
    setDirty((prev) => new Set(prev).add(key))
  }

  const toggleVal = (key: string) => {
    const current = values[key] === 'true' || values[key] === '1' || values[key] === 'on'
    setVal(key, current ? 'false' : 'true')
  }

  const isChecked = (key: string, def = false) => {
    const val = values[key]
    if (val === undefined) return def
    return val === 'true' || val === '1' || val === 'on'
  }

  const onSave = async (e?: FormEvent) => {
    if (e) e.preventDefault()
    if (dirty.size === 0) return
    setSaving(true)
    try {
      for (const key of dirty) {
        await adminAPI.updateSetting(key, values[key] ?? '')
      }
      toast.success(`已保存 ${dirty.size} 项配置`)
      setDirty(new Set())
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '保存失败'
      toast.error(msg)
    } finally {
      setSaving(false)
    }
  }

  const selectedLibraryIDs = parseLibraryIDs(values['adult.library_ids'] || '[]')
  const toggleLibrary = (id: string, checked: boolean) => {
    const next = checked
      ? Array.from(new Set([...selectedLibraryIDs, id]))
      : selectedLibraryIDs.filter((item) => item !== id)
    setVal('adult.library_ids', JSON.stringify(next))
  }

  if (loading) {
    return (
      <div className="flex justify-center py-12 text-ink-50">
        <Loader2 className="animate-spin" />
      </div>
    )
  }

  if (loadError) {
    return (
      <div className="glass-panel flex flex-col items-center gap-3 py-10 text-center">
        <XCircle className="text-rose-500" size={24} />
        <p className="text-sm text-ink-100">成人设置加载失败：{loadError}</p>
        <p className="text-xs text-sand-500">当前展示的并非已保存配置，请重新加载后再修改</p>
        <button type="button" onClick={() => refresh()} className="neon-button px-4! py-1.5! text-xs!">
          重试
        </button>
      </div>
    )
  }

  return (
    <form onSubmit={onSave} className="space-y-6">
      {/* 1. 全局访问与隔离卡片 */}
      <div className="glass-panel space-y-4">
        <div className="flex items-center gap-3 border-b border-gray-200/60 pb-3">
          <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-pink-500/10 text-pink-500">
            <Lock size={18} />
          </div>
          <div>
            <h2 className="text-base font-semibold text-ink-600">访问与媒体库隔离控制</h2>
            <p className="text-xs text-sand-500">
              设置全局成人内容显示规则、指定成人影视库与访问 PIN 保护
            </p>
          </div>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          {/* 主开关 */}
          <div className="rounded-2xl border border-gray-200/60 bg-white/50 p-4">
            <label className="flex cursor-pointer items-start justify-between gap-4">
              <div>
                <span className="block text-sm font-medium text-ink-600">启用成人内容</span>
                <span className="mt-0.5 block text-xs text-sand-500">
                  全局开关。关闭后系统隐藏所有成人库；开启后普通用户仍默认隐藏，可在个人资料或 Bot 自行开启。
                </span>
              </div>
              <input
                type="checkbox"
                className="mt-1 h-5 w-5 accent-pink-500"
                checked={isChecked('adult.enabled', true)}
                onChange={() => toggleVal('adult.enabled')}
              />
            </label>
          </div>

          {/* PIN 保护 */}
          <div className="rounded-2xl border border-gray-200/60 bg-white/50 p-4">
            <label className="flex cursor-pointer items-start justify-between gap-4">
              <div>
                <span className="block text-sm font-medium text-ink-600">访问需要 PIN 码保护</span>
                <span className="mt-0.5 block text-xs text-sand-500">
                  开启后，客户端或网页端浏览成人库内容需输入数字 PIN 码
                </span>
              </div>
              <input
                type="checkbox"
                className="mt-1 h-5 w-5 accent-pink-500"
                checked={isChecked('adult.require_pin', false)}
                onChange={() => toggleVal('adult.require_pin')}
              />
            </label>
            {isChecked('adult.require_pin', false) && (
              <div className="mt-3">
                <input
                  type="text"
                  className="input-base text-sm"
                  placeholder="请输入 4-8 位数字 PIN"
                  value={values['adult.pin'] ?? ''}
                  onChange={(e) => setVal('adult.pin', e.target.value)}
                />
              </div>
            )}
          </div>
        </div>

        {/* 成人媒体库绑定 */}
        <div className="rounded-2xl border border-gray-200/60 bg-white/50 p-4">
          <div className="mb-2">
            <span className="block text-sm font-medium text-ink-600">指定成人媒体库</span>
            <span className="text-xs text-sand-500">
              指定哪些媒体库属于成人分类。指定后，搜索、分类及 Emby/Jellyfin 协议都会统一应用隔离与过滤规则。
            </span>
          </div>

          <div className="mt-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {libraries.length === 0 && (
              <div className="col-span-full py-2 text-xs text-sand-500">暂无媒体库</div>
            )}
            {libraries.map((lib) => {
              const checked = selectedLibraryIDs.includes(lib.id)
              return (
                <label
                  key={lib.id}
                  className={
                    'flex cursor-pointer items-start gap-2.5 rounded-xl border p-2.5 transition ' +
                    (checked
                      ? 'border-pink-500/40 bg-pink-500/5'
                      : 'border-gray-200/60 bg-white/80 hover:bg-gray-50')
                  }
                >
                  <input
                    type="checkbox"
                    className="mt-0.5 h-4 w-4 accent-pink-500"
                    checked={checked}
                    onChange={(e) => toggleLibrary(lib.id, e.target.checked)}
                  />
                  <div className="min-w-0">
                    <span className="block truncate text-xs font-medium text-ink-600">
                      {lib.name}
                    </span>
                    <span className="block truncate font-mono text-[11px] text-sand-500">
                      {lib.path}
                    </span>
                  </div>
                </label>
              )
            })}
          </div>
        </div>
      </div>

      <div className="flex items-center justify-between pt-2">
        <span className="text-xs text-sand-500">
          {dirty.size > 0 ? `有 ${dirty.size} 项修改未保存` : '所有设置已是最新'}
        </span>
        <button
          type="submit"
          disabled={saving || dirty.size === 0}
          className="neon-button disabled:opacity-50"
        >
          {saving ? <Loader2 size={16} className="animate-spin" /> : <Save size={16} />}
          保存配置
        </button>
      </div>
    </form>
  )
}

function parseLibraryIDs(raw: string): string[] {
  try {
    const parsed = JSON.parse(raw || '[]')
    if (Array.isArray(parsed)) {
      return parsed.map((item) => String(item).trim()).filter(Boolean)
    }
  } catch {
    return raw
      .split(/[,\n;，]/)
      .map((item) => item.trim())
      .filter(Boolean)
  }
  return []
}
