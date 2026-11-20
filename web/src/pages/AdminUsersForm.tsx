import { useState } from 'react'
import { Plus, SlidersHorizontal } from 'lucide-react'

import { AdminCreateUserDialog } from './AdminCreateUserDialog'
import { AdminUserLimitDialog } from './AdminUserLimitDialog'

type AdminUsersFormProps = {
  usersCount: number
  maxUsers: number
  userLimitReached: boolean
  onSaveMaxUsers: (value: number) => Promise<void>
  onCreate: (username: string, password: string) => Promise<void>
}

export function AdminUsersForm({
  usersCount,
  maxUsers,
  userLimitReached,
  onSaveMaxUsers,
  onCreate,
}: AdminUsersFormProps) {
  const [showCreate, setShowCreate] = useState(false)
  const [showLimit, setShowLimit] = useState(false)

  return (
    <div className="space-y-3">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="font-display text-2xl font-semibold text-ink-600">用户管理</h1>
          <div className="flex items-center gap-2">
            <span className="text-sm tabular-nums text-(--app-muted)">{usersCount} / {maxUsers} 人</span>
            <button type="button" className="btn-outline min-h-8 px-2.5! py-1! text-xs!" aria-haspopup="dialog"
              onClick={() => setShowLimit(true)}>
              <SlidersHorizontal size={14} aria-hidden="true" />用户上限
            </button>
          </div>
        </div>
        <button type="button" className="btn-primary ml-auto h-11 shrink-0" aria-haspopup="dialog" disabled={userLimitReached}
          onClick={() => setShowCreate(true)}>
          <Plus size={16} aria-hidden="true" />添加用户
        </button>
      </header>

      {userLimitReached && <p role="status" className="text-sm text-amber-800">已达到用户上限，调整上限后可继续添加。</p>}

      {showLimit && <AdminUserLimitDialog maxUsers={maxUsers} onSave={onSaveMaxUsers} onClose={() => setShowLimit(false)} />}
      {showCreate && <AdminCreateUserDialog userLimitReached={userLimitReached} onCreate={onCreate} onClose={() => setShowCreate(false)} />}
    </div>
  )
}
