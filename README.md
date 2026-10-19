# Nook

本 fork 的媒体兼容修复和镜像打包方式见 [维护说明](docs/fork-media-fixes.md)。

<p align="center">
  <img src="web/public/brand/logo-192.png" width="96" height="96" alt="Nook Logo" />
</p>

<h3 align="center">面向 NAS 与家庭影音场景的私人媒体中心</h3>

<p align="center">
  <strong>媒体库 · 刮削整理 · 网盘 STRM · 兼容 Emby/Jellyfin 客户端 · 远程 Emby 挂载 · 多用户权限 · Docker 一键部署</strong>
</p>

<p align="center">
  <a href="#项目简介">项目简介</a> ·
  <a href="#快速开始">快速开始</a> ·
  <a href="#部署档位">部署档位</a> ·
  <a href="#鸣谢">鸣谢</a> ·
  <a href="#开发构建">开发构建</a> ·
  <a href="README_EN.md">English</a> ·
  <a href="CONTRIBUTING.md">贡献规范</a> ·
  <a href="https://github.com/Ferrymania/nook">项目主页</a>
</p>

<p align="center">
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white" />
  <img alt="React" src="https://img.shields.io/badge/React-18-61DAFB?style=flat-square&logo=react&logoColor=111827" />
  <img alt="Docker" src="https://img.shields.io/badge/Docker-ready-2496ED?style=flat-square&logo=docker&logoColor=white" />
  <img alt="License" src="https://img.shields.io/badge/License-GPL--3.0-blue?style=flat-square" />
</p>

---

## 项目简介

