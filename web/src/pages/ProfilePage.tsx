import { useEffect, useState, type FormEvent, type KeyboardEvent, type ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import toast from 'react-hot-toast'
import { Check, Copy, KeyRound, Loader2, Save, Tv } from 'lucide-react'

import { authAPI } from '../api/auth'
import { profileAPI } from '../api/profile'
import { MyDevicesPanel } from '../components/MyDevicesPanel'
import { requestPassword } from '../components/requestPassword'
import { useAuthStore } from '../stores/auth'
import { ProfilePlaybackSettings } from './ProfilePlaybackSettings'

const PROFILE_TABS = [
  { value: 'info', label: '资料与安全' },
  { value: 'playback', label: '播放设置' },
  { value: 'devices', label: '我的设备' },
] as const

type ProfileTab = typeof PROFILE_TABS[number]['value']

export function ProfilePage() {
  const user = useAuthStore((s) => s.user)
  const setUser = useAuthStore((s) => s.setUser)
  const location = useLocation()
  const navigate = useNavigate()
  const requestedTab = new URLSearchParams(location.search).get('tab')
  const activeTab: ProfileTab = location.hash === '#tv-otp'
    ? 'devices'
    : PROFILE_TABS.find((tab) => tab.value === requestedTab)?.value ?? 'info'

  const [username, setUsername] = useState(user?.username ?? '')
  const [nickname, setNickname] = useState(user?.nickname ?? '')
  const [email, setEmail] = useState(user?.email ?? '')
  const [avatar, setAvatar] = useState(user?.avatar_url ?? '')
  const [oldPwd, setOldPwd] = useState('')
  const [newPwd, setNewPwd] = useState('')
  const [savingProfile, setSavingProfile] = useState(false)
  const [savingPassword, setSavingPassword] = useState(false)

  const [otpCode, setOtpCode] = useState('')
  const [otpExpiresIn, setOtpExpiresIn] = useState(0)
  const [generatingOtp, setGeneratingOtp] = useState(false)
  const [copiedOtp, setCopiedOtp] = useState(false)

  useEffect(() => {
    if (otpExpiresIn <= 0) return
    const timer = setInterval(() => {
      setOtpExpiresIn((prev) => {
        if (prev <= 1) {
          clearInterval(timer)
          return 0
        }
        return prev - 1
      })
    }, 1000)
    return () => clearInterval(timer)
  }, [otpExpiresIn])

  useEffect(() => {
    if (location.hash !== '#tv-otp') return
    document.getElementById('tv-otp')?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
  }, [location.hash])

  const changeTab = (tab: ProfileTab) => {
    const params = new URLSearchParams(location.search)
    if (tab === 'info') params.delete('tab')
    else params.set('tab', tab)
    navigate({ pathname: location.pathname, search: params.toString(), hash: '' }, { preventScrollReset: true })
  }

  const onTabKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    let next: number
    if (event.key === 'ArrowRight') next = (index + 1) % PROFILE_TABS.length
    else if (event.key === 'ArrowLeft') next = (index + PROFILE_TABS.length - 1) % PROFILE_TABS.length
    else if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = PROFILE_TABS.length - 1
    else return
    event.preventDefault()
    changeTab(PROFILE_TABS[next].value)
    document.getElementById(`profile-tab-${PROFILE_TABS[next].value}`)?.focus()
  }

  const onGenerateOTP = async () => {
    if (generatingOtp) return
    setGeneratingOtp(true)
    try {
      const res = await authAPI.createTemporaryPassword()
      setOtpCode(res.code)
      setOtpExpiresIn(res.expires_in)
      setCopiedOtp(false)
      toast.success('已生成 6 位临时登录码')
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '生成临时密码失败'
      toast.error(msg)
    } finally {
      setGeneratingOtp(false)
    }
  }

  const copyOTP = async () => {
    if (!otpCode) return
    try {
      await navigator.clipboard.writeText(otpCode)
      setCopiedOtp(true)
      toast.success('已复制到剪贴板')
      setTimeout(() => setCopiedOtp(false), 2000)
    } catch {
      toast.error('复制失败，请手动长按复制')
    }
  }

  const onProfile = async (e: FormEvent) => {
    e.preventDefault()
    if (savingProfile) return
    setSavingProfile(true)
    try {
      let password: string | undefined
      const usernameChanged = username.trim() !== (user?.username ?? '')
      if (usernameChanged) {
        const input = await requestPassword({
          title: '修改用户名',
          message: '修改用户名后需要使用新用户名登录，请输入当前账号密码确认。',
          confirmText: '保存设置',
        })
        if (!input) return
        password = input
      }
      const patch: Parameters<typeof profileAPI.update>[0] = {
        username,
        nickname,
        email,
        avatar_url: avatar,
        password,
      }
      const u = await profileAPI.update(patch)
      setUser(u)
      toast.success('资料已更新')
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '保存失败'
      toast.error(msg)
    } finally {
      setSavingProfile(false)
    }
  }

  const onPwd = async (e: FormEvent) => {
    e.preventDefault()
    if (savingPassword) return
    setSavingPassword(true)
    try {
      await authAPI.changePassword(oldPwd, newPwd)
      toast.success('密码已更新')
      setOldPwd('')
      setNewPwd('')
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '密码更新失败'
      toast.error(msg)
    } finally {
      setSavingPassword(false)
    }
  }

  return (
    <div className="space-y-4 text-(--app-text)">
      <header>
        <h1 className="font-display text-2xl font-semibold">个人资料</h1>
      </header>

      <div role="tablist" aria-label="个人资料分类" className="flex gap-5 border-b border-(--app-border)">
        {PROFILE_TABS.map((tab, index) => (
          <button
            key={tab.value}
            type="button"
            id={`profile-tab-${tab.value}`}
            role="tab"
            aria-selected={activeTab === tab.value}
            aria-controls={`profile-panel-${tab.value}`}
            tabIndex={activeTab === tab.value ? 0 : -1}
            onClick={() => changeTab(tab.value)}
            onKeyDown={(event) => onTabKeyDown(event, index)}
            className={`-mb-px min-h-11 border-b-2 py-2 text-sm font-medium transition-colors ${activeTab === tab.value ? 'border-(--app-text) text-(--app-text)' : 'border-transparent text-(--app-muted) hover:text-(--app-text)'}`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      <div id="profile-panel-info" role="tabpanel" aria-labelledby="profile-tab-info" hidden={activeTab !== 'info'} className="space-y-4">
        <form onSubmit={onProfile} className="rounded-xl bg-(--app-panel) p-4 sm:p-5">
          <h2 className="mb-3 text-base font-semibold">基本信息</h2>
          <fieldset disabled={savingProfile} className="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2 sm:gap-4">
            <Field label="用户名">
              <input
                required
                className="input-base"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoComplete="username"
              />
            </Field>
            <Field label="昵称">
              <input
                className="input-base"
                value={nickname}
                onChange={(e) => setNickname(e.target.value)}
              />
            </Field>
            <Field label="电子邮箱">
              <input
                type="email"
                className="input-base"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </Field>
            <Field label="头像 URL">
              <input
                className="input-base"
                value={avatar}
                onChange={(e) => setAvatar(e.target.value)}
              />
            </Field>
          </fieldset>
          <div className="mt-3 flex justify-end">
            <button type="submit" disabled={savingProfile} className="btn-primary">
              {savingProfile ? <Loader2 size={16} className="animate-spin" /> : <Save size={16} />}
              {savingProfile ? '保存中…' : '保存资料'}
            </button>
          </div>
        </form>

        <form onSubmit={onPwd} className="rounded-xl bg-(--app-panel) p-4 sm:p-5">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
            <h2 className="text-base font-semibold">修改密码</h2>
            <span className="text-xs text-(--app-muted)">新密码至少 6 位</span>
          </div>
          <fieldset disabled={savingPassword} className="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2 sm:gap-4">
            <Field label="当前密码">
              <input
                required
                type="password"
                className="input-base"
                value={oldPwd}
                onChange={(e) => setOldPwd(e.target.value)}
                autoComplete="current-password"
              />
            </Field>
            <Field label="新密码">
              <input
                required
                type="password"
                className="input-base"
                minLength={6}
                value={newPwd}
                onChange={(e) => setNewPwd(e.target.value)}
                autoComplete="new-password"
              />
            </Field>
          </fieldset>
          <div className="mt-3 flex justify-end">
            <button type="submit" disabled={savingPassword} className="btn-outline">
              {savingPassword ? <Loader2 size={16} className="animate-spin" /> : <KeyRound size={16} />}
              {savingPassword ? '更新中…' : '更新密码'}
            </button>
          </div>
        </form>
      </div>

      <div id="profile-panel-playback" role="tabpanel" aria-labelledby="profile-tab-playback" hidden={activeTab !== 'playback'}>
        <ProfilePlaybackSettings active={activeTab === 'playback'} />
      </div>

      <div id="profile-panel-devices" role="tabpanel" aria-labelledby="profile-tab-devices" hidden={activeTab !== 'devices'} className="space-y-4">
        <section id="tv-otp" className="space-y-3 rounded-xl bg-(--app-panel) p-4 sm:p-5">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 className="flex items-center gap-2 text-base font-semibold">
                <Tv size={18} />
                电视端临时登录码
              </h2>
              <p className="mt-1 text-xs leading-5 text-(--app-muted)">
                用于 Apple TV、电视盒子等设备上的 Emby / Infuse / Jellyfin 客户端快速登录。
              </p>
            </div>
            <button
              type="button"
              onClick={onGenerateOTP}
              disabled={generatingOtp}
              className="btn-outline shrink-0"
            >
              {generatingOtp ? <Loader2 size={16} className="animate-spin" /> : <KeyRound size={16} />}
              {otpCode && otpExpiresIn > 0 ? '重新生成' : '获取登录码'}
            </button>
          </div>

          {otpCode && otpExpiresIn > 0 ? (
            <div className="space-y-3 rounded-xl border border-(--app-border) p-3">
              <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-(--app-muted)">
                <div>
                  <span>登录账号：</span>
                  <span className="break-all font-medium text-(--app-text)">{user?.username}</span>
                </div>
                <div>
                  <span>有效时间剩余：</span>
                  <span className={`font-medium tabular-nums ${otpExpiresIn < 60 ? 'text-red-500' : 'text-(--app-text)'}`}>
                    {Math.floor(otpExpiresIn / 60).toString().padStart(2, '0')}:{(otpExpiresIn % 60).toString().padStart(2, '0')}
                  </span>
                </div>
              </div>

              <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg bg-(--app-control-bg) p-3">
                <div className="select-all text-2xl font-semibold tracking-widest tabular-nums">
                  {otpCode}
                </div>
                <button
                  type="button"
                  onClick={copyOTP}
                  className="btn-outline"
                >
                  {copiedOtp ? <Check size={14} /> : <Copy size={14} />}
                  {copiedOtp ? '已复制' : '复制登录码'}
                </button>
              </div>

              <p className="text-xs leading-5 text-(--app-muted)">
                在客户端输入用户名和上方 6 位数字即可登录。登录码一次性使用，登录成功后立即失效。
              </p>
            </div>
          ) : null}
        </section>

        {activeTab === 'devices' && <MyDevicesPanel />}
      </div>
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block min-w-0">
      <span className="mb-1.5 block text-sm text-(--app-muted)">{label}</span>
      {children}
    </label>
  )
}
