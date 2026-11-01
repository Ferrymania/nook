import { useEffect, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import toast from 'react-hot-toast'

import { authAPI } from '../api/auth'
import { useAuthStore } from '../stores/auth'
import { LoginPageShell } from './LoginPageSections'

export function ChangeInitialPasswordPage() {
  const navigate = useNavigate()
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    let active = true
    void authAPI.me().then((user) => {
      if (!active) return
      useAuthStore.getState().setUser(user)
      if (!user.force_password_reset) navigate('/', { replace: true })
    }).catch(() => {
      if (active) setError('无法确认账号状态，请稍后重试。')
    })
    return () => { active = false }
  }, [navigate])

  const signOut = async () => {
    try {
      await authAPI.logout()
    } finally {
      useAuthStore.getState().logout()
      navigate('/login', { replace: true })
    }
  }

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    if (newPassword !== confirmation) {
      setError('两次输入的新密码不一致。')
      return
    }
    if (newPassword === oldPassword || newPassword === 'admin123') {
      setError('请设置与当前密码不同的新密码，不能使用 admin123。')
      return
    }
    setSaving(true)
    try {
      await authAPI.changePassword(oldPassword, newPassword)
      useAuthStore.getState().logout()
      toast.success('密码已修改，请重新登录')
      navigate('/login', { replace: true })
    } catch (err: unknown) {
      setError((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '修改失败，请重试。')
    } finally {
      setSaving(false)
    }
  }

  return (
    <LoginPageShell>
      <main className="relative z-10 my-8 w-full max-w-[420px] rounded-3xl border border-gray-200 bg-white p-7 sm:p-9">
        <div className="mb-7 flex items-center gap-3">
          <img src="/brand/logo-192.png?v=2" alt="Nook" className="h-11 w-11" />
          <div>
            <h1 className="text-xl font-bold text-gray-900">修改初始密码</h1>
            <p className="mt-1 text-sm text-gray-600">修改后才能继续使用 Nook。</p>
          </div>
        </div>
        <form className="space-y-4" onSubmit={handleSubmit}>
          <label className="block space-y-2 text-sm font-medium text-gray-700">
            <span>当前密码</span>
            <input className="input-base w-full" type="password" autoComplete="current-password" required autoFocus value={oldPassword} onChange={(event) => setOldPassword(event.target.value)} />
          </label>
          <label className="block space-y-2 text-sm font-medium text-gray-700">
            <span>新密码</span>
            <input className="input-base w-full" type="password" autoComplete="new-password" required minLength={6} placeholder="至少 6 位" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} />
          </label>
          <label className="block space-y-2 text-sm font-medium text-gray-700">
            <span>确认新密码</span>
            <input className="input-base w-full" type="password" autoComplete="new-password" required minLength={6} value={confirmation} onChange={(event) => setConfirmation(event.target.value)} />
          </label>
          {error && <p role="alert" className="text-sm text-red-600">{error}</p>}
          <button className="min-h-11 w-full rounded-xl bg-gray-900 px-4 py-3 text-sm font-semibold text-white hover:bg-gray-800 disabled:opacity-50" disabled={saving} type="submit">
            {saving ? '正在保存…' : '修改密码并重新登录'}
          </button>
          <button className="min-h-11 w-full text-sm text-gray-600 hover:text-gray-900 disabled:opacity-50" type="button" disabled={saving} onClick={() => { void signOut().catch(() => undefined) }}>
            退出登录
          </button>
        </form>
      </main>
    </LoginPageShell>
  )
}
