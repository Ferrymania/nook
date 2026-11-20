import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { createPortal } from 'react-dom'
import { Loader2, Save, X } from 'lucide-react'

type AdminUserLimitDialogProps = {
  maxUsers: number
  onSave: (value: number) => Promise<void>
  onClose: () => void
}

export function AdminUserLimitDialog({ maxUsers, onSave, onClose }: AdminUserLimitDialogProps) {
  const dialogRef = useRef<HTMLDialogElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const submitting = useRef(false)
  const titleID = useId()
  const helpID = useId()
  const [draft, setDraft] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const inputValue = draft ?? String(maxUsers)
  const value = Number(inputValue)
  const canSave = !saving && Number.isInteger(value) && value >= 1 && value <= 10000 && value !== maxUsers

  useEffect(() => {
    const dialog = dialogRef.current
    if (!dialog) return
    const previousFocus = document.activeElement as HTMLElement | null
    dialog.showModal()
    inputRef.current?.focus()
    return () => {
      dialog.close()
      previousFocus?.focus({ preventScroll: true })
    }
  }, [])

  const close = () => {
    if (!submitting.current) onClose()
  }

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (submitting.current || !canSave) return
    submitting.current = true
    setSaving(true)
    setError('')
    try {
      await onSave(value)
    } catch (err: unknown) {
      const response = (err as { response?: { data?: { error?: string } } })?.response
      setError(response?.data?.error || '保存用户上限失败')
      return
    } finally {
      submitting.current = false
      setSaving(false)
    }
    onClose()
  }

  return createPortal(
    <dialog ref={dialogRef} aria-labelledby={titleID}
      onCancel={(event) => { event.preventDefault(); close() }}
      className="m-auto max-h-[calc(100dvh-2rem)] w-[calc(100%-2rem)] max-w-md overflow-y-auto rounded-2xl border border-(--app-border) bg-(--app-panel-elevated) p-0 text-(--app-text) shadow-xl backdrop:bg-black/40">
      <form onSubmit={submit}>
        <header className="flex items-center justify-between gap-3 border-b border-(--app-border) px-5 py-3">
          <h2 id={titleID} className="font-display text-lg font-semibold">用户上限</h2>
          <button type="button" aria-label="关闭用户上限" className="btn-ghost h-11 w-11 p-0!" disabled={saving} onClick={close}>
            <X size={18} aria-hidden="true" />
          </button>
        </header>
        <div className="space-y-3 p-5">
          <label className="grid gap-1.5 text-sm">
            <span>用户数量上限</span>
            <input ref={inputRef} type="number" required min={1} max={10000} className="input-base w-full"
              aria-describedby={helpID} value={inputValue} disabled={saving} onChange={(event) => {
                setDraft(event.target.value)
                setError('')
              }} />
          </label>
          <p id={helpID} className="text-xs text-(--app-muted)">减少上限不会停用已有账号。</p>
          {error && <p role="alert" className="text-sm text-(--app-danger-text)">{error}</p>}
        </div>
        <footer className="flex justify-end gap-2 border-t border-(--app-border) px-5 py-4">
          <button type="button" className="btn-outline min-h-11" disabled={saving} onClick={close}>取消</button>
          <button type="submit" className="btn-primary min-h-11" disabled={!canSave}>
            {saving ? <Loader2 size={16} className="animate-spin" aria-hidden="true" /> : <Save size={16} aria-hidden="true" />}
            {saving ? '保存中…' : '保存上限'}
          </button>
        </footer>
      </form>
    </dialog>, document.body,
  )
}
