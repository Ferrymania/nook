/** 取路径最后一级目录/文件名（兼容 / 与 \\）。 */
export function lastPathSegment(path: string): string {
  const trimmed = path.trim().replace(/[\\/]+$/, '')
  if (!trimmed) return ''
  const parts = trimmed.split(/[/\\]/).filter(Boolean)
  return parts[parts.length - 1] ?? ''
}

/** 优先取展示路径（115 浏览/反查得到）的末级目录名，否则取远端路径末段。 */
export function remoteTailNameOf(remotePath: string, displayPath?: string): string {
  const trimmed = displayPath?.trim()
  if (trimmed) return lastPathSegment(trimmed)
  if (remotePath.trim() === '0') return '' // 115 根目录
  return lastPathSegment(remotePath)
}

function pathSeparator(path: string): '/' | '\\' {
  return path.includes('\\') ? '\\' : '/'
}

/** 从已保存的输出目录还原自动拼接前的本地父目录。 */
export function localOutputBase(localPath: string, remoteTail: string): string {
  const trimmed = localPath.trim()
  if (!remoteTail) return trimmed
  const path = trimmed.replace(/[\\/]+$/, '')
  const suffix = '/' + remoteTail
  if (!path.replace(/\\/g, '/').endsWith(suffix)) return trimmed
  const base = path.slice(0, -suffix.length)
  return !base || /^[a-z]:$/i.test(base) ? base + pathSeparator(path) : base
}

/** 追加远端末级目录，避免重复添加已有的同名尾段。 */
export function appendRemoteTail(localPath: string, remoteTail: string): string {
  const base = localPath.trim()
  if (!base || !remoteTail || lastPathSegment(base) === remoteTail) return base
  return base.replace(/[\\/]+$/, '') + pathSeparator(base) + remoteTail
}
