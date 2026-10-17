import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { ChevronDown, Loader2, RefreshCw, Sparkles } from 'lucide-react'

export function LibraryScrapeMenu({ scraping = false, repairing, onScrape, onRepair, allLibraries = false }: {
  scraping?: boolean
  repairing: boolean
  onScrape?: () => void
  onRepair: () => void
  allLibraries?: boolean
}) {
  const [position, setPosition] = useState<{ left: number; top: number } | null>(null)
  const button = useRef<HTMLButtonElement>(null)
  const menu = useRef<HTMLDivElement>(null)
  const close = (restoreFocus = false) => {
    setPosition(null)
    if (restoreFocus) button.current?.focus()
  }
  const open = () => {
    const rect = button.current?.getBoundingClientRect()
    if (rect) setPosition({ left: Math.max(12, Math.min(rect.right - 260, window.innerWidth - 272)), top: rect.bottom + 6 })
  }
  useEffect(() => {
    if (!position) return
    menu.current?.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus()
    const outside = (event: MouseEvent) => {
      if (!menu.current?.contains(event.target as Node) && !button.current?.contains(event.target as Node)) setPosition(null)
    }
    const dismiss = () => setPosition(null)
    document.addEventListener('mousedown', outside)
    window.addEventListener('resize', dismiss)
    window.addEventListener('scroll', dismiss, true)
    return () => {
      document.removeEventListener('mousedown', outside)
      window.removeEventListener('resize', dismiss)
      window.removeEventListener('scroll', dismiss, true)
    }
  }, [position])
  return <>
    <button ref={button} type="button" className="library-toolbar-button" aria-haspopup="menu" aria-expanded={!!position}
      onClick={() => position ? close() : open()}
      onKeyDown={event => { if (event.key === 'ArrowDown') { event.preventDefault(); open() } }}>
      {scraping || repairing ? <Loader2 size={16} className="animate-spin" /> : <Sparkles size={16} />}
      刮削<ChevronDown size={14} />
    </button>
    {position && createPortal(<div ref={menu} role="menu" aria-label="媒体库刮削操作" className="library-toolbar-menu" style={position}
      onKeyDown={event => {
        if (event.key === 'Escape') { event.preventDefault(); close(true); return }
        if (event.key === 'Tab') { close(); return }
        const items = Array.from(menu.current?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)') ?? [])
        const index = items.indexOf(document.activeElement as HTMLButtonElement)
        if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
          event.preventDefault()
          items[(index + (event.key === 'ArrowDown' ? 1 : items.length - 1)) % items.length]?.focus()
        }
      }}>
      {onScrape && <button role="menuitem" type="button" className="library-toolbar-menu-item" disabled={scraping} onClick={() => { close(true); onScrape() }}>
        <Sparkles size={16} /><span>{scraping ? '正在提交…' : '整库刮削元数据'}<small>更新当前媒体库的影片资料</small></span>
      </button>}
      {onScrape && <div className="my-1 border-t border-[var(--app-border)]" />}
      <button role="menuitem" type="button" className="library-toolbar-menu-item text-red-600" disabled={repairing} onClick={() => { close(true); onRepair() }}>
        <RefreshCw size={16} /><span>{repairing ? '正在提交…' : allLibraries ? '全库修复并重新刮削…' : '修复并重新刮削…'}<small>{allLibraries ? '重新识别并修复所有媒体库' : '重新识别并修复当前媒体库'}</small></span>
      </button>
    </div>, document.body)}
  </>
}
