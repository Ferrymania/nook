import { useCallback, useEffect, useRef, useState } from 'react'
import toast from 'react-hot-toast'

import { libraryAPI, type LibraryScanTask } from '../api/library'
import { useWebSocket } from '../hooks/useWebSocket'
import { scanTaskIsOlder, scanTaskMessage } from './libraryScanState'

type ScanState = {
  libraryID: string
  phase: 'checking' | 'idle' | 'submitting' | 'running' | 'completed' | 'failed'
  message: string
  task: LibraryScanTask | null
}

type UseLibraryScanStatusInput = {
  libraryID: string
  isAdmin: boolean
  onLibraryChanged: () => void
}

export function useLibraryScanStatus({ libraryID, isAdmin, onLibraryChanged }: UseLibraryScanStatusInput) {
  const [state, setState] = useState<ScanState>({ libraryID, phase: 'checking', message: '', task: null })
  const stateRef = useRef(state)
  const scope = useRef(0)
  const revision = useRef(0)
  const pending = useRef(false)
  const controller = useRef<AbortController | null>(null)
  const finished = useRef(new Set<string>())
  const changed = useRef(onLibraryChanged)
  useEffect(() => { changed.current = onLibraryChanged }, [onLibraryChanged])

  const apply = useCallback((next: ScanState) => {
    stateRef.current = next
    setState(next)
  }, [])

  const applyTask = useCallback((task: LibraryScanTask, live: boolean) => {
    if (task.library_id !== libraryID || task.kind !== 'scan') return
    const previous = stateRef.current
    if (previous.libraryID === libraryID && scanTaskIsOlder(previous.task, task)) return
    apply({ libraryID, phase: task.status, message: scanTaskMessage(task), task })
    if (task.status !== 'running' && !finished.current.has(task.id)) {
      finished.current.add(task.id)
      if (live || previous.phase === 'running' || previous.phase === 'submitting') changed.current()
    }
  }, [apply, libraryID])

  const refreshStatus = useCallback(async () => {
    if (!isAdmin || !libraryID || pending.current) return
    controller.current?.abort()
    const request = new AbortController()
    controller.current = request
    const generation = scope.current
    const version = ++revision.current
    try {
      const result = await libraryAPI.scanStatus(libraryID, request.signal)
      if (request.signal.aborted || generation !== scope.current || version !== revision.current) return
      if (result.task) {
        // The snapshot is authoritative: a newer root scan may have finished
        // while an older root of the same library is still running.
        stateRef.current = { ...stateRef.current, task: null }
        applyTask(result.task, false)
      } else {
        const wasRunning = stateRef.current.phase === 'running'
        apply({ libraryID, phase: stateRef.current.phase === 'failed' ? 'failed' : 'idle', message: wasRunning ? '当前没有运行中的扫描任务' : stateRef.current.phase === 'failed' ? '扫描请求失败，后台没有运行中的扫描任务，请重试' : '', task: null })
        if (wasRunning) changed.current()
      }
    } catch {
      if (request.signal.aborted || generation !== scope.current || version !== revision.current) return
      if (stateRef.current.phase === 'checking') apply({ libraryID, phase: 'idle', message: '暂时无法读取扫描状态，恢复连接后会重新检查', task: null })
    }
  }, [apply, applyTask, isAdmin, libraryID])

  useEffect(() => {
    scope.current += 1
    revision.current += 1
    pending.current = false
    apply({ libraryID, phase: isAdmin ? 'checking' : 'idle', message: '', task: null })
    void refreshStatus()
    const visible = () => { if (document.visibilityState === 'visible') void refreshStatus() }
    document.addEventListener('visibilitychange', visible)
    // Only this mounted page queries, and only while visible and scanning.
    // WebSocket supplies normal updates; this is a missed-event fallback.
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible' && stateRef.current.phase === 'running') void refreshStatus()
    }, 15_000)
    return () => {
      scope.current += 1
      revision.current += 1
      controller.current?.abort()
      document.removeEventListener('visibilitychange', visible)
      window.clearInterval(timer)
    }
  }, [apply, isAdmin, libraryID, refreshStatus])

  const onRealtimeEvent = useCallback((topic: string, payload: unknown) => {
    if (!isAdmin || !payload || typeof payload !== 'object') return
    // Keep automatic scans refreshing the list even when started outside the
    // manual scan endpoint. Tracked scans refresh on their terminal task event.
    if (topic === 'scan') {
      const event = payload as Record<string, unknown>
      if (event.library_id === libraryID && event.finished && !['running', 'submitting'].includes(stateRef.current.phase)) changed.current()
      return
    }
    if (topic !== 'task') return
    const task = payload as LibraryScanTask
    if (task.kind !== 'scan' || task.library_id !== libraryID) return
    revision.current += 1
    applyTask(task, true)
    if (task.status !== 'running') void refreshStatus()
  }, [applyTask, isAdmin, libraryID, refreshStatus])
  useWebSocket(onRealtimeEvent, refreshStatus)

  const handleScan = useCallback(async () => {
    if (!isAdmin || pending.current || ['checking', 'submitting', 'running'].includes(stateRef.current.phase)) return
    const generation = scope.current
    pending.current = true
    revision.current += 1
    const version = revision.current
    apply({ libraryID, phase: 'submitting', message: '正在提交扫描任务…', task: null })
    try {
      const result = await libraryAPI.scan(libraryID)
      if (generation !== scope.current) return
      // A fast scan can finish over WebSocket before this 202 response arrives.
      if (version === revision.current) {
        if (result.queued) apply({ libraryID, phase: 'running', message: result.message || '扫描已在后台启动', task: null })
        else {
          apply({ libraryID, phase: 'completed', message: `扫描完成：新增 ${result.added ?? 0} · 更新 ${result.updated ?? 0}`, task: null })
          changed.current()
        }
      }
      if (result.already_running) toast('该媒体库已有扫描任务，已恢复任务状态')
    } catch {
      if (generation !== scope.current) return
      if (version === revision.current) apply({ libraryID, phase: 'failed', message: '扫描请求未确认，正在核对后台任务状态…', task: null })
      toast.error('扫描请求失败，正在核对后台状态')
    } finally {
      if (generation === scope.current) {
        pending.current = false
        void refreshStatus()
      }
    }
  }, [apply, isAdmin, libraryID, refreshStatus])

  const current = state.libraryID === libraryID ? state : { phase: 'checking', message: '' }
  const scanning = ['checking', 'submitting', 'running'].includes(current.phase)
  return {
    scanning,
    scanProgress: current.message,
    scanLabel: current.phase === 'checking' ? '检查扫描状态…' : current.phase === 'submitting' ? '提交中…' : current.phase === 'running' ? '扫描中…' : '扫描媒体库',
    handleScan,
  }
}
