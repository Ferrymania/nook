import { useId, useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react'
import { createPortal } from 'react-dom'
import { Copy, MoreHorizontal, Pencil, Trash2 } from 'lucide-react'

import type { FileEntry } from '../api/files'

export type FileEntryAction = 'rename' | 'transfer' | 'delete'

export function FileEntryMenu({ entry, disabled, onAction }: {
  entry: FileEntry
  disabled: boolean
  onAction: (entry: FileEntry, action: FileEntryAction) => void
}) {
  const trigger = useRef<HTMLButtonElement>(null)
  const menu = useRef<HTMLDivElement>(null)
  const id = useId()
  const [open, setOpen] = useState(false)
  const [position, setPosition] = useState({ top: 0, left: 0 })
  const close = () => { setOpen(false); trigger.current?.focus({ preventScroll: true }) }

  useLayoutEffect(() => {
    if (!open) return
    const update = () => {
      const rect = trigger.current?.getBoundingClientRect()
      const size = menu.current?.getBoundingClientRect()
      if (!rect || !size) return
      setPosition({
        left: Math.max(8, Math.min(rect.right - size.width, window.innerWidth - size.width - 8)),
        top: Math.max(8, Math.min(rect.bottom + 4, window.innerHeight - size.height - 8)),
      })
    }
    update()
    menu.current?.querySelector<HTMLElement>('[role="menuitem"]')?.focus({ preventScroll: true })
    window.addEventListener('resize', update)
    window.addEventListener('scroll', update, true)
    return () => {
      window.removeEventListener('resize', update)
      window.removeEventListener('scroll', update, true)
    }
  }, [open])

  const keyDown = (event: KeyboardEvent) => {
    if (event.key === 'Escape' || event.key === 'Tab') {
      event.preventDefault(); close(); return
    }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const options = [...(menu.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])]
    const current = options.indexOf(document.activeElement as HTMLElement)
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? options.length - 1
      : (current + (event.key === 'ArrowDown' ? 1 : -1) + options.length) % options.length
    options[next]?.focus()
  }

  const select = (action: FileEntryAction) => { close(); onAction(entry, action) }

  return <>
    <button ref={trigger} type="button" className="file-icon-button" disabled={disabled}
      aria-label={`操作：${entry.name}`} aria-haspopup="menu" aria-expanded={open} aria-controls={open ? id : undefined}
      onClick={() => setOpen(value => !value)}
      onKeyDown={event => { if (event.key === 'ArrowDown') { event.preventDefault(); setOpen(true) } }}>
      <MoreHorizontal size={19} />
    </button>
    {open && createPortal(<>
      <div className="fixed inset-0 z-80" onPointerDown={close} aria-hidden="true" />
      <div ref={menu} id={id} role="menu" aria-label={`${entry.name}的文件操作`} onKeyDown={keyDown}
        className="file-entry-menu" style={position}>
        <button type="button" role="menuitem" onClick={() => select('rename')}><Pencil size={16} />重命名</button>
        {!entry.is_dir && <button type="button" role="menuitem" onClick={() => select('transfer')}><Copy size={16} />复制 / 移动</button>}
        <div className="file-menu-divider" />
        <button type="button" role="menuitem" className="file-danger" onClick={() => select('delete')}><Trash2 size={16} />删除</button>
      </div>
    </>, document.body)}
  </>
}
