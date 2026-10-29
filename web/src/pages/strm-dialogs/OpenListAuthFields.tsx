import { useState } from 'react'

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
  token, onTokenChange, hasToken,
}: {
  token: string
  onTokenChange: (value: string) => void
  hasToken: boolean
}) {
  return (
    <SecretField label="Token" name="strm-openlist-token" value={token} hasSavedValue={hasToken} onChange={onTokenChange} />
  )
}
