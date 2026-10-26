import { useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Link, useLocation } from 'react-router-dom'
import { Cast, ChevronDown, Clock, Heart, ListMusic, LogOut, Settings, Tv } from 'lucide-react'

import { LayoutThemeToggle } from './LayoutThemeToggle'
import { TemporaryPasswordDialog } from './TemporaryPasswordDialog'
import type { ThemeMode } from './useThemeMode'

type MenuPosition = {
  top: number
  right: number
}

type LayoutUser = {
  username?: string
  role?: string
}

type LayoutUserMenuProps = {
  user: LayoutUser | null | undefined
  isOpen: boolean
  onToggle: () => void
  onClose: () => void
  onLogout: () => void
  themeMode?: ThemeMode
  onThemeChange?: (mode: ThemeMode) => void
}

export function LayoutUserMenu({
  user,
  isOpen,
  onToggle,
  onClose,
  onLogout,
  themeMode,
  onThemeChange,
}: LayoutUserMenuProps) {
  const location = useLocation()
  const triggerRef = useRef<HTMLButtonElement>(null)
  const onCloseRef = useRef(onClose)
  const [menuPosition, setMenuPosition] = useState<MenuPosition | null>(null)
  const [isOtpOpen, setIsOtpOpen] = useState(false)

  useEffect(() => {
    onCloseRef.current = onClose
  }, [onClose])

  const updateMenuPosition = useCallback(() => {
    const trigger = triggerRef.current
    if (!trigger) return
    const rect = trigger.getBoundingClientRect()
    setMenuPosition({
      top: rect.bottom + 12,
      right: Math.max(8, window.innerWidth - rect.right),
    })
  }, [])

  useLayoutEffect(() => {
    if (!isOpen) {
      setMenuPosition(null)
      return undefined
    }

    updateMenuPosition()
    window.addEventListener('resize', updateMenuPosition)
    window.addEventListener('scroll', updateMenuPosition, true)
    return () => {
      window.removeEventListener('resize', updateMenuPosition)
      window.removeEventListener('scroll', updateMenuPosition, true)
    }
  }, [isOpen, updateMenuPosition])

  useEffect(() => {
    if (!isOpen) return undefined

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onCloseRef.current()
    }

    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [isOpen])

  useEffect(() => {
    onCloseRef.current()
  }, [location.pathname, location.search])

  const menuPortal = isOpen && menuPosition && typeof document !== 'undefined'
    ? createPortal(
        <>
          <div
            key="layout-user-menu-backdrop"
            className="fixed inset-0 z-120 animate-overlay-in"
            aria-hidden="true"
            onPointerDown={onClose}
          />
          <div
            key="layout-user-menu-panel"
            role="menu"
            style={{ top: menuPosition.top, right: menuPosition.right, maxHeight: `calc(100dvh - ${menuPosition.top + 8}px)` }}
            className="nook-user-menu fixed z-121 w-56 origin-top-right overflow-y-auto rounded-2xl border border-(--app-border) bg-(--app-panel) p-2 shadow-xl animate-menu-in"
            onPointerDown={(event) => event.stopPropagation()}
          >
	            <UserMenuLink to="/profile" icon={<Settings size={16} />} label="个人资料" onNavigate={onClose} />
	            <UserMenuLink to="/favourites" icon={<Heart size={16} />} label="我的收藏" onNavigate={onClose} />
	            <UserMenuLink to="/playlists" icon={<ListMusic size={16} />} label="播放列表" onNavigate={onClose} />
	            <UserMenuLink to="/history" icon={<Clock size={16} />} label="观看历史" onNavigate={onClose} />
	            <UserMenuLink to="/dlna" icon={<Cast size={16} />} label="DLNA投屏" onNavigate={onClose} />
	            <button
	              type="button"
	              onClick={() => {
	                onClose()
	                setIsOtpOpen(true)
	              }}
	              className="flex w-full items-center gap-3 rounded-xl px-3 py-2 text-sm text-(--app-subtle) transition-colors hover:bg-(--app-hover) hover:text-(--app-text)"
	            >
	              <Tv size={16} />
	              <span>电视端临时登录码</span>
	            </button>
            {themeMode && onThemeChange ? (
              <div className="px-3 py-2 sm:hidden">
                <p className="mb-2 text-[10px] font-bold uppercase tracking-wider text-(--app-muted)">
                  主题
                </p>
                <LayoutThemeToggle mode={themeMode} onChange={onThemeChange} />
              </div>
            ) : null}
            {user?.role === 'admin' && (
              <>
                <div className="my-1.5 border-t border-(--app-border)" />
                <UserMenuLink to="/settings" icon={<Settings size={16} />} label="系统设置" onNavigate={onClose} />
              </>
            )}
            <div className="my-1.5 border-t border-(--app-border)" />
            <button
              onClick={() => {
                onClose()
                onLogout()
              }}
              className="flex w-full items-center gap-3 rounded-xl px-3 py-2 text-sm text-red-500 transition-colors hover:bg-(--app-danger-soft)"
            >
              <LogOut size={16} />
              <span>安全登出系统</span>
            </button>
          </div>
        </>,
        document.body,
      )
    : null

  return (
    <div className="relative z-122" data-testid="layout-user-menu">
      <button
        ref={triggerRef}
        onClick={onToggle}
        aria-expanded={isOpen}
        aria-haspopup="menu"
        className="nook-user-trigger relative z-122 flex items-center gap-2.5 rounded-full p-1 pr-3 transition-colors hover:bg-(--app-hover)"
      >
        <div className="flex h-8 w-8 items-center justify-center rounded-full bg-(--app-hover) text-xs font-medium text-(--app-text)">
          {user?.username?.slice(0, 2).toUpperCase() || 'US'}
        </div>
        <div className="hidden text-left md:block">
          <p className="text-[13px] font-medium leading-none text-(--app-text)">{user?.username}</p>
          <p className="mt-1 text-[11px] leading-none text-(--app-muted)">
            {user?.role === 'admin' ? '管理员' : user?.role === 'user' ? '普通用户' : ''}
          </p>
        </div>
        <ChevronDown size={14} className="text-(--app-muted)" />
      </button>
      {menuPortal}
      <TemporaryPasswordDialog isOpen={isOtpOpen} onClose={() => setIsOtpOpen(false)} />
    </div>
  )
}

function UserMenuLink({
  to,
  icon,
  label,
  onNavigate,
}: {
  to: string
  icon: ReactNode
  label: string
  onNavigate: () => void
}) {
  return (
    <Link
      to={to}
      onClick={onNavigate}
      className="flex items-center gap-3 rounded-xl px-3 py-2 text-sm text-(--app-subtle) transition-colors hover:bg-(--app-hover) hover:text-(--app-text)"
    >
      {icon}
      <span>{label}</span>
    </Link>
  )
}
