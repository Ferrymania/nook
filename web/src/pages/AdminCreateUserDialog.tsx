import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { createPortal } from 'react-dom'
import { Loader2, Plus, X } from 'lucide-react'

type AdminCreateUserDialogProps = {
  userLimitReached: boolean
  onCreate: (username: string, password: string) => Promise<void>
  onClose: () => void
}

export function AdminCreateUserDialog({ userLimitReached, onCreate, onClose }: AdminCreateUserDialogProps) {
  const dialogRef = useRef<HTMLDialogElement>(null)
  const usernameRef = useRef<HTMLInputElement>(null)
  const submitting = useRef(false)
  const titleID = useId()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const canCreate = !creating && !userLimitReached && Boolean(username.trim()) && password.length >= 6

  useEffect(() => {
    const dialog = dialogRef.current
    if (!dialog) return
    const previousFocus = document.activeElement as HTMLElement | null
    dialog.showModal()
    usernameRef.current?.focus()
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
    if (submitting.current || !canCreate) return
    submitting.current = true
    setCreating(true)
    setError('')
    try {
      await onCreate(username.trim(), password)
    } catch (err: unknown) {
      setError(userCreateErrorMessage(err))
      return
    } finally {
      submitting.current = false
      setCreating(false)
    }
    onClose()
  }

  return createPortal(
    <dialog
      ref={dialogRef}
      aria-labelledby={titleID}
      onCancel={(event) => { event.preventDefault(); close() }}
      className="m-auto max-h-[calc(100dvh-2rem)] w-[calc(100%-2rem)] max-w-md overflow-y-auto rounded-2xl border border-(--app-border) bg-(--app-panel-elevated) p-0 text-(--app-text) shadow-xl backdrop:bg-black/40"
    >
      <form name="admin-create-user" autoComplete="off" onSubmit={submit}>
        <header className="flex items-center justify-between gap-3 border-b border-(--app-border) px-5 py-3">
          <h2 id={titleID} className="font-display text-lg font-semibold">添加用户</h2>
          <button type="button" aria-label="关闭" className="btn-ghost h-11 w-11 p-0!" disabled={creating} onClick={close}>
            <X size={18} aria-hidden="true" />
          </button>
        </header>
        <div className="space-y-3 p-5">
          <fieldset disabled={creating || userLimitReached} className="grid min-w-0 gap-4">
            <label className="grid min-w-0 gap-1.5 text-sm">
              <span>用户名</span>
              <input ref={usernameRef} name="admin-new-username" autoComplete="off" spellCheck={false} required
                className="input-base w-full" value={username} onChange={(event) => setUsername(event.target.value)} />
            </label>
            <label className="grid min-w-0 gap-1.5 text-sm">
              <span>初始密码</span>
              <input name="admin-new-password" autoComplete="new-password" required minLength={6} type="password"
                className="input-base w-full" placeholder="至少 6 位" value={password} onChange={(event) => setPassword(event.target.value)} />
            </label>
          </fieldset>
          {(error || userLimitReached) && <p role="alert" className="text-sm text-(--app-danger-text)">
            {error || '已达到用户上限，调整上限后可继续添加。'}
          </p>}
        </div>
        <footer className="flex justify-end gap-2 border-t border-(--app-border) px-5 py-4">
          <button type="button" className="btn-outline min-h-11" disabled={creating} onClick={close}>取消</button>
          <button type="submit" className="btn-primary min-h-11" disabled={!canCreate}>
            {creating ? <Loader2 size={16} className="animate-spin" aria-hidden="true" /> : <Plus size={16} aria-hidden="true" />}
            {creating ? '添加中…' : '添加'}
          </button>
        </footer>
      </form>
    </dialog>, document.body,
  )
}

function userCreateErrorMessage(err: unknown): string {
  const data = (err as { response?: { data?: { error?: string; max_users?: number } } })?.response?.data
  if (data?.error === 'username already taken') return '用户名已存在'
  if (data?.error === 'user limit reached' && data.max_users != null) return `用户数量已达到上限：${data.max_users} 人`
  return data?.error || '添加用户失败'
}
