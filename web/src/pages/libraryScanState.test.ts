import { scanTaskIsOlder, scanTaskMessage } from './libraryScanState.ts'
import type { LibraryScanTask } from '../api/library.ts'

const task: LibraryScanTask = { id: 'a', library_id: 'library', kind: 'scan', status: 'running', started_at: '2026-10-18T00:00:00Z', updated_at: '2026-10-18T00:00:01Z', metrics: { visited: 20, added: 2, updated: 3 } }
function check(condition: boolean, message: string) { if (!condition) throw new Error(message) }
check(scanTaskMessage(task).includes('已扫描 20 项 · 新增 2 · 更新 3'), 'running counts')
const completed = { ...task, status: 'completed' as const, updated_at: '2026-10-18T00:00:02Z' }
check(scanTaskIsOlder(completed, task), 'late running event must not reopen completed task')
check(scanTaskIsOlder(task, { ...task, updated_at: '2026-10-18T00:00:00Z' }), 'out of order progress')
check(scanTaskIsOlder(task, { ...task, id: 'old', started_at: '2026-10-17T00:00:00Z' }), 'previous scan must not replace new scan')
check(!scanTaskIsOlder(task, { ...task, id: 'new', started_at: '2026-10-18T01:00:00Z' }), 'new scan accepted')
check(scanTaskMessage({ ...completed, metrics: { errors: 2 } }).includes('2 项异常'), 'partial errors visible')
check(scanTaskMessage({ ...task, status: 'failed', error: '目录不可用' }) === '扫描失败：目录不可用', 'failure reason preserved')
console.log('libraryScanState.test.ts ok')
