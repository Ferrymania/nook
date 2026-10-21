import { formatKnownDuration, formatKnownVideoSize } from './mediaStatistics'

function equal(actual: string, expected: string) {
  if (actual !== expected) throw new Error(`expected ${expected}, got ${actual}`)
}

// Unknown source metadata must not look like zero bytes or a STRM pointer size.
equal(formatKnownVideoSize(248, 0, 1), '—')
equal(formatKnownVideoSize(0, 0, 10), '—')
equal(formatKnownVideoSize(0, 0, 0), '0 B')
equal(formatKnownVideoSize(12, 1, 1), '12 B')
equal(formatKnownVideoSize(1024, 1, 2), '1.00 KiB')
equal(formatKnownVideoSize(1024 ** 4 * 12.59, 1891, 2004), '12.59 TiB')
equal(formatKnownDuration(0, 0), '0 h')
equal(formatKnownDuration(0, 1), '—')
equal(formatKnownDuration(30, 1), '< 1 min')
equal(formatKnownDuration(1800, 1), '30 min')
equal(formatKnownDuration(3722, 2), '1 h')
console.log('media statistics: 11 regression checks passed')
