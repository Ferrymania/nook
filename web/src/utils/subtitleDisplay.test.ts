import { loadSubtitleStyle, saveSubtitleStyle, subtitleTextStyle, type SubtitleStylePreset } from './subtitleDisplay'

function check(label: string, actual: unknown, expected: unknown) {
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error(`${label}: got ${JSON.stringify(actual)}, want ${JSON.stringify(expected)}`)
  }
}

// These values are saved by existing players; a CSS migration must not rename
// them to Tailwind utility names and reset a user's chosen subtitle appearance.
const stored = new Map<string, string>()
const previousWindow = Object.getOwnPropertyDescriptor(globalThis, 'window')
Object.defineProperty(globalThis, 'window', {
  configurable: true,
  value: { localStorage: {
    getItem: (key: string) => stored.get(key) ?? null,
    setItem: (key: string, value: string) => stored.set(key, value),
  } },
})

try {
  for (const legacy of ['shadow', 'outline', 'box'] satisfies SubtitleStylePreset[]) {
    stored.set('nook.subtitle.style', legacy)
    check(`load existing ${legacy} preference`, loadSubtitleStyle(), legacy)
    saveSubtitleStyle(loadSubtitleStyle())
    check(`keep persisted ${legacy} value`, stored.get('nook.subtitle.style'), legacy)
  }
  stored.clear()
  check('missing preference keeps the original default', loadSubtitleStyle(), 'shadow')
  check('outline preference selects the outline rendering', subtitleTextStyle('outline').fontWeight, 700)
  check('box preference selects the background rendering', subtitleTextStyle('box').background, 'rgba(0, 0, 0, 0.72)')
} finally {
  if (previousWindow) Object.defineProperty(globalThis, 'window', previousWindow)
  else Reflect.deleteProperty(globalThis, 'window')
}
