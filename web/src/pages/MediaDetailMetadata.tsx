import { Star } from 'lucide-react'

import type { Media } from '../types'
import { mediaTechnicalSummary } from '../utils/mediaTechnicalSummary'

type MediaDetailMetadataProps = {
  media: Media
}

export function MediaDetailMetadata({ media }: MediaDetailMetadataProps) {
  const heading = media.episode_title?.trim() || media.title
  const showTitleContext = Boolean(media.episode_title?.trim() && media.title && media.title !== heading)
  const genres = parseCSV(media.genres)
  const languages = parseCSV(media.languages)
  const countries = parseCSV(media.countries)
  const hasTaxonomy = genres.length > 0 || languages.length > 0 || countries.length > 0
  const technicalSummary = mediaTechnicalSummary(media)

  return (
    <>
      <div className="nook-detail-heading space-y-3">
        <h1 className="font-display text-3xl sm:text-4xl font-extrabold tracking-tight text-gray-900 leading-tight">
          {heading}
        </h1>
        {showTitleContext && (
          <p className="text-sm font-semibold text-gray-500">
            {media.title}
          </p>
        )}
        {(media.rating > 0 || media.year > 0) && (
          <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm font-medium text-(--app-subtle) tabular-nums">
            {Number.isFinite(media.rating) && media.rating > 0 && (
              <span className="inline-flex items-center gap-1.5" aria-label={`评分 ${media.rating.toFixed(1)}`}>
                <Star size={15} fill="currentColor" className="text-(--app-danger-text)" aria-hidden="true" />
                {media.rating.toFixed(1)}
              </span>
            )}
            {media.year > 0 && <span>{media.year} 年</span>}
          </div>
        )}
        {technicalSummary.length > 0 && (
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm font-medium text-(--app-subtle) tabular-nums" aria-label="媒体信息">
            {technicalSummary.map((value, index) => <span key={index} className="whitespace-nowrap">{value}</span>)}
          </div>
        )}
      </div>

      {media.overview && (
        <div className="nook-detail-overview rounded-xl bg-(--app-panel-soft) p-5 space-y-2">
          <h2 className="text-[13px] font-medium text-(--app-subtle)">剧情简介</h2>
          <p className="text-sm text-(--app-subtle) leading-relaxed">
            {media.overview}
          </p>
        </div>
      )}

      {hasTaxonomy && (
        <div className="nook-detail-taxonomy space-y-3">
          <MetadataRow label="类型流派" values={parseCSV(media.genres)} tone="brand" />
          <MetadataRow label="语言" values={parseCSV(media.languages)} />
          <MetadataRow label="国家/地区" values={parseCSV(media.countries)} />
          </div>
      )}
    </>
  )
}

/**
 * One "label - values" row of the taxonomy block.
 *
 * All rows share the same label gutter, so every value column starts on the
 * same vertical line. Pills keep one geometry and differ by tone only.
 */
function MetadataRow({
  label,
  values,
  tone = 'neutral',
}: {
  label: string
  values: string[]
  tone?: 'neutral' | 'brand'
}) {
  if (values.length === 0) return null
  const tagClass = [
    'inline-flex items-center rounded-lg border px-2.5 py-1 text-xs font-medium',
    tone === 'brand'
      ? 'border-brand-100/50 bg-brand-50 text-brand-700'
      : 'border-gray-200/60 bg-gray-100 text-gray-600',
  ].join(' ')

  return (
    <div className="grid grid-cols-[4.5rem_1fr] items-start gap-2 sm:grid-cols-[5.5rem_1fr]">
      <span className="pt-1 text-xs font-semibold text-gray-500">{label}</span>
      <div className="flex flex-wrap gap-1.5">
        {values.map((value) => (
          <span key={value} className={tagClass}>
            {value}
          </span>
        ))}
      </div>
    </div>
  )
}

function parseCSV(s?: string): string[] {
  if (!s) return []
  return s.split(',').map((x) => x.trim()).filter(Boolean)
}
