import { ArrowUp, ChevronRight, Home, RefreshCw } from 'lucide-react'

type FileManagerToolbarProps = {
  currentPath?: string
  parent?: string
  roots?: { path: string }[]
  recursive: boolean
  loading: boolean
  onNavigate: (path: string) => void
  onRefresh: () => void
  onRecursiveChange: (value: boolean) => void
}

export function FileManagerToolbar({ currentPath, parent, roots, recursive, loading, onNavigate, onRefresh, onRecursiveChange }: FileManagerToolbarProps) {
  const normalize = (path: string) => path.replace(/\\/g, '/').replace(/\/$/, '')
  const current = normalize(currentPath || '')
  const root = roots?.map(item => normalize(item.path))
    .filter(path => current === path || current.startsWith(`${path}/`))
    .sort((a, b) => b.length - a.length)[0]
  const relative = root === undefined ? [] : current.slice(root.length).split('/').filter(Boolean)
  const crumbs = root === undefined ? [] : [
    { name: root || '/', path: root || '/' },
    ...relative.map((name, index) => ({ name, path: root + '/' + relative.slice(0, index + 1).join('/') })),
  ]

  return <div className="file-toolbar">
    <nav aria-label="文件路径" className="file-breadcrumbs">
      {currentPath && <button type="button" className="file-icon-button" title="上一级" aria-label="上一级"
        disabled={loading} onClick={() => onNavigate(parent || '')}><ArrowUp size={17} /></button>}
      <button type="button" className="file-crumb" disabled={loading} aria-current={!currentPath ? 'page' : undefined} onClick={() => onNavigate('')}>
        <Home size={16} /><span>根目录</span>
      </button>
      {crumbs.length ? crumbs.map((item, index) => <span className="file-crumb-segment" key={item.path}>
        <ChevronRight size={13} aria-hidden="true" />
        {index === crumbs.length - 1 ? <span aria-current="page" title={currentPath}>{item.name}</span>
          : <button type="button" className="file-crumb" disabled={loading} onClick={() => onNavigate(item.path)}>{item.name}</button>}
      </span>) : currentPath && <span className="file-crumb-segment"><ChevronRight size={13} /><span aria-current="page">{currentPath}</span></span>}
    </nav>
    <div className="file-toolbar-actions">
      {currentPath && <label className="file-recursive">
        <input type="checkbox" checked={recursive} disabled={loading} onChange={event => onRecursiveChange(event.target.checked)} />
        包含子目录
      </label>}
      <button type="button" className="file-icon-button" aria-label="刷新" title="刷新" disabled={loading} onClick={onRefresh}>
        <RefreshCw size={17} className={loading ? 'animate-spin' : ''} />
      </button>
    </div>
  </div>
}
