import { useId, useState, type FormEvent } from 'react'
import { Info, Loader2, Plus, Save } from 'lucide-react'

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
  const createID = useId()
  const limitID = useId()
  const draftValue = Number(maxUsersDraft)
  const limitChanged = maxUsersDraft.trim() !== '' && Number.isFinite(draftValue) && draftValue !== maxUsers

  return (
    <div className="space-y-3">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="font-display text-2xl font-semibold text-ink-600">用户管理</h1>
          <span className="text-sm tabular-nums text-(--app-muted)">{usersCount} / {maxUsers} 人</span>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <details className="relative">
            <summary className="btn-ghost flex min-h-11 min-w-11 cursor-pointer list-none items-center justify-center rounded-lg [&::-webkit-details-marker]:hidden" aria-label="角色权限说明">
              <Info size={17} aria-hidden="true" />
            </summary>
            <div className="absolute left-0 z-20 w-64 max-w-[calc(100vw-40px)] space-y-2 rounded-xl border border-(--app-border) bg-(--app-panel) p-4 text-sm shadow-lg sm:left-auto sm:right-0">
              <p><strong className="font-medium">管理员：</strong>全部管理权限。</p>
              <p><strong className="font-medium">观看用户：</strong>浏览、播放和使用外部播放器；不能下载或操作文件。</p>
            </div>
          </details>
          <button type="button" className="btn-secondary min-h-11" aria-expanded={showLimit} aria-controls={limitID} onClick={() => setShowLimit((open) => !open)}>
            用户上限{limitChanged ? ' · 未保存' : ''}
          </button>
          <button type="button" className="btn-primary min-h-11" aria-expanded={showCreate} aria-controls={createID} disabled={creating || (userLimitReached && !showCreate)} onClick={() => setShowCreate((open) => !open)}>
            <Plus size={16} aria-hidden="true" />添加用户
          </button>
        </div>
      </header>

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
            className="btn-secondary min-h-11"
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
