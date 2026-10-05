# Emo Registry

[Emo 编程语言](../emo)的官方包注册表——对标 Ruby 的 rubygems.org、Node.js 的
npmjs.com。提供包的发布与 yank、版本与依赖索引、归档下载,以及用于浏览和
token 管理的 Web 界面。

Emo 编译器所依赖的通信协议定义在
[docs/design.zh-CN.md](docs/design.zh-CN.md)([English](docs/design.md))——该文档是权威契约,本 README 只覆盖服务的运行与开发。

基于 [Airway](https://github.com/daqing/airway)(Gin + templ)构建。

## 功能(一期已实现)

- 账号体系:bcrypt 密码、Web session、API token(`emo_<48 hex>`,库里只存
  SHA-256 哈希,按 `push`/`yank` scope 授权)
- 发布:上传 `.emoji` 归档(gzip tar),服务端校验 manifest、计算与编译器一致的
  SHA-256 内容 digest、版本不可变、保留 stdlib 名字
- Yank:软删除——被 yank 的版本从解析索引中移除,但归档仍可下载,不破坏
  已锁定 lockfile 的项目
- 协议 A 裸文件兼容层(`GET /:owner/:name/versions`、逐版本的 manifest 与源文件),
  现有编译器把 `EMO_REGISTRY` 指向自托管实例即可工作
- 协议 B frozen JSON API(`/api/v1` 下的版本列表、批量依赖查询、包元信息、
  发布/yank)
- 归档下载 `/downloads/<owner>--<name>--<version>.emoji`,带 `Digest`/`ETag`
  完整性响应头
- Web UI:首页(最新发布/下载最多)、搜索、包详情页、注册登录、token 管理

## 快速开始

要求:Go 1.27+。用 SQLite 不需要任何其他服务;也可以通过 `DSN` 使用
PostgreSQL 或 MySQL。

```bash
cp .env.example .env
```

编辑 `.env`:

```
AIRWAY_ENV="local"
DSN="sqlite://data/registry.db"
LISTEN="127.0.0.1:1905"
```

然后迁移并启动:

```bash
go run . db:migrate
go run . server          # http://127.0.0.1:1905
```

### 发布你的第一个包

```bash
# 1. 创建账号
curl -X POST http://127.0.0.1:1905/api/v1/signup \
  -H 'Content-Type: application/json' \
  -d '{"username":"foo","email":"foo@example.com","password":"super-secret"}'

# 2. 签发 API token(用 email + 密码走 basic auth;明文 token 只返回这一次)
curl -X POST http://127.0.0.1:1905/api/v1/tokens \
  -u foo@example.com:super-secret \
  -H 'Content-Type: application/json' \
  -d '{"name":"cli","scopes":["push","yank"]}'
# => {"token":"emo_..."}

# 3. 构造 .emoji 归档——文件名为 <owner>--<name>--<version>.emoji 的 gzip tar,
#    内含 package.emo 与 .emo 源文件
mkdir hello && cd hello
cat > package.emo <<'EOF'
package {
  name = "foo/hello"
  version = "0.1.0"
}
EOF
echo 'let main = 1' > hello.emo
tar -czf ../foo--hello--0.1.0.emoji package.emo hello.emo
cd ..

# 4. 发布
curl -X POST http://127.0.0.1:1905/api/v1/packages \
  -H "Authorization: Bearer emo_..." \
  -H 'Content-Type: application/octet-stream' \
  --data-binary @foo--hello--0.1.0.emoji
```

### 让 Emo 工具链接入

```bash
export EMO_REGISTRY=http://127.0.0.1:1905
emo deps resolve     # 走协议 A;指向任何自托管实例都能工作
```

## API 摘要

错误统一为 `{"error":{"code":"...","message":"..."}}`,并返回真实 HTTP 状态码。
完整的请求/响应 schema 见 docs/design.md §5。

| 端点 | 认证 | 用途 |
|---|---|---|
| `POST /api/v1/signup` | 无 | 创建账号 |
| `POST /api/v1/login` | 无 | 换取 web session cookie |
| `POST /api/v1/tokens` | session 或 basic | 签发 API token(明文仅显示一次) |
| `GET /api/v1/tokens` | session 或 basic | 列出我的 token |
| `DELETE /api/v1/tokens/:id` | session 或 basic | 吊销 token |
| `POST /api/v1/packages` | Bearer `push` | 发布 `.emoji` 归档 |
| `DELETE /api/v1/packages/:owner/:name/versions/:version` | Bearer `yank`(仅属主) | yank 版本(幂等) |
| `GET /api/v1/packages/:owner/:name` | 无 | 包元信息(B.4) |
| `PATCH /api/v1/packages/:owner/:name` | Bearer `push`(仅属主) | 修改 description/license/homepage/repository |
| `GET /api/v1/packages/:owner/:name/versions` | 无 | 版本列表,semver 升序(B.1) |
| `GET /api/v1/dependencies?packages=a/b,c/d` | 无 | 批量依赖查询(B.2) |
| `GET /downloads/:owner--:name--:version.emoji` | 无 | 归档下载(B.3) |
| `GET /:owner/:name/versions` | 无 | 协议 A:版本 JSON 数组 |
| `GET /:owner/:name/:version/package.emo` | 无 | 协议 A:manifest 原文 |
| `GET /:owner/:name/:version/<path>.emo` | 无 | 协议 A:单个源文件 |

Web 页面:`/`、`/search`、`/p/:owner/:name`、`/signup`、`/login`、
`/tokens`(需登录)。

## 开发

```bash
just dev               # air(自动重编译)+ templ --watch,由 overmind 管理
just generate          # go generate ./...——改了 .templ 后重新生成 *_templ.go
just generate-watch    # 编辑 .templ 时持续重新生成
go test ./...          # 完整测试(sqlite 临时库,无需外部服务)
go run . repl          # 加载了项目模型的 REPL
```

目录速览:

```
app/
  api/            # HTTP handler,每个命名空间一个 package
  middlewares/    # token 认证(Bearer)、web session 认证
  models/         # users、sessions、api_tokens、packages、versions
  services/emoji/  # manifest 解析器、内容 digest、.emoji 归档处理
  views/          # templ 页面(home、search、packages、auth、tokens 等)
db/migrate/       # Go DSL 迁移(schema.RegisterChange)
config/routes.go  # 完整路由表
docs/design.md    # 权威协议契约
```

## Docker

```bash
docker build -t emo-registry .
docker run -p 1905:1905 -e DSN="sqlite://data/registry.db" emo-registry db:migrate
docker run -p 1905:1905 -e DSN="sqlite://data/registry.db" emo-registry
```

超过单容器规模时请把 `DSN` 换成 PostgreSQL/MySQL;挂载 `/app/data` 卷可持久化
SQLite 文件与已存储的归档。
