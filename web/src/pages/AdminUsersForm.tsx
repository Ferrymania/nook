import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { Ellipsis, Info, Loader2, Plus, Save, SlidersHorizontal } from 'lucide-react'

type AdminUsersFormProps = {
  usersCount: number
  maxUsers: number
  maxUsersDraft: string
  savingLimit: boolean
  creating: boolean
  username: string
  password: string
  userLimitReached: boolean
  onMaxUsersDraftChange: (value: string) => void
  onSaveMaxUsers: () => void
  onUsernameChange: (value: string) => void
  onPasswordChange: (value: string) => void
  onSubmit: (e: FormEvent) => void
}

export function AdminUsersForm({
  usersCount,
  maxUsers,
  maxUsersDraft,
  savingLimit,
  creating,
  username,
  password,
  userLimitReached,
  onMaxUsersDraftChange,
  onSaveMaxUsers,
  onUsernameChange,
  onPasswordChange,
  onSubmit,
}: AdminUsersFormProps) {
  const [showCreate, setShowCreate] = useState(false)
  const [showLimit, setShowLimit] = useState(false)
  const [showRoles, setShowRoles] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const actionsRef = useRef<HTMLDivElement>(null)
  const menuTrigger = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const createID = useId()
  const limitID = useId()
  const rolesID = useId()
  const menuID = useId()
  const draftValue = Number(maxUsersDraft)
  const limitChanged = maxUsersDraft.trim() !== '' && Number.isFinite(draftValue) && draftValue !== maxUsers

  useEffect(() => {
    if (!menuOpen) return
    menuRef.current?.querySelector<HTMLButtonElement>('button')?.focus()
    const closeOutside = (event: PointerEvent) => {
      if (!actionsRef.current?.contains(event.target as Node)) setMenuOpen(false)
    }
    document.addEventListener('pointerdown', closeOutside)
    return () => document.removeEventListener('pointerdown', closeOutside)
  }, [menuOpen])

  return (
    <div className="space-y-3">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="font-display text-2xl font-semibold text-ink-600">用户管理</h1>
          <span className="text-sm tabular-nums text-(--app-muted)">{usersCount} / {maxUsers} 人</span>
        </div>
        <div ref={actionsRef} className="relative ml-auto flex shrink-0 items-center gap-2"
          onBlur={(event) => { if (!event.currentTarget.contains(event.relatedTarget)) setMenuOpen(false) }}
          onKeyDown={(event) => {
            if (event.key === 'Escape' && menuOpen) {
              event.preventDefault()
              setMenuOpen(false)
              menuTrigger.current?.focus()
            }
          }}>
          <button ref={menuTrigger} type="button" className="btn-outline relative h-11 w-11 p-0" title="更多操作"
            aria-label={limitChanged ? '更多操作，有未保存的用户上限' : '更多操作'} aria-haspopup="menu" aria-expanded={menuOpen} aria-controls={menuID}
            onClick={() => setMenuOpen((open) => !open)}>
            <Ellipsis size={18} aria-hidden="true" />
            {limitChanged && <span aria-hidden="true" className="absolute right-1.5 top-1.5 h-1.5 w-1.5 rounded-full bg-amber-500" />}
          </button>
          <button type="button" className="btn-primary h-11" aria-expanded={showCreate} aria-controls={createID} disabled={creating || (userLimitReached && !showCreate)} onClick={() => { setMenuOpen(false); setShowCreate((open) => !open) }}>
            <Plus size={16} aria-hidden="true" />添加用户
          </button>
          {menuOpen && <div ref={menuRef} id={menuID} role="menu" aria-label="用户管理更多操作"
            className="absolute right-0 top-full z-20 mt-2 w-52 max-w-[calc(100vw-36px)] rounded-xl border border-(--app-border) bg-(--app-panel) p-1.5 shadow-lg"
            onKeyDown={(event) => {
              if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
              event.preventDefault()
              const items = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="menuitem"]'))
              const current = items.indexOf(document.activeElement as HTMLButtonElement)
              const next = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : (current + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length
              items[next]?.focus()
            }}>
            <button type="button" role="menuitem" className="flex min-h-11 w-full items-center gap-2.5 rounded-lg px-3 text-left text-sm hover:bg-(--app-hover) focus-visible:bg-(--app-hover) focus-visible:outline-none"
              aria-controls={limitID} onClick={() => { setShowLimit((open) => !open); setMenuOpen(false); menuTrigger.current?.focus() }}>
              <SlidersHorizontal size={16} aria-hidden="true" />
              <span className="flex-1">用户上限</span>
              {limitChanged && <span className="text-xs text-(--app-muted)">未保存</span>}
            </button>
            <button type="button" role="menuitem" className="flex min-h-11 w-full items-center gap-2.5 rounded-lg px-3 text-left text-sm hover:bg-(--app-hover) focus-visible:bg-(--app-hover) focus-visible:outline-none"
              aria-controls={rolesID} onClick={() => { setShowRoles((open) => !open); setMenuOpen(false); menuTrigger.current?.focus() }}>
              <Info size={16} aria-hidden="true" />角色权限说明
            </button>
          </div>}
        </div>
      </header>

      <div id={rolesID} hidden={!showRoles} className="space-y-1 rounded-xl bg-(--app-panel) p-4 text-sm text-(--app-subtle)">
        <p><strong className="font-medium text-(--app-text)">管理员：</strong>全部管理权限。</p>
        <p><strong className="font-medium text-(--app-text)">观看用户：</strong>浏览、播放和使用外部播放器；不能下载或操作文件。</p>
      </div>

      {userLimitReached && <p role="status" className="text-sm text-amber-800">已达到用户上限，调整上限后可继续添加。</p>}

      <div id={limitID} hidden={!showLimit} className="rounded-xl bg-(--app-panel) p-4">
        <div className="flex flex-wrap items-end gap-3">
          <label className="min-w-40 flex-1">
            <span className="mb-1 block text-sm font-medium">用户数量上限</span>
            <input
              type="number"
              min={1}
              max={10000}
              className="input-base"
              value={maxUsersDraft}
              onChange={(e) => onMaxUsersDraftChange(e.target.value)}
              disabled={savingLimit}
            />
          </label>
          <button
            type="button"
            className="btn-outline min-h-11"
            disabled={!limitChanged || savingLimit || !Number.isInteger(draftValue) || draftValue < 1 || draftValue > 10000}
            onClick={onSaveMaxUsers}
          >
            <Save size={16} />
            {savingLimit ? '保存中…' : '保存上限'}
          </button>
        </div>
        <p className="mt-2 text-xs text-(--app-muted)">减少上限不会停用已有账号。</p>
      </div>

      <form id={createID} hidden={!showCreate} name="admin-create-user" autoComplete="off" onSubmit={onSubmit} className="rounded-xl bg-(--app-panel) p-4">
        <fieldset disabled={creating || userLimitReached} className="grid gap-3 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
          <label className="min-w-0 space-y-1 text-sm">
            <span>用户名</span>
            <input
              id="admin-new-username"
              name="admin-new-username"
              autoComplete="off"
              spellCheck={false}
              required
              className="input-base"
              value={username}
              onChange={(e) => onUsernameChange(e.target.value)}
              disabled={userLimitReached}
            />
          </label>
          <label className="min-w-0 space-y-1 text-sm">
            <span>初始密码</span>
            <input
              id="admin-new-password"
              name="admin-new-password"
              autoComplete="new-password"
              required
              minLength={6}
              className="input-base"
              placeholder="至少 6 位"
              type="password"
              value={password}
              onChange={(e) => onPasswordChange(e.target.value)}
              disabled={userLimitReached}
            />
          </label>
          <button
            type="submit"
            className="btn-primary min-h-11"
            disabled={userLimitReached || creating}
          >
            {creating ? <Loader2 size={16} className="animate-spin" /> : <Plus size={16} />}
            {creating ? '添加中…' : '添加'}
          </button>
        </fieldset>
      </form>
    </div>
  )
}
