import { FolderOpen } from 'lucide-react'

type FileRoot = {
  label: string
  path: string
}

type FileBrowserRootsProps = {
  roots: FileRoot[]
  onOpen: (path: string) => void
}

export function FileBrowserRoots({ roots, onOpen }: FileBrowserRootsProps) {
  if (roots.length === 0) return <p className="file-list-state">暂无可访问的目录</p>
  return (
    <div className="file-roots">
      {roots.map((root) => (
        <button
          key={root.path}
          onClick={() => onOpen(root.path)}
          className="file-root"
        >
          <FolderOpen size={20} className="file-folder-icon" />
          <div className="min-w-0">
            <p className="truncate text-sm text-(--app-text)" title={root.label}>{root.label.startsWith('library:') ? root.path.split(/[\\/]/).filter(Boolean).pop() : root.label}</p>
            <p className="truncate text-xs text-(--app-muted)" title={root.path}>{root.path}</p>
          </div>
        </button>
      ))}
    </div>
  )
}
