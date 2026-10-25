# Fork 的媒体兼容修复与打包

本次保留现有自动探测触发方式。`PlaybackInfo` 和视频请求不会因为这些改动而启动新的后台探测；资料缺失时仍需要手动探测，或等待原有触发路径生成缓存。

修复内容：

- 手动探测先解析 STRM/云盘目标及请求头，再探测实际视频，并保存完整资料到现有 `media_probes`。
- Emby 详情和 PlaybackInfo 读取有效的已有探测缓存，返回真实轨道、容器、时长与大小；外挂字幕索引随真实轨道调整，保持获取地址正确。
- 电影不再包含季号、集号字段；真实剧集与第 0 季特别篇仍保留这些字段。
- 没有真实总时长时返回未知，不再根据播放进度估算 30 分钟；已有客户端上报的总时长可用于详情和播放响应。STRM 文本大小不再作为影片大小返回。
- 登录响应补充用户配置、权限及会话字段，成功登录立即返回实际登录时间，避免异步数据库更新时出现空日期。Lenna 是否还要求其他字段需要客户端实测确认。

没有新增数据库表或列；文件大小放在已有 JSON payload 内，旧缓存仍能读取。无需为这些改动重新登录网盘、重新生成全部 STRM 或重新刮削电影。

## 自动打包与镜像

推送到 `main` 会自动运行 **Build & Publish**，生成：

- `ghcr.io/ferrymania/nook:latest` 与对应版本标签的 amd64/arm64 镜像。
- GitHub Release，以及 Linux amd64/arm64、Windows amd64 和 macOS arm64 安装包。

版本号沿用现有 `version` 分支管理方式，每次 main 发布递增 patch；fork 首次从 `0.0.1` 开始。自动发布不连接服务器部署。

Release 说明根据上一个发布标签之后的提交自动生成，按 Features、Fixes、Docs、Build / CI 等分类，并附提交链接和完整版本对比。提交标题格式见 [贡献规范](../CONTRIBUTING.md#commit-message-建议)。无新增提交的重新构建会明确说明代码未变；发布重试会重新生成更新说明。

## 手动构建测试镜像

使用 GitHub Actions 的 **Publish Docker image** 工作流：

1. 选择包含修复的分支运行工作流。
2. `ref` 填写要构建的分支或提交，`version` 填写固定镜像标签，例如 `fixes-20261014`。
3. 产物为 `ghcr.io/ferrymania/nook:fixes-20261014`，支持 `linux/amd64` 和 `linux/arm64`。

此手动工作流不依赖 `version` 分支，也不自动连接服务器部署。只有 `ref=main` 才额外更新 `latest`，构建修复分支不会覆盖稳定标签。

所有 Compose 模板的运行镜像与应用内更新目标共用 `NOOK_IMAGE`，默认使用 `ghcr.io/ferrymania/nook:latest`。使用固定测试标签时，在部署目录的 `.env` 设置：

```dotenv
NOOK_IMAGE=ghcr.io/ferrymania/nook:fixes-20261014
```

首次发布后，需要确认 GHCR 包对部署机器可见；私有包需要对应的拉取凭据。首次镜像发布完成前，上述 fork 镜像地址还不可用于拉取。

本地验证打包使用原有 Dockerfile，例如：

```sh
docker build --platform linux/amd64 --build-arg VERSION=fixes-20261014 -t nook:fixes-20261014 .
```

## 电视剧图片与资料兼容

整剧海报、背景和简介独立保存在现有 `series` 表，单集图片保存在 `media.episode_still_url`。关闭“每集图片”只跳过新单集图片，保留已有图片；整剧背景照常获取。首页和整剧详情不再复用单集剧情及小图。

升级会保留旧版已有单集图片，并在后台按已确认的 TMDb ID 为电视/动漫/综艺库补齐缺少的整剧资料，每个分组补取一次。已有整剧记录会跳过；来源请求失败会记录日志，可通过后续重新刮削或重启重试。此过程不重新识别分集、不探测视频、不改 STRM、条目 ID 或播放进度。

Emby 兼容接口支持 SenPlayer 首页同时请求 `Series,Movie,Video`，保留两类条目并统一分页。图片地址变化时更新缓存标识，让客户端重新获取图片。底部统计是否重新请求仍取决于客户端的刷新行为。

## Nook 命名与旧安装升级

从 `0.0.2` 开始，界面、程序、Go 模块、Docker 服务、环境变量和发布包统一使用 Nook：

- Go 模块为 `github.com/Ferrymania/nook`；程序为 `nook` / `nook.exe`。
- Docker 服务为 `nook`，健康检查为 `/usr/local/bin/nook-healthcheck`。
- 配置环境变量使用 `NOOK_*`，数据库使用 PostgreSQL；发布标签为 `nook-v版本号`，安装包为 `nook_平台_架构`。

升级前须将旧 `MEBOX_*` 环境变量改为对应的 `NOOK_*` 名称。本版本不再读取旧环境变量或迁移旧品牌浏览器偏好；已保存的 `nook` 登录状态、主题和偏好继续使用。旧页面 `/play-profiles`、`/playback-settings`、`/api-configs`、`/tools` 不再单独跳转，请使用当前菜单入口。旧 Compose 的健康检查脚本路径与旧逻辑备份格式仍保留兼容，不改变 `.jwt_secret`。

当前播放设置保存在用户账号中，旧 `play_profiles` 数据不再自动导入；仍需该迁移的旧部署，应先升级到包含 Profile 迁移的版本（PR #44）完成转换，再升级本版本。启动时保留旧表和数据，不执行删除。媒体库标签要求前后端同步升级；读取失败仍显示缓存，写入失败回滚并提示。

已有 PostgreSQL 的用户、数据库名和密码属于实际部署数据，不能仅因改名就替换成默认的 `nook`。将配置键改为 `NOOK_DATABASE_DSN` 时，保留原 DSN 的连接参数和现有挂载目录。将 Compose 服务从 `mebox` 改为 `nook` 前，先停止原应用服务，避免两个应用同时访问相同数据；PostgreSQL 服务与数据卷保持原配置。
