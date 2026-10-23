import { useCallback, useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { Loader2, Save } from 'lucide-react'
import toast from 'react-hot-toast'

import { playbackSettingsAPI, type PlaybackSettings } from '../api/playbackSettings'
import { requestPassword } from '../components/requestPassword'
import { useAuthStore } from '../stores/auth'
import type { Library } from '../types'

export function ProfilePlaybackSettings({ active }: { active: boolean }) {
  const setUser = useAuthStore((s) => s.setUser)
  const [form, setForm] = useState<PlaybackSettings | null>(null)
  const [saved, setSaved] = useState<PlaybackSettings | null>(null)
  const [libraries, setLibraries] = useState<Library[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [saving, setSaving] = useState(false)
  const activatedRef = useRef(false)
  const load = useCallback(async () => {
    setLoading(true)
    setError(false)
    try {
      const [settings, rows] = await Promise.all([playbackSettingsAPI.get(), playbackSettingsAPI.libraries()])
      setForm(settings)
      setSaved(settings)
      setLibraries(rows)
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }, [])
  useEffect(() => {
    if (!active || activatedRef.current) return
    activatedRef.current = true
    void load()
  }, [active, load])

  const update = (patch: Partial<PlaybackSettings>) => setForm((current) => current ? { ...current, ...patch } : current)
  const dirty = JSON.stringify(form) !== JSON.stringify(saved)
  const onSave = async (event: FormEvent) => {
    event.preventDefault()
    if (!form || !saved || saving) return
    setSaving(true)
    try {
      let password: string | undefined
      if (form.allow_adult !== saved.allow_adult) {
        const value = await requestPassword({
          title: '修改成人内容访问',
          message: '此设置会影响网页和第三方客户端，请输入当前账号密码确认。',
          confirmText: '保存设置',
        })
        if (!value) return
        password = value
      }
      const result = await playbackSettingsAPI.save(form, password)
      setForm(result)
      setSaved(result)
      const currentUser = useAuthStore.getState().user
      if (currentUser) setUser({ ...currentUser, hide_adult: !result.allow_adult })
      toast.success('播放设置已保存')
    } catch (err: unknown) {
      toast.error((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '保存失败，请重试')
    } finally {
      setSaving(false)
    }
  }

  return (
    <form onSubmit={onSave} className="space-y-4">
      {loading ? (
        <p role="status" className="flex items-center gap-2 py-6 text-sm text-(--app-muted)"><Loader2 size={18} className="animate-spin" />正在加载播放设置…</p>
      ) : error || !form ? (
        <div role="alert" className="rounded-xl border border-(--app-border) p-4">
          <p className="text-sm">播放设置加载失败。</p>
          <button type="button" className="btn-secondary mt-4" onClick={() => void load()}>重新加载</button>
        </div>
      ) : (
        <fieldset disabled={saving} className="min-w-0 space-y-4">
          <section className="rounded-xl bg-(--app-panel) p-4 sm:p-5">
            <h2 className="text-base font-semibold">播放偏好</h2>
            <p className="mt-1 text-xs leading-5 text-(--app-muted)">字幕和音轨偏好会提供给兼容客户端，实际支持情况由客户端决定。</p>
            <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2 sm:gap-4">
              <LanguageField label="首选字幕" value={form.preferred_subtitle_lang} onChange={(value) => update({ preferred_subtitle_lang: value })} />
              <LanguageField label="首选音轨" value={form.preferred_audio_lang} onChange={(value) => update({ preferred_audio_lang: value })} />
            </div>
            <div className="mt-3 divide-y divide-(--app-border)">
              <SettingToggle label="自动播放下一集" checked={form.autoplay_next} onChange={(value) => update({ autoplay_next: value })} />
              <SettingToggle label="自动跳过片头" hint="用于 Nook 网页播放器；第三方客户端使用各自的跳过设置。" checked={form.skip_intro} onChange={(value) => update({ skip_intro: value })} />
            </div>
          </section>
          <section className="rounded-xl bg-(--app-panel) p-4 sm:p-5">
            <h2 className="text-base font-semibold">内容与媒体库</h2>
            <div className="mt-3">
              <Field label="内容分级偏好">
                <select className="input-base" value={form.content_rating_limit} onChange={(event) => update({ content_rating_limit: event.target.value })}>
                  <option value="">不限制</option>
                  {['G', 'PG', 'PG-13', 'R', 'NC-17'].map((rating) => <option key={rating} value={rating}>{rating}</option>)}
                </select>
              </Field>
              <p className="mt-2 text-xs leading-5 text-(--app-muted)">保留此项偏好，内容分级过滤暂未启用。</p>
            </div>
            <SettingToggle label="允许成人内容" hint="关闭后，网页与第三方客户端均隐藏成人目录和 NSFW 条目；仍受系统设置限制。" checked={form.allow_adult} onChange={(value) => update({ allow_adult: value })} />
            <div className="border-t border-(--app-border) pt-3">
              <h3 className="text-sm font-medium">允许访问的媒体库</h3>
              <p className="mt-1 text-xs leading-5 text-(--app-muted)">未选择时显示账号有权访问的全部媒体库，选择范围不会扩大管理员授予的权限。</p>
              {libraries.length ? (
                <div className="mt-3 flex flex-wrap gap-2">
                  {libraries.map((library) => (
                    <label key={library.id} className="flex min-h-10 max-w-full cursor-pointer items-center gap-2 rounded-lg border border-(--app-border) px-3 py-2 text-sm hover:bg-(--app-hover)">
                      <input type="checkbox" className="h-4 w-4 shrink-0 accent-brand-500" checked={form.allowed_library_ids.includes(library.id)} onChange={(event) => update({ allowed_library_ids: event.target.checked ? [...form.allowed_library_ids, library.id] : form.allowed_library_ids.filter((id) => id !== library.id) })} />
                      <span className="min-w-0 break-words">{library.name}</span>
                    </label>
                  ))}
                </div>
              ) : <p className="mt-3 text-sm text-(--app-muted)">暂无可访问的媒体库。</p>}
            </div>
          </section>
        </fieldset>
      )}
      <div className="flex justify-end">
        <button type="submit" className="btn-primary" disabled={loading || error || saving || !dirty}>
          {saving ? <Loader2 size={16} className="animate-spin" /> : <Save size={16} />}
          {saving ? '保存中…' : '保存播放设置'}
        </button>
      </div>
    </form>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="block min-w-0 space-y-1.5 text-sm"><span className="text-(--app-muted)">{label}</span>{children}</label>
}

function LanguageField({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return <Field label={label}><select className="input-base" value={value} onChange={(event) => onChange(event.target.value)}>
    <option value="">跟随播放器默认</option><option value="zh">中文</option><option value="zh-CN">简体中文</option><option value="en">English</option><option value="ja">日语</option>
  </select></Field>
}

function SettingToggle({ label, hint, checked, onChange }: { label: string; hint?: string; checked: boolean; onChange: (value: boolean) => void }) {
  return <label className="flex min-h-12 cursor-pointer items-center justify-between gap-4 py-3">
    <span><span className="block text-sm font-medium">{label}</span>{hint && <span className="mt-1 block text-xs leading-5 text-(--app-muted)">{hint}</span>}</span>
    <input type="checkbox" className="h-5 w-5 shrink-0 accent-brand-500" checked={checked} onChange={(event) => onChange(event.target.checked)} />
  </label>
}
