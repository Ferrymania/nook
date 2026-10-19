# Nook

<p align="center">
  <img src="web/public/brand/logo-192.png" width="96" height="96" alt="Nook Logo" />
</p>

<h3 align="center">A self-hosted media center for NAS and home theater</h3>

<p align="center">
  <strong>Libraries · Metadata · Cloud STRM · Emby/Jellyfin client compatible · Remote Emby mounts · Multi-user · Docker-first</strong>
</p>

<p align="center">
  <a href="README.md">中文</a> ·
  <a href="#overview">Overview</a> ·
  <a href="#quick-start">Quick Start</a> ·
  <a href="#deployment-tiers">Deployment</a> ·
  <a href="#acknowledgements">Acknowledgements</a> ·
  <a href="#development">Development</a> ·
  <a href="https://github.com/Ferrymania/nook">Project home</a>
</p>

<p align="center">
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white" />
  <img alt="React" src="https://img.shields.io/badge/React-18-61DAFB?style=flat-square&logo=react&logoColor=111827" />
  <img alt="Docker" src="https://img.shields.io/badge/Docker-ready-2496ED?style=flat-square&logo=docker&logoColor=white" />
  <img alt="License" src="https://img.shields.io/badge/License-GPL--3.0-blue?style=flat-square" />
</p>

---

## Overview

