# bangumi-subject-go

本地化的 [Bangumi](https://bgm.tv) 数据服务。利用 Archive 每周导出的 wiki 数据，导入本地 SQLite，
并提供 REST 查询 API，支撑前端网页的复杂查询筛选（条目/人物/角色/章节搜索、合作人物、制作人员、关联关系等）。

单二进制、跨平台（无 CGO）、支持 Docker。

## 数据来源

- 导出说明与表结构：[wiki_database.md](./wiki_database.md)
- 最新导出：[bangumi/Archive releases](https://github.com/bangumi/Archive/releases/tag/archive)（每周三更新，也可解析其 `aux/latest.json`）
- id 常量 yaml：[bangumi/common](https://github.com/bangumi/common)（本仓库 `common/` 子模块，编译时内嵌进二进制）

## 技术栈

| 组件 | 选择 | 说明 |
|---|---|---|
| 语言 | Go 1.25+ | 标准工具链交叉编译 |
| 数据库 | SQLite（[modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)） | 纯 Go 驱动，无 CGO，WAL 模式，FTS5 trigram 中文搜索 |
| Web | [Gin](https://github.com/gin-gonic/gin) | REST API + 静态文件托管 |
| 前端 | [Svelte 5](https://svelte.dev) + [Tailwind CSS 4](https://tailwindcss.com) + [Vite](https://vite.dev) | 搜索测试页，构建后内嵌进二进制（`web/`） |
| YAML | [goccy/go-yaml](https://github.com/goccy/go-yaml) | 支持 common yaml 的 anchor/alias |
| 部署 | Docker 多阶段构建 | alpine 最终镜像，数据卷持久化 |

## 快速开始

### 1. 导入数据

```bash
# 获取 dump（zip 或解压目录均可）
# 从 https://github.com/bangumi/Archive/releases 下载，或解析 aux/latest.json

# 导入（全量重建，~1.7GB 数据约需数分钟）
go run ./cmd/bangumi import -file dump-2026-07-28.210449Z.zip

# 测试用小样本
go run ./cmd/bangumi import -file dump.zip -limit 1000
```

### 2. 启动服务

```bash
go run ./cmd/bangumi serve
# 默认监听 :8080，数据库 data/bangumi.db
```

### 3. Docker

```bash
docker compose build
# 导入（把 dump 放入 ./data/）
docker compose run --rm import /app/data/dump.zip
# 启动服务
docker compose up -d bangumi
```

## 前端页面（内嵌）

`web/` 是一个 Svelte + Tailwind 的搜索测试页（条目/人物/角色搜索、分页、条目详情），
构建产物 `web/dist` 通过 `go:embed` 内嵌进二进制，单文件即可同时提供 API 与页面。
`serve` 时若指定了 `-web` 目录（且存在）则优先托管磁盘目录，否则回退到内嵌页面。

```bash
cd web
npm install
npm run build          # 生成 web/dist
cd ..
go build ./cmd/bangumi # 重新编译即带上最新页面（或 cd web && go generate）
```

开发模式（页面改动热更新，/api 代理到 :8080）：

```bash
go run ./cmd/bangumi serve -web web/dist   # 或直接跑内嵌版
cd web && npm run dev
```

后续扩展复杂检索：表单字段与查询参数映射集中在 `web/src/lib/api.js` 的
`buildSubjectQuery`，新增筛选只需在视图表单加字段并在该函数中映射；API 侧扩展见
`internal/api/subjects.go`。

## 命令行

```
bangumi import -file <dump.zip 或目录> [-db 路径] [-limit N]  导入数据
bangumi serve  [-listen :8080] [-db 路径] [-web 目录]         启动 API 服务
bangumi version                                               版本号
```

环境变量：`BANGUMI_DB`、`BANGUMI_LISTEN`、`BANGUMI_WEB`、`BANGUMI_COMMON`。
常量默认内嵌进二进制（`common/*.yml` 通过 go:embed），如需热加载最新子模块，用 `-common-dir ./common`。

## REST API

统一响应：`{"ok": true, "data": ...}`，错误：`{"ok": false, "error": "..."}`。

| 接口 | 说明 |
|---|---|
| `GET /api/health` | 健康检查 |
| `GET /api/stats` | 各表行数统计 |
| `GET /api/constants` | 全部 id→名称 常量（类型/平台/关联/职位），前端据此渲染 |
| `GET /api/pics/:kind/:id?size=` | 图片解析（轮询）：kind 取 person（人物头像）/ subject（条目封面）/ character（角色头像），size 支持 l/m/s/grid，返回 `{status: ok\|pending\|failed, url}` |
| `GET /api/subjects/search?q=&type=&platform=&tag=&rank_min=&score_min=&date_from=&date_to=&nsfw=&sort=&order=&page=&size=` | 条目搜索/筛选（q 匹配原名与中文名） |
| `GET /api/subjects/:id` | 条目详情（双向关联、制作人员、角色、章节数） |
| `GET /api/subjects/:id/episodes?type=&page=&size=` | 条目章节列表 |
| `GET /api/persons/search?q=&type=` | 人物搜索（q 匹配原名与 infobox 简体中文名） |
| `GET /api/persons/:id` | 人物详情（含人物/角色关联） |
| `GET /api/persons/:id/works?position=&subject_type=&page=&size=` | 人物参与的作品（按职位/类型筛选） |
| `GET /api/persons/:id/collaborators?page=&size=` | 与「X」合作的人物（共同作品数降序） |
| `GET /api/persons/:id/collaboration?page=&size=&positions_a=&positions_b=` | 「人物合作」页数据：人物简介 + 分页的合作人物及共同条目（含声优等，职位 id 已转常量文本）；`positions_a`/`positions_b` 为棋盘筛选的职位标签（`作品类型:职位id` 或 `cv`，逗号分隔多选，两组取交集） |
| `GET /api/persons/:id/collaboration/positions` | 「人物合作」棋盘筛选职位标签：self = 当前人物在共同条目中的职位，other = 合作人物的职位（含 CV） |
| `GET /api/persons/:id/collaboration/:other` | 双人合作：两人物共同参与的条目及双方职务（前端按职位双向合并分组展示） |
| `GET /api/persons/:id/roles` | 「单人作品」页数据：人物参与的全部条目及职务（含 CV 出演，前端按职务分组） |
| `GET /api/characters/search?q=&role=` | 角色搜索（q 匹配原名与 infobox 简体中文名） |
| `GET /api/characters/:id` | 角色详情（出演作品 + CV） |

### 搜索示例

```bash
# 2020 年后评分 8 分以上、rank 前 5000 的动画
curl "localhost:8080/api/subjects/search?type=2&score_min=8&rank_min=1&date_from=2020-01-01&sort=rank&size=20"

# 按标签筛选漫画
curl "localhost:8080/api/subjects/search?type=1&tag=奇幻&sort=score&order=desc"

# 全文搜索（中文子串匹配）
curl "localhost:8080/api/subjects/search?q=路人女主的养成方法"

# 与人物 1 合作的声优/制作人员（对应前端合作板块）
curl "localhost:8080/api/persons/1/collaborators"

# 「人物合作」页（左侧人物简介 + 右侧合作人物及共同条目，含 CV 出演，按共同条目数倒序分页）
curl "localhost:8080/api/persons/7906/collaboration?page=1&size=20"

# 「人物合作」棋盘筛选（当前人物担任导演 × 合作人物担任分镜；标签先经 positions 接口获取）
curl "localhost:8080/api/persons/7906/collaboration/positions"
curl "localhost:8080/api/persons/7906/collaboration?positions_a=2:2&positions_b=2:4&page=1&size=20"

# 双人合作（两人共同参与的作品，含双方职务）
curl "localhost:8080/api/persons/7906/collaboration/596"

# 单人作品（该人物参与的全部作品及职务）
curl "localhost:8080/api/persons/7906/roles"
```

## 项目结构

```
cmd/bangumi/           CLI 入口（import / serve / version）
embedded.go            根级包：go:embed 内嵌 common/*.yml
web/                   Svelte+Tailwind 搜索页面（embed.go 内嵌 web/dist）
common/                bangumi/common 子模块（id 常量）
internal/
  common/              yaml 常量解析（anchor/alias），id→中文名
  model/               jsonlines 数据结构
  db/                  SQLite 连接与 schema（表/索引/FTS）
  importer/            zip/jsonlines 流式导入（事务批量）
  wiki/                轻量 infobox 解析（完整语法见 bangumi/wiki-parser-go）
  api/                 REST 接口
Dockerfile / docker-compose.yml
```

## 说明与限制

- 导入为**全量重建**（每周导出均为全量快照）；如需增量，可把旧库改名后对比。
- `infobox` 的完整 wiki 语法解析（层级列表/嵌套模板等）尚未实现，当前只提取 `{{Infobox}}` 的 key/value；
  复杂语法可参考 [bangumi/wiki-parser-go](https://github.com/bangumi/wiki-parser-go) 扩展 `internal/wiki`。
- 搜索使用 FTS5 trigram：≥3 字符走索引，短查询退化为全表扫描，量级上（百万行）可接受。