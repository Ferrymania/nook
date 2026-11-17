export interface LibraryPlaybackSettings {
  played_threshold_pct: number
  min_resume_pct: number
  min_resume_duration_seconds: number
}

export function playbackSettingsError(percent: string, minimum: string, duration: string): string {
  if (!percent.trim() || !Number.isInteger(Number(percent)) || Number(percent) < 1 || Number(percent) > 100) return '播放完成阈值须为 1–100 的整数百分比'
  if (!minimum.trim() || !Number.isInteger(Number(minimum)) || Number(minimum) < 0 || Number(minimum) >= Number(percent)) return '最小续播百分比须为不小于 0 且小于播放完成阈值的整数'
  if (!duration.trim() || !Number.isInteger(Number(duration)) || Number(duration) < 0 || Number(duration) > 2147483647) return '最短可续播视频时长须为 0–2147483647 的整数秒数'
  return ''
}
