# Nook

<p align="center">
  <img src="web/public/brand/logo-192.png" width="80" height="80" alt="Nook Logo" />
</p>

面向 NAS 与家庭影音场景的自托管媒体中心，统一管理本地媒体、网盘 STRM 和远程 Emby 媒体库。

[使用文档](https://github.com/Ferrymania/nook/wiki) · [版本下载](https://github.com/Ferrymania/nook/releases) · [问题反馈](https://github.com/Ferrymania/nook/issues) · [English](README_EN.md)

## 核心功能

| 功能 | 说明 |
| --- | --- |
| 媒体库 | 电影、剧集、动漫等多种类型，多目录、海报墙与最近添加 |
| 刮削与整理 | TMDb、Bangumi 等元数据源，NFO、手动匹配与媒体规范命名整理 |
| 播放 | 网页播放、HLS 转码、字幕、弹幕、继续观看与收藏 |
| 第三方客户端 | 兼容 Emby/Jellyfin 客户端，使用 Nook 账号连接 |
| 网盘与挂载 | OpenList、115、WebDAV、STRM 同步与远程 Emby 挂载 |
| 管理 | 多用户权限、播放配置、任务队列、日志与系统设置 |

## 快速部署

推荐使用 Docker Compose。以下模板会同时启动 Nook 和 PostgreSQL：

```bash
mkdir -p nook && cd nook
curl -fsSL https://raw.githubusercontent.com/Ferrymania/nook/main/docker-compose.yml -o docker-compose.yml
# 按需修改 docker-compose.yml 中的宿主机媒体目录，然后启动：
docker compose up -d
```

- 访问地址：`http://服务器IP:18080`
- 初始账号：`admin` / `admin123`，首次登录后修改密码。
- 镜像：`ghcr.io/ferrymania/nook:latest`，支持 `linux/amd64` 和 `linux/arm64`。
- 已有 PostgreSQL：查看 [部署指南](https://github.com/Ferrymania/nook/wiki/Deployment)。

升级前保留现有数据库连接参数和数据挂载。部署、更新与备份步骤见下方文档。

## 使用文档

| 文档 | 内容 |
| --- | --- |
| [部署指南](https://github.com/Ferrymania/nook/wiki/Deployment) | Compose 模板、已有 PostgreSQL 与二进制部署 |
| [路径与媒体库](https://github.com/Ferrymania/nook/wiki/Paths-and-Libraries) | Docker 路径映射、下载目录与媒体库目录迁移 |
| [首次使用](https://github.com/Ferrymania/nook/wiki/Getting-Started) | 添加媒体、刮削、播放设置与第三方客户端 |
| [更新与备份](https://github.com/Ferrymania/nook/wiki/Updates-and-Backups) | 版本检查、更新镜像、数据备份与旧安装升级 |
| [常见问题](https://github.com/Ferrymania/nook/wiki/Troubleshooting) | 扫库、整理、硬链接、日志与客户端连接排查 |
| [Telegram 通知](docs/telegram-notifications.md) | 入库海报通知、STRM 同步控制与发送规则 |
| [开发与构建](https://github.com/Ferrymania/nook/wiki/Development) | 本地开发、数据库测试、二进制构建与发布流程 |

## 贡献与致谢

提交 Issue 或 PR 前，请阅读 [贡献规范](CONTRIBUTING.md) 与 [安全策略](SECURITY.md)。媒体兼容修复记录见 [维护说明](docs/fork-media-fixes.md)。

Nook 基于 [MeBox](https://github.com/truewhile/MeBox) 维护，其上游为 [MediaStationGo](https://github.com/ShukeBta/MediaStationGo)；部分网盘同步与整理实现参考了 [qmediasync](https://github.com/qicfan/qmediasync)。感谢这些项目的贡献。

## 许可证

[GPL-3.0](LICENSE)
