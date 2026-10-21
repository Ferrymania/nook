# 贡献规范

感谢你愿意帮助 Nook 变得更稳定。这个项目主要面向 NAS、Docker 部署、媒体库整理、订阅下载和多端播放场景；提交 Issue 或 Pull Request 时，请尽量提供可复现、可验证的信息。

## Issue 提交规范

提交 Issue 前，请先确认：

- 已搜索现有 Issues，避免重复提交同一个问题。
- 使用的是最新镜像、最新主分支，或已说明当前版本号 / 镜像摘要。
- 如果是部署或运行问题，已附上部署方式和关键配置。

### Bug Report 必填信息

- 问题现象：实际发生了什么，是否稳定复现。
- 期望行为：你认为正确结果应该是什么。
- 复现步骤：从哪个页面、点击什么、填写什么、触发什么任务。
- 部署方式：Docker 第一档 / 第二档 / 第三档、裸机运行、反代方式等。
- 环境信息：NAS 型号或系统、Docker / Compose 版本、浏览器、Nook 镜像版本。
- 相关配置：路径映射、下载器保存路径、媒体库路径、站点类型等。请隐藏 Cookie、API Key、密码和 Token。
- 日志和任务信息：优先提供应用日志、任务队列详情、浏览器控制台错误、网络请求错误。

### 日志建议

排查订阅、站点搜索、下载器、整理入库、网盘扫描时，建议临时把日志级别调整为 `info` 或 `debug`，复现后再恢复。

Docker 部署常用命令：

```bash
docker compose ps
docker compose logs --tail=300 nook
docker compose exec nook sh -lc 'ls -la /data/logs || true'
```

PostgreSQL 部署查询示例：

```bash
docker compose exec postgres psql -U nook -d nook -c "select key,value,updated_at from settings order by updated_at desc limit 30;"
```

请勿公开粘贴以下敏感信息：

- 站点 Cookie、Passkey、API Key、YemaPT Auth Key、M-Team API Key。
- qBittorrent / Transmission / Aria2 密码。
- Telegram Bot Token。
- JWT、数据库密码、私有下载链接。

## Pull Request 提交规范

所有非紧急变更都应通过 Pull Request 合入 `main`，不要直接向主分支推送。贡献者可以从 fork 或本仓库的独立分支发起 PR；维护者只有在紧急安全修复、发布流水线修复等特殊场景下，才可以短暂绕过 PR 流程，并需要在提交说明或后续 Issue 中补充原因。

PR 应该尽量小而清晰。一次 PR 聚焦一个问题或一组强相关改动，避免把无关重构、格式化和功能混在一起。

### 分支要求

- 分支从最新 `main` 创建，提交前先同步远端主分支。
- 分支名建议使用 `fix/...`、`feat/...`、`docs/...` 或 `test/...`。
- 不要在 `main` 上直接开发和提交 PR 内容。
- 不要把个人部署配置、NAS 本地路径、私有镜像标签或测试数据提交进 PR。
- 如果基于魔改版、私有部署版或临时补丁开发，请先确认改动能在本仓库最新 `main` 上复现和应用，再提交 PR。

### PR 描述应包含

- 背景：修复哪个 Issue / 哪个用户场景 / 哪个回归。
- 改动摘要：后端、前端、配置、文档分别改了什么。
- 验证结果：运行过哪些命令，是否有无法运行的测试。
- 风险说明：数据迁移、Docker 配置、路径映射、下载器行为、站点 API 限流等是否受影响。
- 截图或录屏：涉及 UI、任务队列、错误提示、设置页时请附上。

### 推荐验证命令

根据改动范围选择运行：

前端开发与构建统一使用 Node.js 24.22.1 LTS 或更新的 24.x 安全补丁版本。使用 nvm 时先在仓库根目录执行 `nvm install` 和 `nvm use`；CI 从 `.nvmrc` 读取版本，Docker 前端构建阶段使用相同版本。升级时同步更新 `.nvmrc`、`Dockerfile` 和 `web/package.json` / `web/package-lock.json` 的 Node 版本要求。

前端使用 Tailwind CSS 4，浏览器最低要求为 Safari 16.4、Chrome/Edge 111、Firefox 128。主题与共享样式位于 `web/src/index.css`。升级样式依赖后需在浏览器验证布局、主题及交互；CI 会通过官方 npm 审计接口检查全部依赖，中危及以上漏洞会使 CI 失败并阻止后续发布。

本地以 mock、单元测试、前端构建和 lint 为主，无需启动 Docker 或 PostgreSQL。未设置 `NOOK_TEST_POSTGRES_DSN` 时，`go test` 会跳过依赖数据库的集成测试，其余测试正常运行。前端构建需先完成，以便 Go 嵌入 `web/dist`。

数据库集成测试交给 CI：流水线自动启动独立 PostgreSQL 测试实例、配置 `NOOK_TEST_POSTGRES_DSN` 并初始化 `pg_trgm` 扩展。每个测试使用独立 schema，结束后自动清理；测试失败会阻止后续发布。本地如需排查数据库问题，也可显式指定独立测试库，不能使用生产数据库地址。

```bash
npm --prefix web run build
npm --prefix web run lint
go test ./...
git diff --check
```

如果只改动某个模块，可以先跑定向测试，例如：

```bash
go test ./internal/service -run "TestOrganize|TestSubscription|TestMTeam" -count=1
go test ./cmd/server -count=1
```

### 代码要求

- Go 代码使用 `gofmt`。
- 前端 TypeScript 需要通过 `npm --prefix web run build`。
- 用户可见错误需要可操作：说明失败原因、下一步怎么查或怎么修。
- 后台任务失败不要静默吞掉，应进入任务队列、日志或 API 响应。
- Docker/NAS 路径相关改动必须考虑宿主机路径与容器路径映射。
- 站点 API、订阅和下载器改动需要注意去重、限流、敏感信息脱敏。

## Commit Message 建议

提交标题使用 `类型(可选范围): 具体变更`。自动发布会把上一个发布标签之后的提交按类型生成 changelog，并附上提交及版本对比链接：

- `feat:`：Features，新功能。
- `fix:`：Fixes，问题修复。
- `perf:` / `refactor:`：性能优化 / 重构。
- `docs:`：文档与配置说明。
- `build:` / `ci:`：构建与发布。
- `test:`、`chore:` 等其他类型或没有前缀的提交：Other Changes。
- 类型后带 `!`（如 `feat(api)!:`）：Breaking Changes，不兼容变更。

标题应说明具体结果，避免只写“优化”或“bug 处理”。示例：

```text
fix(organizer): report the cause of hardlink failures
feat(yemapt): add auth-only site adapter
docs: add issue and pull request guidelines
test: cover subscription restore matching
```

## 维护者合并检查

PR 运行独立的 CI。合并到 `main`、推送发布 tag 或手动发布时，发布工作流先调用同一套 CI（含独立 PostgreSQL 集成测试和 Compose 检查），全部成功后才递增版本、推送镜像和创建 Release。测试失败会阻止所有后续发布；测试与构建固定使用同一提交。手动指定其他 ref 发布镜像时，会先解析为固定 SHA，再测试和构建该 SHA。服务器部署仍需单独进行。

合并前建议确认：

- PR 范围清楚，没有夹带无关改动。
- 自动化检查通过，或失败原因已说明。
- 修改涉及的用户路径已有日志、错误提示或回归测试。
- 文档、示例配置和 README 是否需要同步更新。
