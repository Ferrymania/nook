# 播放列表

播放列表属于用户。网页与 Nook 的 Emby 兼容接口共用同一组列表和播放顺序，不会复制媒体文件。

## 网页操作

- 媒体详情页点击「加入播放列表」，选择已有列表，或填写名称后「新建并加入」。整剧详情页会按季集顺序加入全部单集，已有条目自动跳过。
- 播放列表页可以创建、删除列表；列表详情页可以重命名、调整顺序、移除影片，以及从任意一项开始播放。
- 从列表进入播放器后，上一项、下一项与自动连播都沿用列表顺序。自动连播仍遵循用户的「自动播放下一集」设置。
- 网页新建列表默认为私有。删除列表或移除条目不会删除媒体文件、收藏或观看进度。

## 网页接口

以下路径需要 Nook 登录凭据。`id` 是原生播放列表 ID，`entry_id` 是列表条目的 ID；两者都不同于媒体 ID。

| 方法与路径 | 请求 / 返回 |
| --- | --- |
| `GET /api/playlists?media_id=…` | 当前用户的列表，包含 `item_count`、`contains_media` |
| `POST /api/playlists` | `{ "name": "周末电影", "is_public": false, "media_ids": ["媒体 ID"] }`；返回列表，状态码 201 |
| `GET /api/playlists/:id` | `playlist`、有权观看的 `items`、带稳定条目 ID 的 `entries`、`can_edit` |
| `PUT /api/playlists/:id` | `{ "name": "新名称" }`；成功 204 |
| `POST /api/playlists/:id/items` | `{ "media_ids": ["媒体 ID"] }`；返回 `{ "added": 1 }`，兼容原来的单个 `media_id` |
| `POST /api/playlists/:id/reorder` | `{ "order": ["媒体 ID"] }`；未指定的条目按原顺序接在末尾 |
| `DELETE /api/playlists/:id/items/by-id/:entry_id` | 移除指定列表条目 |
| `DELETE /api/playlists/:id/items/:media_id` | 兼容按媒体 ID 移除 |
| `DELETE /api/playlists/:id` | 删除列表及列表条目 |

名称去除首尾空白后须为 1–128 个字符。一次创建、添加或重排最多提交 1,000 个媒体 ID。重复添加会跳过已有媒体；创建并添加为同一个事务，添加失败不会留下空列表。

私有列表仅创建者和管理员可读。公开列表可按 ID 读取，但仍按读取者的媒体库、内容分级等权限过滤。公开不赋予编辑权限。编辑操作仅允许创建者或管理员；查询、增加、移除和重排均使用实际登录身份，不信任请求中额外传入的用户 ID。

## Emby 兼容接口

Emby 列表 ID 为 `playlist~原生列表ID`；播放列表目录 ID 为 `nook-playlists`。以下路径支持 `/emby` 前缀及常用大小写形式。

| 方法与路径 | 用途 |
| --- | --- |
| `GET /Users/:userId/Views` | 用户有列表时返回 `CollectionType: playlists` 的目录入口 |
| `GET /Items?ParentId=nook-playlists` | 列出当前用户的播放列表 |
| `GET /Items?IncludeItemTypes=Playlist&Recursive=true` | 查询、名称搜索、排序及分页；也可使用 `/Users/:userId/Items` |
| `GET /Items/:playlistId` | 返回 `Type: Playlist` 的列表信息 |
| `GET /Playlists/:playlistId/Items` | 按列表顺序读取内容，支持 `StartIndex`、`Limit`；每项包含 `PlaylistItemId` |
| `GET /Items?ParentId=:playlistId` | 读取列表内容的兼容入口 |
| `POST /Playlists?Name=…&Ids=…&MediaType=Video` | 创建列表，可同时添加电影、单集，或将整剧 / 季展开为单集 |
| `POST /Playlists/:playlistId/Items?Ids=…` | 添加媒体 |
| `GET /Playlists/:playlistId/AddToPlaylistInfo?Ids=…` | 返回 `ItemCount` 与 `ContainsDuplicates` |
| `DELETE /Playlists/:playlistId/Items?EntryIds=…` | 用 `PlaylistItemId` 移除条目；兼容 `POST …/Items/Delete` |
| `POST /Playlists/:playlistId/Items/:entryId/Move/:newIndex` | 移动条目，位置从 0 开始 |
| `POST /Items/:playlistId` | JSON `{ "Name": "新名称" }` 重命名 |
| `DELETE /Items/:playlistId` | 删除列表 |

网页根路径 `/playlists` 保留用于打开页面；小写列表查询可以使用 `/emby/playlists`。通用 `/Items` 的重命名、删除端点仅接受播放列表 ID，不能用来修改普通媒体。

这些接口让支持 Emby 播放列表的客户端能够发现和管理 Nook 列表；客户端是否显示入口，仍由客户端实现决定。尚未完成 SenPlayer 真机联调，不能仅凭接口已实现就认定它一定展示入口。
