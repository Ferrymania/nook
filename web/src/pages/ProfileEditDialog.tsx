import { useEffect, useId, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import toast from 'react-hot-toast'
import { Loader2, Save, Upload, X } from 'lucide-react'

import { profileAPI } from '../api/profile'
import { UserAvatar } from '../components/UserAvatar'
import { useAuthStore } from '../stores/auth'
import type { User } from '../types'

export function ProfileEditDialog({ user, onClose }: { user: User; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const usernameInput = useRef<HTMLInputElement>(null)
  const avatarInput = useRef<HTMLInputElement>(null)
  const titleID = useId()
  const avatarHelpID = useId()
  const [username, setUsername] = useState(user.username)
  const [nickname, setNickname] = useState(user.nickname ?? '')
  const [email, setEmail] = useState(user.email ?? '')
  const [password, setPassword] = useState('')
  const [saving, setSaving] = useState(false)
  const [avatarAction, setAvatarAction] = useState<'upload' | 'remove' | null>(null)
  const [error, setError] = useState('')
  const [avatarError, setAvatarError] = useState('')
  const busy = saving || avatarAction !== null
  const usernameChanged = username.trim() !== user.username

  useEffect(() => {
    const el = dialog.current
    if (!el) return
    const previousFocus = document.activeElement as HTMLElement | null
    el.showModal()
    usernameInput.current?.focus()
    return () => {
      el.close()
      previousFocus?.focus({ preventScroll: true })
    }
  }, [])

  const close = () => { if (!busy) onClose() }

  const save = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (busy || !username.trim() || (usernameChanged && !password)) return
    setSaving(true)
    setError('')
    try {
      const updated = await profileAPI.update({ username, nickname, email, password: usernameChanged ? password : undefined })
      if (useAuthStore.getState().user?.id === updated.id) {
        useAuthStore.getState().setUser(updated)
        toast.success('资料已更新')
      }
      onClose()
    } catch (err: unknown) {
      setError(profileErrorMessage(err, '保存资料失败，请重试'))
    } finally {
      setSaving(false)
    }
  }

  const changeAvatar = async (file: File | null) => {
    if (busy) return
    if (file && file.size > 5 * 1024 * 1024) {
      setAvatarError('头像图片不能超过 5 MB')
      return
    }
    setAvatarError('')
    setAvatarAction(file ? 'upload' : 'remove')
    try {
      const updated = await (file ? profileAPI.uploadAvatar(file) : profileAPI.removeAvatar())
      const current = useAuthStore.getState().user
      if (current?.id === updated.id) {
        useAuthStore.getState().setUser({ ...current, avatar_url: updated.avatar_url })
      }
    } catch (err: unknown) {
      setAvatarError(profileErrorMessage(err, file ? '头像上传失败，请重试' : '头像移除失败，请重试'))
    } finally {
      setAvatarAction(null)
    }
  }

  return createPortal(
    <dialog ref={dialog} aria-labelledby={titleID} onCancel={(event) => {
      // The file picker also emits a cancel event that bubbles to the dialog.
      if (event.target !== event.currentTarget) return
      event.preventDefault()
      close()
    }}
      className="m-auto max-h-[calc(100dvh-2rem)] w-[calc(100%-2rem)] max-w-lg overflow-y-auto rounded-2xl border border-(--app-border) bg-(--app-panel-elevated) p-0 text-(--app-text) shadow-xl backdrop:bg-black/40">
      <form onSubmit={save}>
        <header className="flex items-center justify-between gap-3 border-b border-(--app-border) px-5 py-3">
          <h2 id={titleID} className="font-display text-lg font-semibold">编辑资料</h2>
          <button type="button" aria-label="关闭编辑资料" className="btn-ghost h-11 w-11 p-0!" disabled={busy} onClick={close}>
            <X size={18} aria-hidden="true" />
          </button>
        </header>
        <div className="space-y-4 p-5">
          <div className="flex items-start gap-3 border-b border-(--app-border) pb-4" aria-busy={avatarAction !== null}>
            <UserAvatar user={user} square className="size-14 text-base" />
            <div className="min-w-0 space-y-2">
              <input ref={avatarInput} type="file" accept="image/jpeg,image/png,image/webp" hidden onChange={(event) => {
                const file = event.currentTarget.files?.[0]
                event.currentTarget.value = ''
                if (file) void changeAvatar(file)
              }} />
              <div className="flex flex-wrap items-center gap-2">
                <button type="button" className="btn-outline min-h-10" aria-describedby={avatarHelpID} disabled={busy} onClick={() => avatarInput.current?.click()}>
                  {avatarAction === 'upload' ? <Loader2 size={15} className="animate-spin" aria-hidden="true" /> : <Upload size={15} aria-hidden="true" />}
                  {avatarAction === 'upload' ? '上传中…' : user.avatar_url ? '更换头像' : '上传头像'}
                </button>
                {user.avatar_url && <button type="button" className="btn-ghost min-h-10" disabled={busy} onClick={() => void changeAvatar(null)}>
                  {avatarAction === 'remove' ? '移除中…' : '移除头像'}
                </button>}
              </div>
              <p id={avatarHelpID} className="text-xs text-(--app-muted)">JPG、PNG、WebP，最大 5 MB。头像更改立即生效。</p>
              {avatarError && <p role="alert" className="text-sm text-(--app-danger-text)">{avatarError}</p>}
            </div>
          </div>
          <fieldset disabled={busy} className="grid min-w-0 gap-3 sm:grid-cols-2">
            <ProfileField label="用户名"><input ref={usernameInput} required className="input-base" value={username}
              onChange={(event) => { setUsername(event.target.value); setPassword('') }} autoComplete="username" /></ProfileField>
            <ProfileField label="昵称"><input className="input-base" value={nickname}
              onChange={(event) => setNickname(event.target.value)} /></ProfileField>
            <div className="sm:col-span-2"><ProfileField label="电子邮箱"><input type="email" className="input-base" value={email}
              onChange={(event) => setEmail(event.target.value)} /></ProfileField></div>
            {usernameChanged && <div className="sm:col-span-2"><ProfileField label="修改用户名需要当前密码">
              <input type="password" required className="input-base" value={password} autoComplete="current-password"
                onChange={(event) => setPassword(event.target.value)} />
            </ProfileField></div>}
          </fieldset>
          {error && <p role="alert" className="text-sm text-(--app-danger-text)">{error}</p>}
        </div>
        <footer className="flex justify-end gap-2 border-t border-(--app-border) px-5 py-4">
          <button type="button" className="btn-outline min-h-11" disabled={busy} onClick={close}>取消</button>
          <button type="submit" className="btn-primary min-h-11" disabled={busy || !username.trim() || (usernameChanged && !password)}>
            {saving ? <Loader2 size={16} className="animate-spin" aria-hidden="true" /> : <Save size={16} aria-hidden="true" />}
            {saving ? '保存中…' : '保存资料'}
          </button>
        </footer>
      </form>
    </dialog>, document.body,
  )
}

function ProfileField({ label, children }: { label: string; children: ReactNode }) {
  return <label className="block min-w-0">
    <span className="mb-1.5 block text-sm text-(--app-muted)">{label}</span>
    {children}
  </label>
}

function profileErrorMessage(error: unknown, fallback: string): string {
  const message = (error as { response?: { data?: { error?: string } } })?.response?.data?.error
  return message === 'username already taken' ? '用户名已存在' : message || fallback
}
