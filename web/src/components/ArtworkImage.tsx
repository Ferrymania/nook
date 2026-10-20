import { useState, type ImgHTMLAttributes } from 'react'
import { ImageOff } from 'lucide-react'

import { isMissingArtwork } from '../utils/artworkSources'

type ArtworkImageProps = Omit<ImgHTMLAttributes<HTMLImageElement>, 'src' | 'srcSet' | 'alt' | 'onError'> & {
  sources: string[]
  alt: string
  compact?: boolean
  fallbackClassName?: string
}

export function ArtworkImage(props: ArtworkImageProps) {
  const sources = [...new Set(props.sources.map(source => source.trim()).filter(Boolean))]
  // A new media/version must retry its own sources, including URLs that failed
  // on the previous media. Keeping the state in this keyed child also ignores
  // late image events from the previous source list.
  return <ArtworkImageAttempt key={JSON.stringify(sources)} {...props} sources={sources} />
}

function ArtworkImageAttempt({ sources, alt, compact = false, fallbackClassName = '', onLoad, ...props }: ArtworkImageProps) {
  const [attempt, setAttempt] = useState(0)
  const source = sources[attempt]
  const next = () => setAttempt(current => current === attempt ? current + 1 : current)

  if (!source) {
    return (
      <div role="img" aria-label={`${alt}：图片暂不可用`} title="图片暂不可用" className={`flex flex-col items-center justify-center gap-2 bg-[var(--app-panel-soft)] text-[var(--app-muted)] ${fallbackClassName}`}>
        <ImageOff size={compact ? 16 : 28} aria-hidden="true" />
        <span className={compact ? 'text-[11px] leading-none' : 'text-xs'}>{compact ? '缺图' : '图片暂不可用'}</span>
      </div>
    )
  }

  return <img key={source} loading="lazy" decoding="async" referrerPolicy="no-referrer" {...props} src={source} alt={alt} onError={next}
    onLoad={event => {
      if (isMissingArtwork(event.currentTarget.naturalWidth, event.currentTarget.naturalHeight)) next()
      else onLoad?.(event)
    }} />
}
