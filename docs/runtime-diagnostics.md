# 运行诊断

在「系统设置 → 关于 → 运行诊断」展开面板，点击「开启 15 分钟」。该状态仅保存在当前进程内；15 分钟后过期，也可提前关闭，重启后默认关闭。重复点击开启不会延长尚未到期的窗口。

诊断使用 Nook 现有 HTTP/HTTPS 服务和管理员登录鉴权。无需添加端口、修改 Compose 或重启服务。未登录返回 401，普通用户返回 403；管理员在关闭期间请求分析文件返回 404。

面板提供四种下载：

| 文件 | 用途 |
| --- | --- |
| 内存快照 `heap` | Go 堆中仍在使用的采样内存及分配调用栈 |
| 分配快照 `allocs` | 累计分配内存及分配调用栈 |
| 协程快照 `goroutine` | 当前 Go 协程及调用栈 |
| CPU 采集 `profile` | 默认采集 10 秒的 CPU 执行采样 |

同时只允许一个采集请求。CPU 时长允许 1–30 秒；关闭诊断、窗口到期或客户端断开会提前结束 CPU 采样。已开始输出的快照可能仍会完成，提前结束的 CPU 文件包含已采集到的数据。

内存快照默认不强制执行 GC，不开放 `gc`、`debug` 或快照的 `seconds` 参数；这些参数会被忽略。第一版仅提供上述四个分析入口，不公开 cmdline、trace、symbol、block 或 mutex。

文件直接返回浏览器，不写入服务器磁盘。文件包含程序调用栈等内部信息，应仅分享给可信的排查人员。

## 离线分析

电脑安装 Go 后，对已下载的文件执行：

```bash
go tool pprof -http=127.0.0.1:8081 ./nook-heap-YYYYMMDDTHHMMSSZ.pprof
```

`8081` 是电脑上分析页面的端口，可自行更换。堆内存采样不等同于容器全部内存；文件缓存、程序映射和内核内存仍需结合 `docker stats`、cgroup 和进程 RSS 查看。

## 管理员接口

以下接口均要求现有管理员会话或 `Authorization: Bearer <管理员令牌>`，不要把令牌写进 URL。

| 方法 | 路径 |
| --- | --- |
| GET | `/api/admin/diagnostics/pprof/status` |
| POST | `/api/admin/diagnostics/pprof/enable` |
| POST | `/api/admin/diagnostics/pprof/disable` |
| GET | `/api/admin/diagnostics/pprof/heap` |
| GET | `/api/admin/diagnostics/pprof/allocs` |
| GET | `/api/admin/diagnostics/pprof/goroutine` |
| GET | `/api/admin/diagnostics/pprof/profile?seconds=10` |

状态包含 `enabled`、开启时的 `expires_at`（UTC）和 `capturing`。采集互斥返回 409，无效 CPU 时长返回 400。所有诊断响应设置 `Cache-Control: no-store`；二进制分析文件不再经过 API gzip，避免重复压缩。
