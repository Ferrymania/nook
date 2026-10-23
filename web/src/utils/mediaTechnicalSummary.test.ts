import { mediaTechnicalSummary } from './mediaTechnicalSummary'

function check(name: string, actual: string[], expected: string[]) {
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error(`${name}: ${JSON.stringify(actual)} != ${JSON.stringify(expected)}`)
  }
}

const media = { duration_sec: 5760, width: 3840, height: 1600, size_bytes: Math.round(10.35 * 1024 ** 3) }
check('cropped 4K and official DV profile', mediaTechnicalSummary({
  ...media, video_metadata: { frame_rate: 23.976, extended_video_type: 'DolbyVision', extended_video_sub_type: 'DoviProfile50' },
}), ['1h36m', '4K', 'DV(P5.0)', '23.976FPS', '10.35GB'])
check('HDR10+ and whole hours', mediaTechnicalSummary({
  ...media, duration_sec: 7200, video_metadata: { frame_rate: 60, extended_video_type: 'Hdr10Plus' },
}), ['2h', '4K', 'HDR10+', '60FPS', '10.35GB'])
check('missing metadata is hidden', mediaTechnicalSummary({ duration_sec: 0, width: 0, height: 0, size_bytes: 0 }), [])
check('legacy probe does not become SDR or HDR', mediaTechnicalSummary(media), ['1h36m', '4K', '10.35GB'])
check('unknown DV subtype retains DV without inventing a profile', mediaTechnicalSummary({
  ...media, video_metadata: { extended_video_type: 'DolbyVision', extended_video_sub_type: 'None' },
}), ['1h36m', '4K', 'DV', '10.35GB'])
check('invalid frame rates are hidden', mediaTechnicalSummary({
  duration_sec: -1, width: NaN, height: 1080, size_bytes: Infinity, video_metadata: { frame_rate: NaN },
}), [])
check('cropped full HD with HLG', mediaTechnicalSummary({
  ...media, width: 1920, height: 800, video_metadata: { extended_video_type: 'HyperLogGamma' },
}), ['1h36m', '1080p', 'HLG', '10.35GB'])
