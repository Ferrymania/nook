/* eslint-disable react-refresh/only-export-components */
import { useCallback, useEffect, useRef, useState } from 'react'
import { isAxiosError } from 'axios'
import toast from 'react-hot-toast'
import {
  Ban,
  Cloud,
  Download,
  Folder,
  History,
  Loader2,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  Settings,
  Trash2,
  Upload,
} from 'lucide-react'

import { strmAPI } from '../api/strm'
import { confirmAction } from '../components/confirmAction'
import type { StrmAccount, StrmSyncPath, StrmSyncRecord, StrmSyncRecordPage } from '../types/strm'
import { STRM_PROVIDER_LABELS } from '../types/strm'
import { StrmAccountDialog, StrmSettingsDialog, StrmSyncPathDialog } from './StrmDialogs'

type StrmDialogKey = 'account' | 'settings' | 'path' | null
type AccountTestState = { pending: boolean; error?: string }

const STATUS_META: Record<string, { label: string; cls: string }> = {
  '': { label: '尚未同步', cls: 'bg-gray-100 text-gray-500' },
  idle: { label: '尚未同步', cls: 'bg-gray-100 text-gray-500' },
  running: { label: '同步中', cls: 'bg-brand-100 text-brand-600' },
  done: { label: '上次同步成功', cls: 'bg-emerald-100 text-emerald-600' },
  failed: { label: '上次同步失败', cls: 'bg-rose-100 text-rose-600' },
  // 兼容旧版保存的目录状态；新同步结果使用 done / failed。
  ok: { label: '上次同步成功', cls: 'bg-emerald-100 text-emerald-600' },
  error: { label: '上次同步失败', cls: 'bg-rose-100 text-rose-600' },
  canceled: { label: '已取消', cls: 'bg-amber-100 text-amber-600' },
}

const RECORD_STATUS_META: Record<string, { label: string; cls: string }> = {
  pending: { label: '排队中', cls: 'bg-gray-100 text-gray-500' },
  running: { label: '进行中', cls: 'bg-brand-100 text-brand-600' },
  done: { label: '完成', cls: 'bg-emerald-100 text-emerald-600' },
  failed: { label: '失败', cls: 'bg-rose-100 text-rose-600' },
  canceled: { label: '已取消', cls: 'bg-amber-100 text-amber-600' },
}

const iconButtonCls =
  'inline-flex items-center gap-1 rounded-lg border border-gray-200 px-2 py-1 text-xs font-semibold text-ink-100 transition hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50'

