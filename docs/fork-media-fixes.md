# Fork 的媒体兼容修复与打包

本次保留现有自动探测触发方式。`PlaybackInfo` 和视频请求不会因为这些改动而启动新的后台探测；资料缺失时仍需要手动探测，或等待原有触发路径生成缓存。

修复内容：

- 手动探测先解析 STRM/云盘目标及请求头，再探测实际视频，并保存完整资料到现有 `media_probes`。
- Emby 详情和 PlaybackInfo 读取有效的已有探测缓存，返回真实轨道、容器、时长与大小；外挂字幕索引随真实轨道调整，保持获取地址正确。
- 电影不再包含季号、集号字段；真实剧集与第 0 季特别篇仍保留这些字段。
- 没有真实总时长时返回未知，不再根据播放进度估算 30 分钟；已有客户端上报的总时长可用于详情和播放响应。STRM 文本大小不再作为影片大小返回。
- 登录响应补充用户配置、权限及会话字段，成功登录立即返回实际登录时间，避免异步数据库更新时出现空日期。Lenna 是否还要求其他字段需要客户端实测确认。

没有新增数据库表或列；文件大小放在已有 JSON payload 内，旧缓存仍能读取。无需为这些改动重新登录网盘、重新生成全部 STRM 或重新刮削电影。

## 打包镜像

使用 GitHub Actions 的 **Publish Docker image** 工作流：

1. 选择包含修复的分支运行工作流。
2. `ref` 填写要构建的分支或提交，`version` 填写固定镜像标签，例如 `fixes-20261014`。
3. 产物为 `ghcr.io/ferrymania/nook:fixes-20261014`，支持 `linux/amd64` 和 `linux/arm64`。

该工作流不依赖 `version` 分支，也不自动连接服务器部署。只有 `ref=main` 才额外更新 `latest`，构建修复分支不会覆盖稳定标签。上游的自动版本递增/部署工作流在 fork 中跳过。

所有 Compose 模板的运行镜像与应用内更新目标共用 `MEBOX_IMAGE`，默认使用 `ghcr.io/ferrymania/nook:latest`。使用固定测试标签时，在部署目录的 `.env` 设置：

```dotenv
MEBOX_IMAGE=ghcr.io/ferrymania/nook:fixes-20261014
```

首次发布后，需要确认 GHCR 包对部署机器可见；私有包需要对应的拉取凭据。首次镜像发布完成前，上述 fork 镜像地址还不可用于拉取。

本地验证打包使用原有 Dockerfile，例如：

```sh
docker build --platform linux/amd64 --build-arg VERSION=fixes-20261014 -t nook:fixes-20261014 .
```

修改仓库名称不要求重命名 Go module、程序名或容器内路径；这些保持原有约定，便于继续合并上游更新。
