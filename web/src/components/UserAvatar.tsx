import { useEffect, useState } from 'react'

import { profileAPI } from '../api/profile'
import type { User } from '../types'

export function UserAvatar({ user, className, square = false }: {
  user: Pick<User, 'id' | 'username' | 'avatar_url'> | null | undefined
  className: string
  square?: boolean
}) {
  const avatarAddress = user?.avatar_url
  const imageKey = avatarAddress ? `${user?.id}:${avatarAddress}` : ''
  const [image, setImage] = useState<{ key: string; objectURL: string }>()

  useEffect(() => {
    if (!imageKey) return
    const controller = new AbortController()
    let objectURL: string | undefined
    profileAPI.avatarImage(controller.signal).then((blob) => {
      if (controller.signal.aborted) return
      objectURL = URL.createObjectURL(blob)
      setImage({ key: imageKey, objectURL })
    }).catch(() => undefined)
    return () => {
      controller.abort()
      if (objectURL) URL.revokeObjectURL(objectURL)
    }
  }, [imageKey])

  return (
    <span className={`inline-flex shrink-0 items-center justify-center overflow-hidden bg-(--app-hover) font-medium text-(--app-text) ${square ? 'rounded-2xl' : 'rounded-full'} ${className}`}>
      {image && image.key === imageKey
        ? <img src={image.objectURL} alt="" className="size-full object-cover" onError={() => setImage(undefined)} />
        : user?.username?.slice(0, 2).toUpperCase() || 'US'}
    </span>
  )
}
