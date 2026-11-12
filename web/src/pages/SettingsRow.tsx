import { useId } from 'react'

export interface SettingDef {
  key: string
  label: string
  type: 'text' | 'select' | 'toggle' | 'number' | 'textarea'
  hint?: string
  defaultValue?: string
  options?: { value: string; label: string }[]
  placeholder?: string
}

type SettingsRowProps = {
  def: SettingDef
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  showKey?: boolean
  showHint?: boolean
  layout?: 'horizontal' | 'stacked'
  align?: 'start' | 'center'
  unit?: string
}

export function SettingRow({ def, value, onChange, disabled = false, showKey = true, showHint = true, layout = 'horizontal', align = 'start', unit }: SettingsRowProps) {
  const controlID = useId()
  const toggleOn = value === 'true' || value === '1' || value === 'on'
  return (
    <fieldset disabled={disabled} className={`grid min-w-0 items-start gap-2 ${layout === 'horizontal' ? `md:grid-cols-[280px_1fr] ${align === 'center' ? 'md:items-center' : ''}` : ''} ${disabled ? 'opacity-60' : ''}`}>
      <label htmlFor={controlID} className="text-sm text-ink-100">
        <div className="font-medium">{def.label}</div>
        {showHint && def.hint && <div className="mt-0.5 text-xs text-sand-500">{def.hint}</div>}
        {showKey && <div className="mt-0.5 font-mono text-[10px] text-gray-500">{def.key}</div>}
      </label>
      <div className={unit ? 'relative' : undefined}>
        {def.type === 'text' && (
          <input
            id={controlID}
            className="input-base"
            value={value}
            placeholder={def.placeholder}
            onChange={(event) => onChange(event.target.value)}
          />
        )}
        {def.type === 'number' && (
          <input
            id={controlID}
            type="number"
            className={`input-base ${unit ? 'pr-14' : ''}`}
            aria-label={unit ? `${def.label} (${unit})` : undefined}
            value={value}
            placeholder={def.placeholder}
            onChange={(event) => onChange(event.target.value)}
          />
        )}
        {unit && <span aria-hidden="true" className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-(--app-muted)">{unit}</span>}
        {def.type === 'textarea' && (
          <textarea
            id={controlID}
            rows={3}
            className="input-base font-mono text-xs"
            value={value}
            placeholder={def.placeholder}
            onChange={(event) => onChange(event.target.value)}
          />
        )}
        {def.type === 'select' && (
          <select id={controlID} className="input-base" value={value} onChange={(event) => onChange(event.target.value)}>
            <option value="">(未设置)</option>
            {def.options?.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        )}
        {def.type === 'toggle' && (
          <label className={`flex items-center gap-2 ${disabled ? 'cursor-not-allowed' : 'cursor-pointer'}`}>
            <input
              id={controlID}
              type="checkbox"
              className="h-4 w-4 accent-primary-400"
              checked={toggleOn}
              onChange={(event) => onChange(event.target.checked ? 'true' : 'false')}
            />
            <span className="text-sm text-ink-100">{disabled ? `${toggleOn ? '已开启' : '已关闭'} · 暂不生效` : toggleOn ? '已启用' : '已关闭'}</span>
          </label>
        )}
      </div>
    </fieldset>
  )
}
