/* eslint-disable react-refresh/only-export-components */
import { lazy, type ReactElement } from 'react'
import { Navigate, useLocation } from 'react-router-dom'

import { LIBRARY_QUEUE_PATH, libraryQueueLocation, type LibraryQueueTab } from './utils/libraryQueueNavigation'

const HomePageLoader = () => import('./pages/HomePage').then((m) => ({ default: m.HomePage }))
const LibraryPageLoader = () => import('./pages/LibraryPage').then((m) => ({ default: m.LibraryPage }))
const LibrariesPageLoader = () => import('./pages/LibrariesPage').then((m) => ({ default: m.LibrariesPage }))
const FavouritesPageLoader = () => import('./pages/FavouritesPage').then((m) => ({ default: m.FavouritesPage }))
const PlaylistsPageLoader = () => import('./pages/PlaylistsPage').then((m) => ({ default: m.PlaylistsPage }))
const PlaylistDetailPageLoader = () =>
  import('./pages/PlaylistDetailPage').then((m) => ({ default: m.PlaylistDetailPage }))
const MediaDetailPageLoader = () =>
  import('./pages/MediaDetailPage').then((m) => ({ default: m.MediaDetailPage }))
const PlayerPageLoader = () => import('./pages/PlayerPage').then((m) => ({ default: m.PlayerPage }))
const WatchHistoryPageLoader = () =>
  import('./pages/WatchHistoryPage').then((m) => ({ default: m.WatchHistoryPage }))
const WatchStatsPageLoader = () =>
  import('./pages/WatchStatsPage').then((m) => ({ default: m.WatchStatsPage }))

const HomePage = lazy(HomePageLoader)
const LibraryPage = lazy(LibraryPageLoader)
const LibrariesPage = lazy(LibrariesPageLoader)
const FavouritesPage = lazy(FavouritesPageLoader)
const PlaylistsPage = lazy(PlaylistsPageLoader)
const PlaylistDetailPage = lazy(PlaylistDetailPageLoader)
const MediaDetailPage = lazy(MediaDetailPageLoader)
const PlayerPage = lazy(PlayerPageLoader)
const AdminPage = lazy(() => import('./pages/AdminPage').then((m) => ({ default: m.AdminPage })))
const ProfilePage = lazy(() => import('./pages/ProfilePage').then((m) => ({ default: m.ProfilePage })))
const WatchHistoryPage = lazy(WatchHistoryPageLoader)
const WatchStatsPage = lazy(WatchStatsPageLoader)
const PosterWallPage = lazy(() => import('./pages/PosterWallPage').then((m) => ({ default: m.PosterWallPage })))
const SettingsPage = lazy(() => import('./pages/SettingsPage').then((m) => ({ default: m.SettingsPage })))
const EmbyMountPage = lazy(() => import('./pages/EmbyMountPage').then((m) => ({ default: m.EmbyMountPage })))
const StrmManagePage = lazy(() => import('./pages/StrmManagePage').then((m) => ({ default: m.StrmManagePage })))
const StrmDownloadQueuePage = lazy(() =>
  import('./pages/StrmQueuePage').then((m) => ({ default: m.StrmDownloadQueuePage })),
)
const StrmUploadQueuePage = lazy(() =>
  import('./pages/StrmQueuePage').then((m) => ({ default: m.StrmUploadQueuePage })),
)
const LibraryQueuePage = lazy(() => import('./pages/LibraryQueuePage').then((m) => ({ default: m.LibraryQueuePage })))

function LegacyLibraryQueueRedirect({ tab }: { tab: LibraryQueueTab }) {
  const { search, hash, state } = useLocation()
  return <Navigate to={{ ...libraryQueueLocation(tab, search), hash }} state={state} replace />
}

function LegacyTaskQueueRedirect() {
  const { search, hash, state } = useLocation()
  const params = new URLSearchParams(search)
  const type = params.get('type')
  params.delete('type')
  switch (type) {
    case 'scrape':
    case 'segments':
      return <Navigate to={{ ...libraryQueueLocation(type, params.toString()), hash }} state={state} replace />
    case 'download':
      return <Navigate to="/strm/downloads" replace />
    case 'upload':
      return <Navigate to="/strm/uploads" replace />
    default:
      return <Navigate to="/libraries" replace />
  }
}

// 常用页面的路由 chunk 空闲预取：应用加载完成后浏览器一空闲就把浏览路径
// （库列表/库详情/媒体详情/收藏/历史等）的 chunk 拉下来，
// 首次点击进入时不再出现"加载中…"等 chunk 下载。失败静默（导航时会重试）。
let prefetchStarted = false
export function prefetchCommonRouteChunks() {
  if (prefetchStarted || typeof window === 'undefined') return

  // 弱网/省流模式不做空闲预取：首屏预览和海报加载优先占用带宽。
  const connection = (navigator as Navigator & {
    connection?: { saveData?: boolean; effectiveType?: string }
  }).connection
  if (
    connection?.saveData ||
    connection?.effectiveType === 'slow-2g' ||
    connection?.effectiveType === '2g'
  ) {
    return
  }

  prefetchStarted = true
  const loaders = [
    LibrariesPageLoader,
    LibraryPageLoader,
    MediaDetailPageLoader,
    FavouritesPageLoader,
    WatchHistoryPageLoader,
    PlaylistsPageLoader,
  ]
  const run = () => {
    for (const load of loaders) {
      load().catch(() => undefined)
    }
  }
  if (typeof requestIdleCallback !== 'undefined') {
    requestIdleCallback(run, { timeout: 8000 })
  } else {
    window.setTimeout(run, 2500)
  }
}

export type AppRoute = {
  path?: string
  index?: boolean
  element: ReactElement
  adminOnly?: boolean
}

export const appRoutes: AppRoute[] = [
  { index: true, element: <HomePage /> },
  { path: 'libraries', element: <LibrariesPage /> },
  { path: 'library/:id', element: <LibraryPage /> },
  { path: 'favourites', element: <FavouritesPage /> },
  { path: 'playlists', element: <PlaylistsPage /> },
  { path: 'playlist/:id', element: <PlaylistDetailPage /> },
  { path: 'media/:id', element: <MediaDetailPage /> },
  { path: 'play/:id', element: <PlayerPage /> },
  { path: 'profile', element: <ProfilePage /> },
  { path: 'history', element: <WatchHistoryPage /> },
  { path: 'history/stats', element: <WatchStatsPage /> },
  { path: 'poster-wall', element: <PosterWallPage /> },
  { path: 'files', element: <Navigate to="/strm" replace />, adminOnly: true },
  { path: 'settings', element: <SettingsPage />, adminOnly: true },
  { path: 'emby-mount', element: <EmbyMountPage />, adminOnly: true },
  { path: 'strm', element: <StrmManagePage />, adminOnly: true },
  { path: 'strm/downloads', element: <StrmDownloadQueuePage />, adminOnly: true },
  { path: 'strm/uploads', element: <StrmUploadQueuePage />, adminOnly: true },
  { path: LIBRARY_QUEUE_PATH, element: <LibraryQueuePage />, adminOnly: true },
  { path: 'scraper/queue', element: <LegacyLibraryQueueRedirect tab="scrape" />, adminOnly: true },
  { path: 'segments/queue', element: <LegacyLibraryQueueRedirect tab="segments" />, adminOnly: true },
  { path: 'queue', element: <LegacyTaskQueueRedirect />, adminOnly: true },
  { path: 'admin', element: <AdminPage />, adminOnly: true },
]
