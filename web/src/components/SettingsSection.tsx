import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'

export function SettingsSection({ title, description, icon: Icon, actions, children }: {
  title: string
  description?: string
  icon: LucideIcon
  actions?: ReactNode
  children?: ReactNode
}) {
  return (
    <section className="settings-section" aria-label={title}>
      <header className="settings-section-header">
        <div className="min-w-0">
          <h2 className="settings-section-heading"><Icon size={17} aria-hidden="true" />{title}</h2>
          {description && <p className="settings-section-description">{description}</p>}
        </div>
        {actions}
      </header>
      {children}
    </section>
  )
}

export function SettingsSwitch({ label, checked, disabled = false, describedBy, onChange }: {
  label: string
  checked: boolean
  disabled?: boolean
  describedBy?: string
  onChange: (checked: boolean) => void
}) {
  return (
    <button type="button" role="switch" aria-label={label} aria-checked={checked}
      aria-describedby={describedBy} disabled={disabled} onClick={() => onChange(!checked)} className="settings-switch">
      <span />
    </button>
  )
}
