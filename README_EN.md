# Nook

<p align="center">
  <img src="web/public/brand/logo-192.png" width="80" height="80" alt="Nook Logo" />
</p>

A self-hosted media center for NAS and home theaters, bringing local media, cloud STRM files, and remote Emby libraries together.

[Documentation (中文)](https://github.com/Ferrymania/nook/wiki) · [Releases](https://github.com/Ferrymania/nook/releases) · [Issues](https://github.com/Ferrymania/nook/issues) · [中文](README.md)

## Features

| Feature | Includes |
| --- | --- |
| Libraries | Movies, series, anime, multiple directories, poster browsing, and recently added media |
| Metadata and organization | TMDb, Bangumi, NFO, manual matching, and download-folder organization |
| Playback | Web player, HLS transcoding, subtitles, danmaku, continue watching, and favorites |
| Third-party clients | Emby/Jellyfin client compatibility using your Nook account |
| Cloud and remote media | OpenList, 115, WebDAV, STRM sync, and remote Emby mounts |
| Administration | User permissions, per-user playback settings, task queues, logs, and settings |

## Quick Start

Use Docker Compose. This template starts Nook and PostgreSQL together:

```bash
mkdir -p nook && cd nook
curl -fsSL https://raw.githubusercontent.com/Ferrymania/nook/main/docker-compose.yml -o docker-compose.yml
# Adjust host media paths in docker-compose.yml as needed, then start:
docker compose up -d
```

- Open `http://SERVER_IP:18080`.
- Initial account: `admin` / `admin123`. Change the password after signing in.
- Image: `ghcr.io/ferrymania/nook:latest`, available for `linux/amd64` and `linux/arm64`.
- For an existing PostgreSQL instance, see the [deployment guide](https://github.com/Ferrymania/nook/wiki/Deployment).

Keep existing database credentials and data mounts when upgrading. Detailed setup and maintenance instructions are in the Wiki below (in Chinese).

## Documentation

| Guide | Covers |
| --- | --- |
| [Deployment](https://github.com/Ferrymania/nook/wiki/Deployment) | Compose templates, external PostgreSQL, and binary deployment |
| [Paths and libraries](https://github.com/Ferrymania/nook/wiki/Paths-and-Libraries) | Docker mounts, download paths, and library directory migration |
| [First-time setup](https://github.com/Ferrymania/nook/wiki/Getting-Started) | Media, metadata, playback settings, and clients |
| [Updates and backups](https://github.com/Ferrymania/nook/wiki/Updates-and-Backups) | Version checks, image updates, backups, and legacy installations |
| [Troubleshooting](https://github.com/Ferrymania/nook/wiki/Troubleshooting) | Scanning, organization, hardlinks, logs, and client connections |
| [Development](https://github.com/Ferrymania/nook/wiki/Development) | Local development, database tests, builds, and releases |

## Contributing and Acknowledgements

Read [CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md) before reporting an issue or submitting a PR. Media compatibility notes are in [the maintenance guide](docs/fork-media-fixes.md).

Nook is maintained from [MeBox](https://github.com/truewhile/MeBox), which builds on [MediaStationGo](https://github.com/ShukeBta/MediaStationGo). Some cloud synchronization and organization work also references [qmediasync](https://github.com/qicfan/qmediasync). Thank you to these projects.

## License

[GPL-3.0](LICENSE)
