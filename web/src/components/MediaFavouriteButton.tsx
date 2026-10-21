import { Heart } from 'lucide-react'

type MediaFavouriteButtonProps = {
  favourite: boolean
  onToggle: () => void
  disabled?: boolean
  variant?: 'compact' | 'inline'
  className?: string
}

export function MediaFavouriteButton({
  favourite,
  onToggle,
  disabled = false,
  variant = 'inline',
  className = '',
}: MediaFavouriteButtonProps) {
  if (variant === 'compact') {
    return (
      <button
        type="button"
        title={favourite ? '取消收藏' : '加入收藏'}
        aria-label={favourite ? '取消收藏' : '加入收藏'}
        aria-pressed={favourite}
        disabled={disabled}
        onClick={(event) => {
          event.preventDefault()
          event.stopPropagation()
          onToggle()
        }}
        className={
          'nook-poster-action' +
          (className ? ` ${className}` : '')
        }
      >
        <Heart size={13} fill={favourite ? 'currentColor' : 'none'} />
      </button>
    )
  }

  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onToggle}
      className={
        'btn-outline gap-2 ' +
        (favourite
          ? '!border-red-200 !bg-red-50 !text-red-600 hover:!bg-red-100/50'
          : 'hover:border-red-200 hover:text-red-600 hover:bg-red-50/50') +
        (className ? ` ${className}` : '')
      }
    >
      <Heart size={14} fill={favourite ? 'currentColor' : 'none'} />
      <span>{favourite ? '取消收藏' : '加入收藏'}</span>
    </button>
  )
}
