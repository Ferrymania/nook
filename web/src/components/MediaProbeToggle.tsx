import { Loader2, ScanLine } from 'lucide-react'

type MediaProbeToggleProps = {
  checked: boolean
  saving: boolean
  onChange: (checked: boolean) => void
}

export function MediaProbeToggle({ checked, saving, onChange }: MediaProbeToggleProps) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label="刮削时自动探测媒体信息"
      aria-busy={saving}
      disabled={saving}
      title="开启后会在刮削时读取视频技术信息。媒体数量较多时，连续探测会频繁访问网盘和视频源，可能触发限流或风控。默认关闭；单独手动探测不受影响。"
      onClick={() => onChange(!checked)}
      style={checked ? {
        backgroundColor: 'var(--app-brand-soft)',
        borderColor: 'var(--app-brand-border)',
        color: 'var(--app-brand-text)',
      } : undefined}
      className="btn-outline shrink-0 gap-2 px-3! py-1.5! text-xs sm:text-sm"
    >
      {saving ? <Loader2 size={16} className="animate-spin" /> : <ScanLine size={16} />}
      <span className="whitespace-nowrap">刮削时自动探测媒体信息</span>
      <span className="rounded-full bg-sand-300/30 px-2 py-0.5 text-[11px] font-bold">
        {checked ? '开启' : '关闭'}
      </span>
    </button>
  )
}