**Nook** is a self-hosted private media management system for NAS, mini PCs, family sharing, and multi-device playback. This repository is a maintained fork of [MediaStationGo](https://github.com/ShukeBta/MediaStationGo), extended with stronger cloud playback, task queues, remote mounts, and permission controls.

In practice, Nook gives you:

- A modern **web media library**
- An **Emby/Jellyfin-compatible protocol gateway** for third-party players
- A single panel for **local disks, download folders, and cloud storage**

### Key capabilities

| Area | Highlights |
| --- | --- |
| **Libraries** | Movies, TV, anime, variety, music, custom libraries; multi-root scanning; poster wall; continue watching |
| **Metadata** | TMDb, Bangumi, Douban, TheTVDB, Fanart, NFO import, manual matching, scrape queue |
| **Playback** | Web player, HLS transcoding, danmaku, subtitles, play profiles, history and favourites |
| **Emby/Jellyfin client compatible** | Full Emby server protocol implementation: Infuse, SenPlayer, Fileball, and official Emby/Jellyfin clients can add Nook as an Emby server and sign in with Nook accounts — poster walls, watch progress, and multi-user work out of the box |
| **Remote Emby mounts** | Browse remote Emby libraries inside Nook without a separate Emby client |
| **Cloud & STRM** | OpenList, CloudDrive2, 115, WebDAV; STRM sync; upload/download queues; direct or 302 playback |
| **Downloads & organize** | Scheduled download-folder organization (smart classification, auto library registration), file manager (copy/move/hardlink/symlink) |
| **Users & permissions** | Admin/regular users, expiry, NSFW toggle, play-profile PIN, granular permissions |
| **Operations** | Unified task queue, storage stats, DLNA casting, settings and logs |

### Tech stack

- **Backend**: Go, Gin, GORM, PostgreSQL, optional Redis
- **Frontend**: React 18, Vite, TypeScript, Tailwind CSS, Zustand
- **Deployment**: Standalone Docker Compose templates, amd64/arm64 images, single-binary releases

---

## Quick Start

Docker Compose is the recommended path. Choose a standalone template based on whether you already have PostgreSQL. Edit the YAML directly; no `.env` is required.

| Database setup | File | Services started |
| --- | --- | --- |
| No PostgreSQL yet | [docker-compose.yml](docker-compose.yml) | Nook + a new PostgreSQL instance |
| Existing PostgreSQL | [docker-compose.external-postgres.yml](docker-compose.external-postgres.yml) | Nook only, connected to your existing database |

```bash
mkdir -p Nook && cd Nook
mkdir -p media downloads

# No PostgreSQL yet: start Nook + PostgreSQL
curl -fsSL https://raw.githubusercontent.com/Ferrymania/nook/main/docker-compose.yml -o docker-compose.yml

# Existing PostgreSQL: use this URL and edit NOOK_DATABASE_DSN before starting
# curl -fsSL https://raw.githubusercontent.com/Ferrymania/nook/main/docker-compose.external-postgres.yml -o docker-compose.yml


docker compose up -d
```

Open:

```text
http://SERVER_IP:18080
```

Default login: `admin` / `admin123` — change the password immediately.

> 💡 **Seamless for Emby users**: Nook fully implements the Emby/Jellyfin client protocol. Infuse, SenPlayer, Fileball, and official Emby/Jellyfin apps on phones, TVs, and tablets can add it as an Emby server at `http://SERVER_IP:18080` and sign in with Nook accounts — no change to your existing workflow.

Image:

```text
ghcr.io/ferrymania/nook:latest
```

For existing installations, keep the PostgreSQL credentials and data directories. Legacy `MEBOX_*` settings remain supported; see the [upgrade notes](docs/fork-media-fixes.md#nook-命名与旧安装升级).

---

## Deployment tiers

Pick one compose file. Do **not** stack multiple `-f` files.

| Tier | File | Stack | Best for |
| --- | --- | --- | --- |
| Default | `docker-compose.yml` | Nook + a new PostgreSQL instance | Most home NAS deployments without PostgreSQL |
| Existing PostgreSQL | `docker-compose.external-postgres.yml` | Nook + external PostgreSQL | Reuse an existing database instance |
| Optional Redis | `docker-compose.standard.yml` | + Redis | Multi-user, frequent Emby client refreshes |

### No PostgreSQL yet

- `docker-compose.yml` starts both Nook and PostgreSQL.
- Back up both `./postgres` (primary DB) and `./data` (secrets and runtime files).

### Existing PostgreSQL

Use `docker-compose.external-postgres.yml`, which starts Nook only. Fill in your actual connection settings:

```yaml
NOOK_DATABASE_TYPE: postgres
NOOK_DATABASE_DSN: "postgres://DB_USER:DB_PASSWORD@DB_HOST:5432/DB_NAME?sslmode=disable"
```

Create the database and user beforehand, and allow connections from the Nook container. URL-encode special characters in credentials and set `sslmode` as required by your database.

- Host-installed PostgreSQL: use `host.docker.internal`; the template includes the host gateway mapping.
- Another server: use its reachable IP address or hostname.
- Another Docker container: uncomment the existing-network configuration at the end of the template and use the database service name.

`127.0.0.1` refers to the Nook container itself. Back up external PostgreSQL through its original deployment, and keep backing up Nook's `./data` directory.

If PostgreSQL is already part of an older Nook / MeBox Compose deployment, keep that service, its credentials, and data mounts when upgrading. You do not need to separate it into an external database.

### Backup

| Path | Notes |
| --- | --- |
| `./data` | JWT secret and runtime config |
| `./postgres` | Bundled PostgreSQL primary DB; back up external PostgreSQL at its original deployment |
| `./cache`, `./redis` | Rebuildable |

### Version checks

**Settings → About** automatically checks the latest stable GitHub release, shows release notes, and offers manual checks and a copyable update command. Checks run on the server without a Docker socket. Results are cached for 30 minutes, with a one-minute minimum interval for manual refreshes and retries after failures. Development or unrecognized versions require manual comparison; network failures are never reported as up to date.

### Update

```bash
docker compose pull nook
docker compose up -d --no-deps nook
```

---

## Path mapping

The most common Docker mistake is mixing host paths with container paths.

- Left side of `volumes` = real host/NAS path
- Right side = container path; use `/media/...` in the web UI
- Use matching container paths in the downloader and Nook, such as `/downloads`; extra path-mapping environment variables are normally unnecessary

Example:

```yaml
volumes:
  - /vol1/1000/Media:/media
  - /vol1/1000/Downloads:/downloads
```

To change an existing library source, browse to the new path in library management and save. Choose **Migrate existing content** to validate relative paths and file sizes (STRM playback targets) and preserve IDs, metadata, favorites, and progress. Missing files or conflicts block saving; confirmation checks again and database failures roll back the entire update. This is not a full content-hash comparison.

Choose **Use a new directory** for different content. Confirmation removes only that source's old index, favorites, progress, and related records, then scans the new directory. Other sources are unaffected. Neither option moves, copies, renames, or deletes actual files. Prepare files or Docker mounts first; wait for scans/scraping to finish before changing a path.

Advanced compatibility overrides remain available when a downloader returns host paths: `NOOK_MEDIA_DIR` / `NOOK_MEDIA_CONTAINER_DIR` and `NOOK_DOWNLOAD_DIR` / `NOOK_DOWNLOAD_CONTAINER_DIR`. These translate legacy path prefixes; they do not create mounts or migrate indexed records.

---

## First-time setup

1. Create a library with a container path such as `/media/Movies`, then scan
2. Add metadata providers (TMDb, Bangumi, etc.) in system settings
3. Optionally set up download-folder auto-organization under file management so finished downloads land in the right library
4. Optionally configure cloud accounts under STRM management
5. Add the server in Emby-compatible players at `http://SERVER_IP:18080` using Nook credentials

Scraping also extracts and saves technical media information with ffprobe, including STRM targets. Valid cached results are reused, remote probes run one at a time with a 30-second budget, and failures have a six-hour cooldown without failing the metadata match. The manual probe button bypasses the cooldown. PlaybackInfo and MediaSegments only read saved technical information and do not start probes during playback.

---

## FAQ

**Library scan is slow**  
Check path mapping and DB tier. Cloud scans also depend on API limits and folder size.

**Downloaded files are not organized**  
Ensure the download directory is mounted into the container and env vars match. Nook handles folder organization; run qBittorrent or any downloader yourself as a regular app.

**Hardlink fails with cross-device link**  
Hardlinks require the same filesystem/subvolume; use copy or symlink across disks or cloud mounts.

**External player cannot connect**  
Use `http://IP:18080` and a Nook user account; reverse proxies need correct external URL and HTTPS headers.

---

## Development

The backend embeds `web/dist` via `go:embed`. Build the frontend first.

```bash
npm --prefix web ci
npm --prefix web run build

go test ./...
go run ./cmd/server
npm --prefix web run dev
```

Release builds ship single-file binaries for Windows, Linux, and macOS on amd64 and arm64.

Build the Windows executable locally:

```powershell
.\scripts\build-windows.ps1 -Version dev
```

The Windows executable uses the project logo and runs without a console window. It stays in the notification area, with menu actions for opening Nook, toggling auto-start, viewing logs, restarting, and exiting.

---

## Acknowledgements

Nook is maintained as a fork of [MeBox](https://github.com/truewhile/MeBox), which builds on [MediaStationGo](https://github.com/ShukeBta/MediaStationGo). Thank you to the upstream project for the media-library architecture, Emby-protocol compatibility, and self-hosted foundation.

Many cloud sync, STRM, and media-organization ideas in this project were also informed by [qmediasync](https://github.com/qicfan/qmediasync). Thank you for the reference implementation and design patterns.

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md) before opening issues or pull requests.

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

## License

This project is licensed under [GPL-3.0](LICENSE).