**Nook** 是一个自托管私人媒体管理系统，适合 NAS、小主机、家庭共享和多端播放场景。本项目由 [MediaStationGo](https://github.com/ShukeBta/MediaStationGo) fork 并持续二开维护，在保留「一套服务覆盖网页、手机、电视与第三方播放器」思路的同时，围绕网盘播放、任务队列、远程挂载和权限体系做了大量增强。

你可以把 Nook 理解为：

- 一个带现代 Web UI 的**媒体库后台**
- 一个兼容 Emby/Jellyfin 客户端的**协议网关**
- 一个连接本地硬盘、下载目录与网盘存储的**整理与播放入口**

### 核心能力

| 模块 | 说明 |
| --- | --- |
| **媒体库** | 电影、电视剧、动漫、综艺、音乐与自定义库；多根目录、扫库、海报墙、继续观看 |
| **元数据刮削** | TMDb、Bangumi、Douban、TheTVDB、Fanart 等；支持 NFO、手动匹配、刮削队列 |
| **播放** | 网页播放器、HLS 转码、弹幕、字幕、播放配置档、观看历史与收藏 |
| **Emby/Jellyfin 客户端兼容** | 内置完整 Emby 服务端协议实现：Infuse、SenPlayer、Fileball、Emby/Jellyfin 官方客户端等可直接把本服务当作 Emby 服务器添加，使用 Nook 账号登录，海报墙、进度同步、多用户无缝衔接 |
| **远程 Emby 挂载** | 将远程 Emby 媒体库挂载到本地界面统一浏览（无需单独开 Emby 客户端） |
| **网盘与 STRM** | OpenList、CloudDrive2、115、WebDAV 等；STRM 同步、上传/下载队列、直链/302 播放 |
| **下载与整理** | 下载目录定时自动整理（智能分类、自动注册媒体库）、文件管理器（复制/移动/硬链/软链） |
| **用户与权限** | 管理员/普通用户、有效期、成人内容开关、播放配置 PIN、细粒度操作权限 |
| **运维能力** | 统一任务队列、存储统计、DLNA 投屏、系统设置与日志 |

### 技术栈

- **后端**：Go · Gin · GORM · PostgreSQL · 可选 Redis
- **前端**：React 18 · Vite · TypeScript · Tailwind CSS · Zustand
- **部署**：Docker Compose 多档模板，支持 amd64 / arm64 镜像与单文件可执行发布

---

## 快速开始

推荐使用 Docker Compose。根据是否已有 PostgreSQL 选择一份完整模板，直接编辑 YAML，无需 `.env`：

| 数据库情况 | 配置文件 | 启动的服务 |
| --- | --- | --- |
| 没有 PostgreSQL | [docker-compose.yml](docker-compose.yml) | Nook + 新建 PostgreSQL |
| 已有 PostgreSQL | [docker-compose.external-postgres.yml](docker-compose.external-postgres.yml) | 只启动 Nook，连接已有数据库 |

```bash
mkdir -p Nook && cd Nook

# 没有 PostgreSQL：自动启动 Nook + PostgreSQL
curl -fsSL https://raw.githubusercontent.com/Ferrymania/nook/main/docker-compose.yml -o docker-compose.yml

# 已有 PostgreSQL：改用下面的下载地址，并先修改 YAML 中的 NOOK_DATABASE_DSN
# curl -fsSL https://raw.githubusercontent.com/Ferrymania/nook/main/docker-compose.external-postgres.yml -o docker-compose.yml


docker compose up -d
```

默认挂载 Compose 文件同目录下的 `media`、`downloads` 文件夹，不存在时 Docker Compose 会自动创建。使用已有媒体目录时，修改 YAML 中挂载左侧的宿主机路径。

浏览器访问：

```text
http://服务器IP:18080
```

默认账号：`admin` / `admin123`（首次登录后请立即修改密码）

> 💡 **Emby 用户无缝切换**：Nook 完整兼容 Emby/Jellyfin 客户端协议。手机、电视、平板上的 Infuse、SenPlayer、Fileball、Emby/Jellyfin 官方客户端，直接按「添加 Emby 服务器」填入 `http://服务器IP:18080`，用 Nook 账号登录即可，无需改变原有使用习惯。

镜像地址：

```text
ghcr.io/ferrymania/nook:latest
```

已有安装升级时，请保留原 PostgreSQL 用户、数据库、密码与数据目录；旧 `MEBOX_*` 配置仍兼容。详见 [升级说明](docs/fork-media-fixes.md#nook-命名与旧安装升级)。

---

## 部署档位

按机器资源选择档位。每份 Compose 文件均可单独使用，**不要**叠加多个 `-f`。

| 档位 | 配置文件 | 组件 | 适合场景 |
| --- | --- | --- | --- |
| 默认 | `docker-compose.yml` | Nook + 新建 PostgreSQL | 没有 PostgreSQL，大多数家庭 NAS |
| 已有 PostgreSQL | `docker-compose.external-postgres.yml` | Nook + 外部 PostgreSQL | 复用已有数据库实例 |
| Redis 可选 | `docker-compose.standard.yml` | + Redis | 多用户、Emby 客户端频繁刷新、首页/列表访问多 |

### 没有 PostgreSQL

- 使用 `docker-compose.yml`，会启动 Nook 和 PostgreSQL 两个服务。
- 主库在 `./postgres`，配置与密钥在 `./data`，两者都要备份。

### 已有 PostgreSQL

使用 `docker-compose.external-postgres.yml`，只启动 Nook。在 YAML 中填写实际连接参数：

```yaml
NOOK_DATABASE_TYPE: postgres
NOOK_DATABASE_DSN: "postgres://数据库用户:数据库密码@数据库地址:5432/数据库名?sslmode=disable"
```

数据库和用户需事先创建，并允许 Nook 容器连接。用户名、密码中的特殊字符需进行 URL 编码，`sslmode` 按数据库要求设置。

- 数据库在宿主机：地址使用 `host.docker.internal`，模板已配置宿主机网关。
- 数据库在其他服务器：地址使用实际 IP 或域名。
- 数据库在其他 Docker 容器：按模板末尾的注释加入其已有网络，地址使用数据库服务名。

`127.0.0.1` 指向 Nook 容器自身。外部 PostgreSQL 的数据与备份由原数据库部署管理，Nook 目录中仍需保留和备份 `./data`。

若 PostgreSQL 本来就在旧 Nook / MeBox 的同一份 Compose 中，升级时继续保留原数据库服务、连接参数及数据挂载即可；不必拆成外部数据库。

播放方式、转码和硬件加速统一在网页「系统设置 → 常规」中管理，Compose 模板不再重复设置转码参数。网页保存的配置会在重启后继续生效。只需一个「客户端直连解码」开关：开启时由客户端解码，服务器不转码；关闭时允许服务器按需转码。客户端解码开启期间，下方转码参数暂不生效，但会保留原值。

### 必须备份与可重建

| 路径 | 说明 |
| --- | --- |
| `./data` | JWT 密钥和运行配置 |
| `./postgres` | 自带 PostgreSQL 的主库；外部 PostgreSQL 在原部署处备份 |
| `./cache` | 海报/转码缓存，可重建 |
| `./redis` | 热缓存，可重建 |

### 版本检查

网页「系统设置 → 关于」会自动检查最新正式版本，显示更新说明，并提供手动检查和复制更新命令。检查通过服务器访问 GitHub，不需要 Docker socket；正常结果缓存 30 分钟，手动检查和失败重试间隔至少 1 分钟。开发版或无法识别的版本号会提示手动核对，网络失败不会显示为已是最新版。

### 更新镜像

在 Compose 文件所在目录执行以下命令；通过容器管理面板部署时，也可以在面板中拉取新镜像并重建 Nook 容器。

```bash
docker compose pull nook
docker compose up -d --no-deps nook
```

日常更新只拉 `nook` 服务即可，不要随意 `docker compose pull` 升级 PostgreSQL/Redis 基础镜像。
更新后继续使用原有数据挂载。默认部署不向 Nook 容器挂载 Docker socket，网页目前没有一键更新入口。

---

## 路径映射

Docker 部署最常见的问题是路径填错。记住：

- `volumes` **左侧**是宿主机真实路径，**右侧**是容器内路径
- 网页后台添加媒体库时，应填写**容器内**路径（如 `/media/电影`）
- 下载器与 Nook 应使用一致的容器路径（如 `/downloads`），通常不需要额外的路径换算参数

NAS 示例：

```yaml
volumes:
  - /vol1/1000/Media:/media
  - /vol1/1000/Downloads:/downloads
```

已有媒体库更换目录时，在「管理媒体库」中浏览新路径并保存，选择：

- **迁移原有内容**：同一批文件换位置。先按相对路径和文件大小检查（STRM 核对播放地址），全部通过后更新索引，保留媒体 ID、刮削资料、收藏与观看进度。缺失、冲突或检查后目录发生变化时，阻止保存；数据库更新失败会整体回滚。
- **改用新目录**：换成另一批内容。确认后只清除该来源的旧媒体索引、收藏、进度及关联记录，再扫描新目录；同库的其他来源不受影响。

两种操作都不会移动、复制、重命名或删除实际文件。请先自行准备文件或调整 Docker 挂载，再修改 Nook 路径。正在扫描或刮削时请等待完成后操作。迁移检查不比较整个视频的内容哈希，无法识别同名同大小但内容不同的文件。

高级兼容：如果下载器只能返回宿主机路径，仍可按需添加 `NOOK_MEDIA_DIR` / `NOOK_MEDIA_CONTAINER_DIR`、`NOOK_DOWNLOAD_DIR` / `NOOK_DOWNLOAD_CONTAINER_DIR` 作为旧路径前缀换算。它们不创建 Docker 挂载，也不替代上述目录迁移。

---

## 首次使用建议

1. **创建媒体库** → 填写 `/media/...` → 执行扫库
2. **配置元数据源** → 系统设置中添加 TMDb、Bangumi 等 API
3. **（可选）配置下载目录自动整理** → 文件管理中将下载目录设为整理源，下载完成后自动分类入库
4. **（可选）配置网盘账号** → STRM 管理中添加 OpenList / 115 / WebDAV 等
5. **第三方播放器** → 以 Emby 服务器添加 `http://服务器IP:18080`，使用 Nook 账号登录

刮削时会通过 ffprobe 提取并保存媒体技术信息，STRM 会解析到实际视频。已有有效缓存时跳过，远程探测并发为 1、单次预算 30 秒；失败冷却 6 小时，不影响影片元数据匹配结果。手动探测按钮可绕过冷却。PlaybackInfo 和 MediaSegments 只读取已保存的技术信息，不在播放时启动探测。

---

## 常见问题

**扫库或入库很慢？**  
先确认路径映射与数据库档位。网盘扫描还受接口限速与目录规模影响；多用户高频访问可按需启用 Redis。

**下载目录文件没有被自动整理？**  
确认下载目录已通过 `volumes` 挂进容器，且 `NOOK_DOWNLOAD_*` 环境变量对应正确。Nook 负责目录整理入库，qBittorrent 等下载器按普通软件自行部署即可。

**硬链接失败（cross-device link）？**  
硬链接要求源与目标在同一文件系统/子卷；跨盘、跨 btrfs 子卷或网盘挂载时请改用复制或软链接。

**日志保留时间太短？**

应用日志默认保存在 `./data/logs`，单文件上限为 20 MB，每类日志最多保留 5 份轮转备份，轮转时清理超过 30 天的旧备份。Compose 模板使用内置默认值，不再重复列出应用日志参数；容器 stdout 日志仍限制为 `20m x 3`。

排障时可在 Nook 服务的 `environment` 中临时添加 `NOOK_LOGGING_LEVEL: debug`，或按需添加 `NOOK_LOGGING_MAX_SIZE_MB`、`NOOK_LOGGING_MAX_BACKUPS`、`NOOK_LOGGING_MAX_AGE_DAYS` 覆盖默认值；修改后执行 `docker compose up -d --no-deps nook` 重新创建容器生效。Docker 日志限制仍通过 `logging.options.max-size/max-file` 调整。

**第三方播放器连不上？**  
确认地址为 `http://IP:18080`，使用 Nook 用户账号；反代部署需正确配置外部 URL 与 HTTPS 头。

---

## 开发构建

后端通过 `go:embed` 嵌入 `web/dist`，**编译前必须先构建前端**。
前端构建要求 Node.js `20.19+` 或 `22.12+`。

```bash
npm --prefix web ci
npm --prefix web run build

go test ./...
go run ./cmd/server          # http://127.0.0.1:8080
npm --prefix web run dev     # http://127.0.0.1:3000
```

CI 会在 Release 中提供 Windows / Linux 的 amd64、arm64，以及 macOS Apple Silicon（arm64）的单文件可执行程序。

Windows 本地打包：

```powershell
.\scripts\build-windows.ps1 -Version dev
```

Windows 可执行程序使用项目 Logo，不显示控制台窗口；启动后会常驻系统托盘。托盘菜单可打开 Nook、切换开机自启、查看日志、重启或退出。

---

## 鸣谢

Nook 基于 [MeBox](https://github.com/truewhile/MeBox) fork 维护；MeBox 的上游为 [MediaStationGo](https://github.com/ShukeBta/MediaStationGo)。感谢上游项目在媒体库架构、Emby 协议兼容和自托管体验上的奠基工作。

项目中许多网盘同步、STRM 与媒体整理相关的设计与实现，也参考了 [qmediasync](https://github.com/qicfan/qmediasync)。感谢该项目的思路与实践经验。

---

## 贡献与反馈

提交 Issue 或 Pull Request 前，请阅读 [贡献规范](CONTRIBUTING.md) 与 [安全策略](SECURITY.md)。

- Bug 请附部署方式、复现步骤与相关日志
- 功能建议请说明使用场景与期望行为
- PR 请从独立分支发起，提交前运行 `go test ./...` 与 `npm --prefix web run build`

---

## Star History

<a href="https://www.star-history.com/?repos=Ferrymania%2Fnook&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=Ferrymania/nook&type=date&theme=dark&legend=top-left" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=Ferrymania/nook&type=date&legend=top-left" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=Ferrymania/nook&type=date&legend=top-left" />
 </picture>
</a>

---

## 许可证

本项目采用 [GPL-3.0](LICENSE) 许可证。

---