export function StrmManagePage() {
  const [accounts, setAccounts] = useState<StrmAccount[]>([])
  const [paths, setPaths] = useState<StrmSyncPath[]>([])
  const [recordSnapshot, setRecordSnapshot] = useState<StrmSyncRecordPage>({ items: [], total: 0, page: 1, page_size: 10 })
  const [recordsLoading, setRecordsLoading] = useState(false)
  const [recordsError, setRecordsError] = useState('')
  const recordPageRef = useRef(1)
  const recordRequestRef = useRef(0)
  const [loading, setLoading] = useState(true)
  const [dialog, setDialog] = useState<StrmDialogKey>(null)
  const [editingPath, setEditingPath] = useState<StrmSyncPath | null>(null)
  const [editingAccount, setEditingAccount] = useState<StrmAccount | null>(null)
  const [actingPath, setActingPath] = useState<string | null>(null)
  const [accountTests, setAccountTests] = useState<Record<string, AccountTestState>>({})
  const testingAccountIds = useRef(new Set<string>())

  const loadRecords = useCallback(async (page = recordPageRef.current) => {
    const request = ++recordRequestRef.current
    recordPageRef.current = page
    setRecordsLoading(true)
    setRecordsError('')
    try {
      const snapshot = await strmAPI.listRecordsPage(page)
      if (request !== recordRequestRef.current) return
      recordPageRef.current = snapshot.page
      setRecordSnapshot(snapshot)
    } catch (err) {
      if (request === recordRequestRef.current) setRecordsError(apiErrorMessage(err))
    } finally {
      if (request === recordRequestRef.current) setRecordsLoading(false)
    }
  }, [])

  useEffect(() => () => { recordRequestRef.current += 1 }, [])

  const refresh = useCallback(async () => {
    try {
      const [accts, pths] = await Promise.all([
        strmAPI.listAccounts().then((rows) => rows.filter((a) => a.provider !== 'emby_remote')),
        strmAPI.listPaths(),
        loadRecords(),
      ])
      setAccounts(accts)
      setPaths(pths)
    } catch {
      /* 保留旧数据 */
    } finally {
      setLoading(false)
    }
  }, [loadRecords])

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [refresh])

  // 有进行中的同步时轮询刷新状态
  useEffect(() => {
    if (!paths.some((p) => p.last_sync_status === 'running')) return
    const timer = setInterval(() => {
      if (document.hidden) return
      refresh().catch(() => undefined)
    }, 4000)
    return () => clearInterval(timer)
  }, [paths, refresh])

  const startSync = async (path: StrmSyncPath, mode: 'incremental' | 'full' = 'incremental') => {
    setActingPath(path.id)
    try {
      await strmAPI.startSync(path.id, mode)
      toast.success(`已开始${mode === 'full' ? '全量' : '增量'}同步「${path.name}」`)
      await refresh()
    } catch (err) {
      toast.error(apiErrorMessage(err))
    } finally {
      setActingPath(null)
    }
  }

  const cancelSync = async (path: StrmSyncPath) => {
    try {
      await strmAPI.cancelSync(path.id)
      toast.success('已请求取消同步')
      await refresh()
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }

  const deletePath = async (path: StrmSyncPath) => {
    const ok = await confirmAction({ message: `确定删除同步目录「${path.name}」？本地已生成的 strm 文件不会删除。`, confirmText: '删除' })
    if (!ok) return
    try {
      await strmAPI.deletePath(path.id)
      toast.success('已删除同步目录')
      await refresh()
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }

  const deleteAccount = async (account: StrmAccount) => {
    const ok = await confirmAction({ message: `确定删除网盘账号「${account.name}」？`, confirmText: '删除' })
    if (!ok) return
    try {
      await strmAPI.deleteAccount(account.id)
      toast.success('已删除网盘账号')
      await refresh()
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }

  const testAccount = async (account: StrmAccount) => {
    if (testingAccountIds.current.has(account.id)) return
    testingAccountIds.current.add(account.id)
    setAccountTests((tests) => ({ ...tests, [account.id]: { pending: true } }))
    try {
      const updated = await strmAPI.testAccount(account.id)
      setAccounts((list) => list.map((a) => (a.id === updated.id ? updated : a)))
      if (updated.last_test_ok) {
        toast.success(`「${account.name}」连接正常`)
      } else {
        toast.error(`「${account.name}」连接失败：${updated.last_test_result}`)
      }
    } catch (err) {
      const message = accountTestErrorMessage(err)
      setAccountTests((tests) => ({ ...tests, [account.id]: { pending: true, error: message } }))
      toast.error(`「${account.name}」${message}`)
    } finally {
      testingAccountIds.current.delete(account.id)
      setAccountTests((tests) => ({ ...tests, [account.id]: { ...tests[account.id], pending: false } }))
    }
  }

  const openAddPathDialog = async () => {
    try {
      const settings = await strmAPI.getSettings()
      const baseUrl = settings?.['strm.base_url']?.trim()
      if (!baseUrl) {
        toast.error('未填写strm地址')
        setDialog('settings')
        return
      }
      setEditingPath(null)
      setDialog('path')
    } catch (err) {
      toast.error(apiErrorMessage(err))
    }
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-sand-300/40 text-ink-100">
            <Play size={20} />
          </div>
          <div>
            <h1 className="font-display text-3xl font-bold text-ink-600">STRM 管理</h1>
            <p className="text-sm text-ink-50">
              网盘目录同步生成 .strm 文件（Emby / Jellyfin 直接刮削播放），元数据经下载 / 上传队列双向同步
            </p>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button type="button" onClick={() => { setEditingAccount(null); setDialog('account') }} className="neon-button">
            <Plus size={16} />
            添加网盘账号
          </button>
          <button type="button" onClick={() => setDialog('settings')} className="neon-button">
            <Settings size={16} />
            STRM 设置
          </button>
          <button type="button" onClick={openAddPathDialog} className="neon-button">
            <Plus size={16} />
            添加同步目录
          </button>
        </div>
      </header>

      {loading ? (
        <div className="flex justify-center py-12 text-ink-50">
          <Loader2 className="animate-spin" />
        </div>
      ) : (
        <>
          <AccountSection
            accounts={accounts}
            accountTests={accountTests}
            onAdd={() => { setEditingAccount(null); setDialog('account') }}
            onEdit={(account) => { setEditingAccount(account); setDialog('account') }}
            onDelete={deleteAccount}
            onTest={testAccount}
          />

          <SyncPathSection
            paths={paths}
            actingPath={actingPath}
            onAdd={openAddPathDialog}
            onEdit={(path) => { setEditingPath(path); setDialog('path') }}
            onDelete={deletePath}
            onStart={startSync}
            onCancel={cancelSync}
          />

          <RecordSection
            snapshot={recordSnapshot}
            paths={paths}
            loading={recordsLoading}
            error={recordsError}
            onPageChange={loadRecords}
            onDeleted={() => loadRecords()}
          />
        </>
      )}

      {dialog === 'account' && (
        <StrmAccountDialog
          key={editingAccount?.id ?? 'new'}
          existing={editingAccount}
          onClose={() => setDialog(null)}
          onSaved={() => { setDialog(null); refresh().catch(() => undefined) }}
        />
      )}
      {dialog === 'settings' && <StrmSettingsDialog onClose={() => setDialog(null)} />}
      {dialog === 'path' && (
        <StrmSyncPathDialog
          accounts={accounts}
          existing={editingPath}
          onClose={() => setDialog(null)}
          onSaved={() => { setDialog(null); refresh().catch(() => undefined) }}
          onOpenSettings={() => setDialog('settings')}
        />
      )}
    </div>
  )
}

// ─── 网盘账号 ────────────────────────────────────────────────────────────────

function AccountSection({
  accounts,
  accountTests,
  onAdd,
  onEdit,
  onDelete,
  onTest,
}: {
  accounts: StrmAccount[]
  accountTests: Record<string, AccountTestState>
  onAdd: () => void
  onEdit: (account: StrmAccount) => void
  onDelete: (account: StrmAccount) => void
  onTest: (account: StrmAccount) => void
}) {
  return (
    <section className="glass-panel space-y-3 p-5">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Cloud size={18} className="text-brand-500" />
          <h2 className="font-display text-lg font-semibold text-ink-600">网盘账号</h2>
        </div>
        <button type="button" onClick={onAdd} className="inline-flex items-center gap-1 text-sm font-semibold text-brand-500 hover:text-brand-600">
          <Plus size={14} />新建
        </button>
      </div>
      {accounts.length === 0 ? (
        <p className="rounded-xl bg-gray-50 px-4 py-6 text-center text-sm text-sand-500">
          还没有网盘账号，点击「添加网盘账号」创建（115 网盘支持二维码登录）
        </p>
      ) : (
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {accounts.map((account) => {
            const testState = accountTests[account.id]
            const testing = testState?.pending ?? false
            const testError = testState?.error
            return (
              <div key={account.id} className="rounded-2xl border border-gray-100 bg-white p-4 shadow-xs">
                <div className="flex items-start justify-between gap-2">
                  <div className="flex items-center gap-2.5">
                    <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-brand-50 text-brand-500">
                      <Cloud size={18} />
                    </div>
                    <div>
                      <p className="font-semibold text-ink-600">{account.name}</p>
                      <p className="text-xs text-sand-500">{account.provider_label}</p>
                    </div>
                  </div>
                  <span
                    className={
                      'rounded-full px-2 py-0.5 text-[11px] font-semibold ' +
                      (account.enabled ? 'bg-emerald-100 text-emerald-600' : 'bg-gray-200 text-gray-500')
                    }
                  >
                    {account.enabled ? '已启用' : '已停用'}
                  </span>
                </div>
                <div className="mt-3 space-y-1 text-xs text-ink-50">
                  <p>
                    凭据：
                    {account.has_credential ? (
                      <span className="text-emerald-600">已配置</span>
                    ) : (
                      <span className="text-gray-400">未配置</span>
                    )}
                  </p>
                  <div role="status" aria-live="polite" aria-atomic="true" className="wrap-anywhere">
                    {testing ? (
                      <p>正在测试连接，请稍候…</p>
                    ) : testError ? (
                      <p className="text-rose-500">测试未完成：{testError}</p>
                    ) : account.last_test_at ? (
                      <p className={account.last_test_ok ? 'text-emerald-600' : 'text-rose-500'}>
                        最近测试：{account.last_test_ok ? '正常' : account.last_test_result}
                      </p>
                    ) : null}
                  </div>
                </div>
                <div className="mt-3 flex flex-wrap items-center gap-1.5">
                  <button type="button" disabled={testing} aria-busy={testing} onClick={() => onTest(account)} className={iconButtonCls}>
                    <RefreshCw size={14} className={testing ? 'animate-spin' : undefined} />
                    {testing ? '测试中…' : '测试'}
                  </button>
                  <button type="button" disabled={testing} onClick={() => onEdit(account)} className={iconButtonCls}>
                    <Pencil size={14} />
                    编辑
                  </button>
                  <button type="button" disabled={testing} onClick={() => onDelete(account)} className={`${iconButtonCls} text-rose-500`}>
                    <Trash2 size={14} />
                    删除
                  </button>
                </div>
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}

// ─── STRM 同步目录（参考 QMediaSync 的 STRM 同步目录页） ─────────────────────

function SyncPathSection({
  paths,
  actingPath,
  onAdd,
  onEdit,
  onDelete,
  onStart,
  onCancel,
}: {
  paths: StrmSyncPath[]
  actingPath: string | null
  onAdd: () => void
  onEdit: (path: StrmSyncPath) => void
  onDelete: (path: StrmSyncPath) => void
  onStart: (path: StrmSyncPath, mode?: 'incremental' | 'full') => void
  onCancel: (path: StrmSyncPath) => void
}) {
  return (
    <section className="glass-panel space-y-3 p-5">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Folder size={18} className="text-brand-500" />
          <h2 className="font-display text-lg font-semibold text-ink-600">STRM 同步目录</h2>
          <span className="rounded-full bg-gray-100 px-2 py-0.5 text-[11px] text-sand-500">{paths.length}</span>
        </div>
        <button type="button" onClick={onAdd} className="inline-flex items-center gap-1 text-sm font-semibold text-brand-500 hover:text-brand-600">
          <Plus size={14} />新建
        </button>
      </div>
      {paths.length === 0 ? (
        <p className="rounded-xl bg-gray-50 px-4 py-6 text-center text-sm text-sand-500">
          还没有同步目录。添加后系统会把网盘 / 本地目录里的视频生成 .strm 文件到本地输出目录
        </p>
      ) : (
        <div className="space-y-2.5">
          {paths.map((path) => {
            const status = STATUS_META[path.last_sync_status] ?? { label: '状态未知', cls: STATUS_META.idle.cls }
            const running = path.last_sync_status === 'running'
            return (
              <div key={path.id} className="rounded-2xl border border-gray-100 bg-white p-4 shadow-xs">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="flex min-w-0 items-center gap-2.5">
                    <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-brand-50 text-brand-500">
                      <Folder size={18} />
                    </div>
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <p className="font-semibold text-ink-600">
                          {path.name}
                          {!path.enabled && <span className="ml-2 text-xs text-gray-400">（已停用）</span>}
                        </p>
                        <span className={'rounded-full px-2 py-0.5 text-[11px] font-semibold ' + status.cls}>
                          {running && <Loader2 size={10} className="mr-0.5 inline animate-spin" />}
                          {status.label}
                        </span>
                      </div>
                      <p className="mt-0.5 text-xs text-sand-500">
                        {STRM_PROVIDER_LABELS[path.provider]}
                        {path.account_name ? ` · ${path.account_name}` : ''}
                        {path.enable_cron && path.cron ? ` · 定时 ${path.cron}` : ''}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-1.5">
                    {running ? (
                      <button type="button" onClick={() => onCancel(path)} className={`${iconButtonCls} text-amber-600`}>
                        <Ban size={14} />
                        取消
                      </button>
                    ) : (
                      <>
                        <button
                          type="button"
                          disabled={actingPath === path.id || !path.enabled}
                          onClick={() => onStart(path, 'incremental')}
                          className={`${iconButtonCls} text-brand-600 font-medium disabled:opacity-40`}
                          title="增量同步：基于目录缓存快速同步新增与更新文件"
                        >
                          {actingPath === path.id ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />}
                          增量同步
                        </button>
                        <button
                          type="button"
                          disabled={actingPath === path.id || !path.enabled}
                          onClick={() => onStart(path, 'full')}
                          className={`${iconButtonCls} text-sand-600 disabled:opacity-40`}
                          title="全量同步：重置目录缓存并全量比对所有文件"
                        >
                          <RefreshCw size={14} />
                          全量同步
                        </button>
                      </>
                    )}
                    <button type="button" onClick={() => onEdit(path)} className={`${iconButtonCls}`}>
                      <Pencil size={14} />
                      编辑
                    </button>
                    <button type="button" onClick={() => onDelete(path)} className={`${iconButtonCls} text-rose-500`}>
                      <Trash2 size={14} />
                      删除
                    </button>
                  </div>
                </div>
                <div className="mt-2.5 flex flex-wrap items-center gap-x-2 gap-y-1 font-mono text-xs text-ink-50">
                  <span
                    className="max-w-[45%] truncate"
                    title={path.remote_display_path || path.remote_path || '（根目录）'}
                  >
                    {path.remote_display_path || path.remote_path || '（根目录）'}
                  </span>
                  <span className="text-sand-500">→</span>
                  <span className="max-w-[45%] truncate">{path.local_path}</span>
                </div>
                {path.last_sync_at && (
                  <p className="mt-1.5 text-xs text-sand-500" title="该次同步的开始时间">
                    最近同步：{formatTime(path.last_sync_at)}
                  </p>
                )}
                {path.last_sync_message && (
                  <p className="mt-1.5 text-xs text-sand-500">{path.last_sync_message}</p>
                )}
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}

// ─── 同步记录 ────────────────────────────────────────────────────────────────

function RecordSection({ snapshot, paths, loading, error, onPageChange, onDeleted }: {
  snapshot: StrmSyncRecordPage
  paths: StrmSyncPath[]
  loading: boolean
  error: string
  onPageChange: (page: number) => Promise<void>
  onDeleted: () => Promise<void>
}) {
  const records = snapshot.items
  const totalPages = Math.max(1, Math.ceil(snapshot.total / snapshot.page_size))
  const [deletingId, setDeletingId] = useState<string | null>(null)
  const [clearing, setClearing] = useState(false)
  const busy = loading || deletingId !== null || clearing

  const deleteRecord = async (record: StrmSyncRecord) => {
    const ok = await confirmAction({ message: '确定删除这条同步记录？', confirmText: '删除' })
    if (!ok) return
    setDeletingId(record.id)
    try {
      await strmAPI.deleteRecord(record.id)
      toast.success('已删除同步记录')
      await onDeleted()
    } catch (err) {
      toast.error(apiErrorMessage(err))
    } finally {
      setDeletingId(null)
    }
  }

  const clearRecords = async () => {
    const ok = await confirmAction({ message: '确定清空全部同步记录？此操作不可恢复。', confirmText: '清空' })
    if (!ok) return
    setClearing(true)
    try {
      const res = await strmAPI.clearRecords()
      toast.success(`已清空 ${res.deleted} 条同步记录`)
      await onDeleted()
    } catch (err) {
      toast.error(apiErrorMessage(err))
    } finally {
      setClearing(false)
    }
  }

  return (
    <section className="glass-panel space-y-3 p-5">
      <div className="flex items-center gap-2">
        <History size={18} className="text-brand-500" />
        <h2 className="font-display text-lg font-semibold text-ink-600">同步记录</h2>
        <span className="rounded-full bg-gray-100 px-2 py-0.5 text-[11px] text-sand-500">{snapshot.total}</span>
        {snapshot.total > 0 && (
          <button type="button" onClick={clearRecords} disabled={busy} className={iconButtonCls + ' ml-auto'}>
            <Trash2 size={14} />
            清空
          </button>
        )}
      </div>
      {error && (
        <p role="alert" className="flex items-center gap-3 text-sm text-rose-500">
          同步记录加载失败：{error}
          <button type="button" disabled={busy} onClick={onDeleted} className="text-ink-100 underline disabled:opacity-40">重试</button>
        </p>
      )}
      {records.length === 0 && !error ? (
        <p className="rounded-xl bg-gray-50 px-4 py-6 text-center text-sm text-sand-500">还没有同步记录</p>
      ) : records.length > 0 && (
        <div className="table-scroll" aria-busy={loading}>
          <table className="min-w-[1120px] w-full text-left text-sm">
            <thead className="border-b border-gray-200 text-xs uppercase tracking-wider text-sand-500">
              <tr>
                <th className="px-3 py-2">时间</th>
                <th className="px-3 py-2">同步目录</th>
                <th className="px-3 py-2">类型</th>
                <th className="px-3 py-2">状态</th>
                <th className="px-3 py-2 text-right">扫描文件</th>
                <th className="px-3 py-2 text-right">新增/更新</th>
                <th className="px-3 py-2 text-right">跳过</th>
                <th className="px-3 py-2 text-right">下载元数据</th>
                <th className="px-3 py-2 text-right">上传元数据</th>
                <th className="px-3 py-2 text-right">清理</th>
                <th className="px-3 py-2">说明</th>
                <th className="px-3 py-2"></th>
              </tr>
            </thead>
            <tbody>
              {records.map((record) => {
                const meta = RECORD_STATUS_META[record.status] ?? RECORD_STATUS_META.pending
                const isFull = record.sync_type === 'full'
                const syncPath = paths.find((path) => path.id === record.sync_path_id)
                const directory = syncPath?.name.trim() || syncPath?.remote_display_path?.trim() || syncPath?.remote_path || '—'
                return (
                  <tr key={record.id} className="border-t border-gray-100">
                    <td className="whitespace-nowrap px-3 py-2 text-xs text-ink-50">
                      {formatTime(record.started_at ?? record.created_at)}
                    </td>
                    <td className="px-3 py-2 text-ink-100">
                      <span className="block max-w-40 truncate" title={directory}>{directory}</span>
                    </td>
                    <td className="px-3 py-2">
                      <span className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${isFull ? 'bg-amber-50 text-amber-600 border border-amber-200' : 'bg-brand-50 text-brand-600 border border-brand-200'}`}>
                        {isFull ? '全量' : '增量'}
                      </span>
                    </td>
                    <td className="px-3 py-2">
                      <span className={'rounded-full px-2 py-0.5 text-[11px] font-semibold ' + meta.cls}>
                        {meta.label}
                      </span>
                    </td>
                    <td className="px-3 py-2 text-right">{record.total}</td>
                    <td className="px-3 py-2 text-right text-brand-500">{record.new_strm}</td>
                    <td className="px-3 py-2 text-right text-gray-500">{record.skipped}</td>
                    <td className="px-3 py-2 text-right">{record.new_meta}</td>
                    <td className="px-3 py-2 text-right">{record.uploaded ?? 0}</td>
                    <td className="px-3 py-2 text-right">{record.pruned}</td>
                    <td className="max-w-[260px] truncate px-3 py-2 text-xs text-sand-500">{record.message}</td>
                    <td className="px-3 py-2 text-right">
                      <button
                        type="button"
                        onClick={() => deleteRecord(record)}
                        disabled={busy}
                        title="删除记录"
                        className="rounded-md p-1 text-sand-400 transition hover:bg-rose-50 hover:text-rose-500 disabled:opacity-40"
                      >
                        <Trash2 size={15} />
                      </button>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
      {snapshot.total > 0 && (
        <nav aria-label="同步记录分页" className="flex flex-wrap items-center justify-between gap-3 border-t border-gray-200 pt-3">
          <span className="text-xs tabular-nums text-sand-500">第 {snapshot.page} / {totalPages} 页</span>
          <div className="flex items-center gap-2">
            <button type="button" disabled={busy || snapshot.page <= 1} onClick={() => onPageChange(snapshot.page - 1)} className="min-h-11 rounded-lg border border-gray-200 px-3 text-sm text-ink-100 transition hover:bg-gray-50 disabled:opacity-40">上一页</button>
            <button type="button" disabled={busy || snapshot.page >= totalPages} onClick={() => onPageChange(snapshot.page + 1)} className="min-h-11 rounded-lg border border-gray-200 px-3 text-sm text-ink-100 transition hover:bg-gray-50 disabled:opacity-40">下一页</button>
          </div>
        </nav>
      )}
    </section>
  )
}

export function formatTime(value?: string | null): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getMonth() + 1}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

export function apiErrorMessage(err: unknown): string {
  const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error
  return msg ?? '操作失败'
}

function accountTestErrorMessage(err: unknown): string {
  if (isAxiosError<{ error?: string }>(err)) {
    if (err.response?.data?.error) return err.response.data.error
    if (err.code === 'ECONNABORTED' || err.code === 'ETIMEDOUT') {
      return '等待测试结果超时，请检查网盘服务地址、端口和容器网络后重试'
    }
    if (!err.response) return '无法连接 Nook 服务，请检查网络后重试'
  }
  return '连接测试未完成，请稍后重试'
}

export function formatBytes(size: number): string {
  if (!size) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = size
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value.toFixed(value >= 100 ? 0 : 1)} ${units[unit]}`
}

// 队列概览小组件（下载队列 / 上传队列页顶部统计）
export function QueueStatPill({ icon: Icon, label, value }: { icon: typeof Download; label: string; value: number }) {
  return (
    <div className="flex items-center gap-2 rounded-xl border border-gray-100 bg-white px-3 py-2 shadow-xs">
      <Icon size={16} className="text-brand-500" />
      <span className="text-xs text-sand-500">{label}</span>
      <span className="font-display text-lg font-bold text-ink-600">{value}</span>
    </div>
  )
}

// 队列状态徽章
export function taskStatusMeta(status: string): { label: string; cls: string } {
  switch (status) {
    case 'pending':
      return { label: '排队中', cls: 'bg-gray-100 text-gray-500' }
    case 'running':
      return { label: '进行中', cls: 'bg-brand-100 text-brand-600' }
    case 'done':
      return { label: '已完成', cls: 'bg-emerald-100 text-emerald-600' }
    case 'failed':
      return { label: '失败', cls: 'bg-rose-100 text-rose-600' }
    case 'canceled':
      return { label: '已取消', cls: 'bg-amber-100 text-amber-600' }
    default:
      return { label: status, cls: 'bg-gray-100 text-gray-500' }
  }
}

// 供下载/上传队列页复用
export { Upload as UploadIcon }
