# 修复 NFO 类型混入标签和片商

新版导入只把 NFO 的 `<genre>` 保存到 `media.genres`，不限制自定义类型。`<tag>`、`<studio>`、`<maker>`、`<publisher>`、`<label>` 不再混入类型，源 NFO 不会被修改。中英文类型不在本次强制合并。

已有数据可以用独立维护命令定向修复，无需重新刮削或探测视频。命令使用当前配置连接 PostgreSQL，但不执行表结构迁移，也不启动 HTTP 服务、定时任务或其他后台服务。不会自动在应用启动时执行。

## 先生成只读报告

使用包含本修复的新版本，在与 Nook 相同的配置和媒体路径环境中执行。将 `<媒体库ID>` 替换为媒体库页面 URL 中的 ID：

```sh
docker compose run --rm --no-deps --entrypoint nook nook repair-genres \
  --library '<媒体库ID>' --report /data/genre-repair.json
```

报告文件默认权限为 `0600`，不会覆盖已有同名文件。报告包含 `changes`（媒体 ID、路径、NFO 路径、修改前后类型）和 `skipped`（需人工核对的条目及原因），数据库不变。报告可能包含私人目录名称，请妥善保存。

只有满足以下条件才会生成修改项：

- 可读取完整、有效的本地 NFO，并且有非空 `<genre>`。
- 数据库现值与该 NFO 按旧版「类型 + 标签 + 片商」导入的结果完全一致（忽略顺序、重复、大小写和分隔符差异）。
- 提取出的类型与现值不同。

剧集优先使用整剧 NFO，没有整剧类型时才按原有来源关系考虑单集 NFO。无 NFO、损坏或缺少类型、网盘远端路径以及疑似手动改过的类型均跳过。若旧的错误值已被导出成 NFO `<genre>`，无法可靠区分，命令也不会猜测删除，应从可信元数据来源单独核对。

## 核对后应用

先备份数据库，检查报告中的修改范围。暂停 Nook，避免后台扫描或刮削同时写入，并在同一目录执行：

```sh
docker compose stop nook
docker compose run --rm --no-deps --entrypoint nook nook repair-genres \
  --apply /data/genre-repair.json
docker compose up -d nook
```

应用只修改 `media.genres`，不修改标题、简介、外部 ID、影片文件、观看记录或表结构。所有修改在同一事务内执行；任一媒体自报告生成后已修改或删除，会回滚整个事务，需重新生成报告。原值保留在报告的 `before` 中。重启用于清除已有的内存元数据缓存，随后检查网页类型筛选和 Emby 客户端。

二进制部署对应使用 `nook repair-genres ...`，并保证工作目录、环境变量和媒体目录与正常服务一致。
