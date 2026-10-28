import { useState } from 'react'
import type { OpenListAuthMode } from '../../utils/openListAuth'

function SecretField({
  label, name, value, hasSavedValue, onChange,
}: {
  label: string
  name: string
  value: string
  hasSavedValue: boolean
  onChange: (value: string) => void
}) {
  const [replacing, setReplacing] = useState(Boolean(value))
  const showSaved = hasSavedValue && !replacing

  return (
    <div className="min-w-0 space-y-2">
      <div className="flex items-center justify-between gap-2">
        <label htmlFor={showSaved ? undefined : name} className="text-sm font-medium text-ink-100">{label}</label>
        {hasSavedValue && replacing && (
          <button type="button" className="text-xs text-ink-100 underline underline-offset-4" onClick={() => {
            onChange('')
            setReplacing(false)
          }}>取消更换</button>
        )}
      </div>
      {showSaved ? (
        <div className="input-base flex w-full items-center gap-3">
          <span aria-hidden="true" className="min-w-0 flex-1 tracking-wider">••••••••••••</span>
          <span className="text-xs text-ink-100">已保存</span>
          <button type="button" className="shrink-0 text-sm font-medium text-ink-600 underline underline-offset-4" aria-label={`更换${label}`} onClick={() => setReplacing(true)}>更换</button>
        </div>
      ) : (
        <input
          id={name}
          name={name}
          type="password"
          autoComplete="new-password"
          autoCapitalize="none"
          spellCheck={false}
          className="input-base w-full"
          value={value}
          placeholder={hasSavedValue ? `输入新的${label}` : `输入${label}`}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
    </div>
  )
}

export function OpenListAuthFields({
  mode, onModeChange, username, onUsernameChange, password, onPasswordChange,
  token, onTokenChange, hasPassword, hasToken,
}: {
  mode: OpenListAuthMode
  onModeChange: (mode: OpenListAuthMode) => void
  username: string
  onUsernameChange: (value: string) => void
  password: string
  onPasswordChange: (value: string) => void
  token: string
  onTokenChange: (value: string) => void
  hasPassword: boolean
  hasToken: boolean
}) {
  return (
    <div className="space-y-4">
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium text-ink-100">认证方式</legend>
        <div className="flex flex-wrap gap-x-6 gap-y-2">
          {(['token', 'password'] as const).map((option) => (
            <label key={option} className="flex cursor-pointer items-center gap-2 text-sm text-ink-600">
              <input type="radio" name="strm-openlist-auth" value={option} checked={mode === option} onChange={() => onModeChange(option)} className="h-4 w-4 accent-primary-400" />
              {option === 'token' ? 'Token' : '用户名和密码'}
            </label>
          ))}
        </div>
      </fieldset>
      {mode === 'token' ? (
        <SecretField key="token" label="Token" name="strm-openlist-token" value={token} hasSavedValue={hasToken} onChange={onTokenChange} />
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <label className="block min-w-0 space-y-2">
            <span className="text-sm font-medium text-ink-100">用户名</span>
            <input name="strm-openlist-username" autoComplete="off" autoCapitalize="none" spellCheck={false} className="input-base w-full" placeholder="输入用户名" value={username} onChange={(event) => onUsernameChange(event.target.value)} />
          </label>
          <SecretField key="password" label="密码" name="strm-openlist-password" value={password} hasSavedValue={hasPassword} onChange={onPasswordChange} />
        </div>
      )}
    </div>
  )
}
