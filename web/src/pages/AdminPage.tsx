import { Users } from 'lucide-react'

import { AdminUsersPanel } from './AdminUsersPanel'

export function AdminPage() {
  return (
    <div className="space-y-6">
      <header className="flex items-center gap-3">
        <Users className="h-6 w-6 text-brand-500" />
        <div>
          <h1 className="font-display text-3xl font-bold text-ink-600">用户管理</h1>
        </div>
      </header>

      <AdminUsersPanel />
    </div>
  )
}
