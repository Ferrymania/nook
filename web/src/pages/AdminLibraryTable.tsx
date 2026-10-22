import { useEffect, useRef, useState, type DragEvent, type MouseEvent, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { ArrowDown, ArrowUp, ChevronDown, ChevronUp, Folder, GripVertical, Image, MoreVertical, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-react'
import toast from 'react-hot-toast'

import { LocalDirBrowserDialog } from '../components/LocalDirBrowserDialog'
import type { Library, LibraryRoot } from '../types'
import type { RootDraft } from './adminLibraryPanelModel'
import { apiErrorMessage, displayLibraryRootName, displayLibraryRootPath, fallbackLibraryRoot, libraryTypeLabels, rootDraftKey } from './adminLibraryPanelModel'

type LibraryTableProps = {
  libs: Library[]
  disabled?: boolean
  editableRootDraft: (libraryID: string, root: LibraryRoot) => RootDraft
  onEditableRootChange: (libraryID: string, root: LibraryRoot, patch: Partial<RootDraft>) => void
  onCancelRootEdit: (libraryID: string, rootID: string) => void
  onSaveRoot: (libraryID: string, root: LibraryRoot) => Promise<boolean>
  onScanRoot: (libraryID: string, root: LibraryRoot) => Promise<unknown>
  onToggleRoot: (libraryID: string, root: LibraryRoot) => Promise<unknown>
  onRemoveRoot: (library: Library, root: LibraryRoot) => Promise<unknown>
  onScanLibrary: (library: Library) => Promise<unknown>
  onRemoveLibrary: (library: Library) => Promise<unknown>
  onAddLibraryRoot: (library: Library) => void
  onEditLibraryCover: (library: Library) => void
  onToggleCarousel: (library: Library) => Promise<unknown>
  onReorder: (orderedLibs: Library[]) => Promise<unknown>
}

export function AdminLibraryTable({ libs, disabled = false, ...actions }: LibraryTableProps) {
  const [expanded, setExpanded] = useState<string[]>([])
  const [editing, setEditing] = useState<string | null>(null)
  const [browsing, setBrowsing] = useState<{ library: Library; root: LibraryRoot } | null>(null)
  const [draggingID, setDraggingID] = useState<string | null>(null)
  const [dragOverID, setDragOverID] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const busyRef = useRef(false)
  const blocked = busy || disabled
  const participating = libs.filter(library => library.carousel_enabled).length

  const run = async (action: () => Promise<unknown>) => {
    if (busyRef.current || disabled) return
    busyRef.current = true
    setBusy(true)
    try { return await action() }
    catch (error) { toast.error(apiErrorMessage(error, '操作失败，请重试')) }
    finally { busyRef.current = false; setBusy(false) }
  }
  const reorder = (fromID: string, toID: string) => {
    const from = libs.findIndex(library => library.id === fromID), to = libs.findIndex(library => library.id === toID)
    if (from < 0 || to < 0 || from === to) return
    const ordered = [...libs]
    const [moved] = ordered.splice(from, 1)
    ordered.splice(to, 0, moved)
    void run(() => actions.onReorder(ordered))
  }
  const drop = (event: DragEvent, id: string) => {
    event.preventDefault()
    if (draggingID) reorder(draggingID, id)
    setDraggingID(null)
    setDragOverID(null)
  }
  const cancelEdit = (library: Library, root: LibraryRoot) => { actions.onCancelRootEdit(library.id, root.id); setEditing(null) }

  return <section aria-labelledby="admin-library-directory-title" aria-busy={busy}>
    <div className="nook-library-manager-section-title"><h3 id="admin-library-directory-title">媒体库目录</h3><span>拖动排序，也可在更多菜单中调整</span></div>
    <div className="nook-library-manager-carousel-note"><span>{participating ? `${participating} 个库参与首页轮播` : '首页轮播未开启'}</span><details><summary>轮播规则</summary><p id="home-carousel-help">仅展示已勾选媒体库的最近添加内容，多个库交替展示，最多 10 条；全部关闭时隐藏。</p></details></div>
    <div className="nook-library-manager-grid-heading" aria-hidden="true"><span /><span>媒体库</span><span>来源目录</span><span>首页轮播</span><span>操作</span><span /></div>
    {libs.length === 0 && <p className="nook-library-manager-empty">暂无媒体库，点击上方“新建媒体库”开始。</p>}
    <ul aria-label="媒体库列表">{libs.map((library, index) => {
      const roots = library.roots?.length ? library.roots : [fallbackLibraryRoot(library)]
      const open = expanded.includes(library.id)
      return <li key={library.id} className="nook-library-manager-item" data-dragging={draggingID === library.id} data-drag-over={dragOverID === library.id} onDragOver={event => { if (!draggingID || blocked) return; event.preventDefault(); event.dataTransfer.dropEffect = 'move'; setDragOverID(library.id) }} onDrop={event => drop(event, library.id)}>
        <div className="nook-library-manager-row">
          <button type="button" className="nook-library-manager-grip" draggable={!blocked} disabled={blocked} aria-label={`拖动排序 ${library.name}`} onDragStart={event => { event.dataTransfer.effectAllowed = 'move'; event.dataTransfer.setData('text/plain', library.id); setDraggingID(library.id) }} onDragEnd={() => { setDraggingID(null); setDragOverID(null) }}><GripVertical size={16} /></button>
          <div className="nook-library-manager-info"><h4>{library.name}</h4><p>{libraryTypeLabels[library.type] ?? library.type} · {(library.total ?? 0).toLocaleString('zh-CN')} 个条目</p></div>
          <button type="button" className="nook-library-manager-text-button nook-library-manager-directory-toggle" aria-expanded={open} aria-controls={`library-roots-${library.id}`} aria-label={`管理目录 ${library.name}`} onClick={() => setExpanded(previous => open ? previous.filter(id => id !== library.id) : [...previous, library.id])}>管理目录 <span className="nook-library-manager-count">{roots.length}</span>{open ? <ChevronUp size={13} /> : <ChevronDown size={13} />}</button>
          <label className="nook-library-manager-checkbox"><input type="checkbox" disabled={blocked} checked={Boolean(library.carousel_enabled)} aria-label={`${library.name}首页轮播`} onChange={() => void run(() => actions.onToggleCarousel(library))} />首页轮播</label>
          <button type="button" className="nook-library-manager-button nook-library-manager-add-directory" disabled={blocked} onClick={() => { setExpanded(previous => previous.includes(library.id) ? previous : [...previous, library.id]); actions.onAddLibraryRoot(library) }}><Plus size={15} />添加目录</button>
          <div className="nook-library-manager-menu"><ActionMenu label={`${library.name}更多操作`} disabled={blocked}>
            <MenuButton icon={<RefreshCw size={14} />} label="扫描媒体库" onClick={() => void run(() => actions.onScanLibrary(library))}>扫描媒体库</MenuButton>
            <MenuButton icon={<Image size={14} />} label="修改封面" onClick={() => actions.onEditLibraryCover(library)}>修改封面</MenuButton>
            <MenuButton icon={<ArrowUp size={14} />} label="上移" disabled={index === 0} onClick={() => reorder(library.id, libs[index - 1].id)}>上移</MenuButton>
            <MenuButton icon={<ArrowDown size={14} />} label="下移" disabled={index === libs.length - 1} onClick={() => reorder(library.id, libs[index + 1].id)}>下移</MenuButton>
            <MenuButton danger icon={<Trash2 size={14} />} label="删除媒体库" onClick={() => void run(() => actions.onRemoveLibrary(library))}>删除媒体库</MenuButton>
          </ActionMenu></div>
        </div>
        {open && <div className="nook-library-manager-roots" id={`library-roots-${library.id}`} aria-label={`${library.name}来源目录`}>{roots.map(root => {
          const key = rootDraftKey(library.id, root.id), draft = actions.editableRootDraft(library.id, root)
          const enabled = root.enabled
          return <div className="nook-library-manager-root" key={root.id || root.path}>{editing === key && root.id ? <div className="nook-library-manager-root-edit">
            <div className="nook-library-manager-path-fields"><label>目录名称<input className="input-base" value={draft.name ?? ''} disabled={blocked} onChange={event => actions.onEditableRootChange(library.id, root, { name: event.target.value })} /></label><label>目录路径<div className="nook-library-manager-path-input"><input className="input-base" value={draft.path} disabled={blocked} onChange={event => actions.onEditableRootChange(library.id, root, { path: event.target.value })} /><button type="button" className="nook-library-manager-button" disabled={blocked} onClick={() => setBrowsing({ library, root })}><Folder size={15} />浏览</button></div></label></div>
            <div className="nook-library-manager-form-footer"><button type="button" className="nook-library-manager-button" disabled={blocked} onClick={() => cancelEdit(library, root)}>取消</button><button type="button" className="nook-library-manager-button nook-library-manager-primary" disabled={blocked} onClick={() => void run(async () => { if (await actions.onSaveRoot(library.id, root)) setEditing(null) })}>保存目录</button></div>
          </div> : <>
            <div className="nook-library-manager-root-heading"><strong>{displayLibraryRootName(root.name, root.path)}</strong><label className="nook-library-manager-checkbox"><input type="checkbox" disabled={blocked || !root.id} checked={enabled} aria-label={`${library.name} ${displayLibraryRootName(root.name, root.path)}启用`} onChange={() => void run(() => actions.onToggleRoot(library.id, root))} />{enabled ? '已启用' : '已停用'}</label></div>
            <p className="nook-library-manager-root-path">{displayLibraryRootPath(root.path)}</p>
            <div className="nook-library-manager-root-footer"><span className="nook-library-manager-muted">{enabled ? '参与媒体库扫描' : '暂停扫描此目录'}</span><div className="nook-library-manager-root-actions">
              {root.id && <button type="button" className="nook-library-manager-text-button" disabled={blocked} onClick={() => { if (editing) { const [oldLibrary, oldRoot] = editing.split(':'); actions.onCancelRootEdit(oldLibrary, oldRoot) } setEditing(key) }}><Pencil size={14} />编辑</button>}
              <button type="button" className="nook-library-manager-text-button" disabled={blocked} onClick={() => void run(() => root.id ? actions.onScanRoot(library.id, root) : actions.onScanLibrary(library))}><RefreshCw size={14} />扫描</button>
              {root.id && <button type="button" className="nook-library-manager-text-button nook-library-manager-danger" disabled={blocked} onClick={() => void run(() => actions.onRemoveRoot(library, root))}><Trash2 size={14} />移除</button>}
            </div></div>
          </>}</div>
        })}</div>}
      </li>
    })}</ul>
    {browsing && <LocalDirBrowserDialog initialDir={actions.editableRootDraft(browsing.library.id, browsing.root).path} title="选择媒体库目录" onSelect={path => { actions.onEditableRootChange(browsing.library.id, browsing.root, { path }); setBrowsing(null) }} onClose={() => setBrowsing(null)} />}
  </section>
}
function ActionMenu({ label, children, disabled }: { label: string; children: ReactNode; disabled?: boolean }) {
  const [isOpen, setIsOpen] = useState(false)
  const [coords, setCoords] = useState<{ top?: number; bottom?: number; right: number } | null>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)

  const toggleMenu = (e: MouseEvent) => {
    e.stopPropagation()
    if (isOpen) {
      setIsOpen(false)
      return
    }
    if (triggerRef.current) {
      const rect = triggerRef.current.getBoundingClientRect()
      const spaceBelow = window.innerHeight - rect.bottom
      const estimatedHeight = 180
      const openUpward = spaceBelow < estimatedHeight && rect.top > estimatedHeight
      setCoords({
        top: openUpward ? undefined : rect.bottom + 4,
        bottom: openUpward ? window.innerHeight - rect.top + 4 : undefined,
        right: window.innerWidth - rect.right,
      })
      setIsOpen(true)
    }
  }

  useEffect(() => {
    if (!isOpen) return
    const handleClickOutside = (e: globalThis.MouseEvent) => {
      if (
        menuRef.current &&
        !menuRef.current.contains(e.target as Node) &&
        triggerRef.current &&
        !triggerRef.current.contains(e.target as Node)
      ) {
        setIsOpen(false)
      }
    }
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === 'Escape') { event.stopPropagation(); setIsOpen(false); triggerRef.current?.focus() } }
    window.addEventListener('keydown', handleKeyDown)
    const handleScrollOrResize = () => setIsOpen(false)
    window.addEventListener('mousedown', handleClickOutside)
    window.addEventListener('scroll', handleScrollOrResize, true)
    window.addEventListener('resize', handleScrollOrResize)
    return () => {
      window.removeEventListener('keydown', handleKeyDown)
      window.removeEventListener('mousedown', handleClickOutside)
      window.removeEventListener('scroll', handleScrollOrResize, true)
      window.removeEventListener('resize', handleScrollOrResize)
    }
  }, [isOpen])

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        onClick={toggleMenu}
        disabled={disabled}
        aria-label={label}
        aria-expanded={isOpen}
        className={`inline-flex h-8 w-8 cursor-pointer items-center justify-center rounded-lg border transition ${
          isOpen
            ? 'border-brand-500 bg-brand-500/10 text-brand-500'
            : 'border-(--app-border) bg-(--app-panel) text-(--app-muted) hover:text-(--app-text)'
        }`}
        title={label}
      >
        <MoreVertical size={16} />
      </button>

      {isOpen &&
        coords &&
        createPortal(
          <div
            ref={menuRef}
            style={{
              position: 'fixed',
              top: coords.top !== undefined ? `${coords.top}px` : undefined,
              bottom: coords.bottom !== undefined ? `${coords.bottom}px` : undefined,
              right: `${coords.right}px`,
              zIndex: 99999,
            }}
            className="min-w-32 rounded-xl border border-(--app-border) bg-(--app-panel) p-1.5 shadow-xl text-(--app-text)"
            onClick={() => {
              setIsOpen(false)
              triggerRef.current?.focus()
            }}
          >
            {children}
          </div>,
          document.body,
        )}
    </>
  )
}

function MenuButton({
  icon,
  label,
  danger,
  disabled,
  onClick,
  children,
}: {
  icon: ReactNode
  label: string
  danger?: boolean
  disabled?: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <button
      type="button"
      className={`flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-xs font-medium transition ${
        danger
          ? 'text-red-500 hover:bg-red-50'
          : 'text-(--app-text) hover:bg-(--app-panel-soft)'
      }`}
      title={label}
      disabled={disabled}
      onClick={onClick}
    >
      {icon}
      <span>{children}</span>
    </button>
  )
}
