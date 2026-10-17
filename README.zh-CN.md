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
- 账号与作用域:用户名本身就是一个发布 scope,无需组织即可发布 `<username>/<pkg>`。
  组织是*共享* scope——成员都能以组织 scope 发布、yank、改元信息,owner 管理成员;
  用户名与组织名共用一个命名空间
- 短名保留名单存在 `reserved_names` 表(启动时从 stdlib 名单播种),另有硬编码兜底;
  管理员在 `/admin/reserved` 维护 DB 名单
- 管理后台 `/admin`:列出全部已发布的包(含私有包),按创建时间倒序;支持强制删除
  (purge)——输入 `I want to purge this package <owner>/<name>` 确认后,从数据库与
  存储中整体移除该包;另可管理保留名单;还可在 `/admin/accounts` 锁定账号或组织
  ——锁定后其包照常展示,但该 scope 下的下载、解析与写入一律返回
  `403 owner_locked`,解锁即恢复。可用 `.env` 里的
  `ADMIN_USERNAME`/`ADMIN_PASSWORD` 在 `/admin/login` 登录,或用
  `go run . admin:grant <username>` 提升站点账号
- 发布:上传 `.emoji` 归档(gzip tar),服务端校验 manifest、计算与编译器一致的
  SHA-256 内容 digest、版本不可变、保留 stdlib 名字
- Yank:软删除——被 yank 的版本从解析索引中移除,但归档仍可下载,不破坏
  已锁定 lockfile 的项目
- 协议 A 裸文件兼容层(`GET /:owner/:name/versions`、逐版本的 manifest 与源文件),
  现有编译器把 `EMO_REGISTRY` 指向自托管实例即可工作
- 协议 B frozen JSON API(`/api/v1` 下的版本列表、批量依赖查询、包元信息、
  发布/yank)
- 归档下载 `/downloads/<owner>-<name>-<version>.emoji`,带 `Digest`/`ETag`
  完整性响应头
- 下载统计:按版本按日的 `downloads` 聚合表;配置了 `REDIS` 时先计数到 Redis、每分钟
  批量落库(包详情页近 30 天柱状图,元信息 API 返回 `downloads_last_30d`)
- 限流([airway-ratelimit-plugin](https://github.com/daqing/airway-ratelimit-plugin)):
  发布按 token 用户 30 次/小时,signup/login 按 IP 10 次/分钟,搜索 60 次/分钟;
  配置了 `REDIS` 时计数在 Redis,否则进程内
- Redis 集成基于
  [airway-redis-plugin](https://github.com/daqing/airway-redis-plugin)
  (`REDIS` 环境变量、PING 验证、健康检查)
- 私有包:按包设置 `visibility`(API PATCH 或包详情页);属主、组织成员及其
  `read` scope token 可读——工具链用 `EMO_TOKEN` 认证;无权限请求得到与包不存在
  完全相同的 404
- 静态导出:`go run . registry:export <dir>` 把整个注册表(仅公开包)渲染为
  协议 A 文件树加归档目录——nginx/CDN/S3 静态托管后把 `EMO_REGISTRY` 指过去即可
- README 支持:归档可携带根部 `README.md`(不参与 digest),在包详情页渲染为
  消毒后的 HTML
- Web UI:首页(最新发布/下载最多)、分页搜索、包详情页、用户主页
  (`/u/:username`,列出该账号已上传的 emoji 包)、注册登录、token 与组织管理

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

# 3. 构造 .emoji 归档——文件名为 <owner>-<name>-<version>.emoji 的 gzip tar,
#    内含 package.emo 与 .emo/.c/.h 源文件
mkdir hello && cd hello
cat > package.emo <<'EOF'
package {
  name = "foo/hello"
  version = "0.1.0"
}
EOF
echo 'let main = 1' > hello.emo
tar -czf ../foo-hello-0.1.0.emoji package.emo hello.emo
cd ..

# 4. 发布
curl -X POST http://127.0.0.1:1905/api/v1/packages \
  -H "Authorization: Bearer emo_..." \
  -H 'Content-Type: application/octet-stream' \
  --data-binary @foo-hello-0.1.0.emoji
```

### 个人账号 vs 组织

包名是 `owner/name`,其中 `owner` 就是一个**作用域(scope)**。你的用户名本身
就是一个作用域:拿到 API token 后即可发布 `yourname/hello`,无需创建组织——
适合个人自己拥有的包。

**组织**是*共享*作用域。只有在团队共同拥有某个包、需要多人一起发布,你想用
项目/品牌名而非个人 handle,或希望作用域独立于某一账号长期存在时,才需要创建。

组织名与用户名共用一个命名空间;组织的**每个成员(不只是 owner)**都能以组织
scope 发布、yank、改元信息,owner 另外还能管理成员列表。上面的例子里 `foo`
就是发布者自己的用户名——只有在需要 `acme/widgets` 这类共享 scope 时,才去
`/orgs`(或 `POST /api/v1/orgs`)创建组织。

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
| `GET /downloads/:owner-:name-:version.emoji` | 无 | 归档下载(B.3) |
| `POST /api/v1/orgs` | session 或 basic | 创建组织 |
| `GET /api/v1/orgs/:name` | 无 | 组织信息 + 成员列表 |
| `POST /api/v1/orgs/:name/members` | session 或 basic(组织 owner) | 加成员 |
| `DELETE /api/v1/orgs/:name/members/:username` | session 或 basic(组织 owner) | 删成员(最后一个 owner 不可删) |
| `GET /:owner/:name/versions` | 无 | 协议 A:版本 JSON 数组 |
| `GET /:owner/:name/:version/package.emo` | 无 | 协议 A:manifest 原文 |
| `GET /:owner/:name/:version/<path>` | 无 | 协议 A:单个源文件(`.emo`/`.c`/`.h`) |

Web 页面:`/`、`/search`、`/p/:owner/:name`、`/u/:username`、`/signup`、
`/login`、`/tokens`、`/orgs`(需登录)与 `/admin`(仅管理员)。

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
docker run -d --name emo-registry -p 1905:1905 \
  -v registry-data:/app/data \
  -e DSN="sqlite://data/registry.db" emo-registry
```

镜像的 entrypoint 会在启动服务前自动执行迁移(`db:migrate`),因此
`docker compose up` 可以直接对着空库启动。只想迁移、不启动服务时显式传入命令:
`docker run --rm ... emo-registry db:migrate`。超过单容器规模时请把 `DSN` 换成
PostgreSQL/MySQL;挂载 `/app/data` 卷可持久化 SQLite 文件与已存储的归档。
多副本部署时建议在发布前单独执行一次迁移,而不是让每个副本启动时各自迁移。

## 企业内网部署

自建实例是一等场景:Emo 工具链只需要把 `EMO_REGISTRY` 指向你的服务器。
典型的内网部署结构:

```
开发者 ──> nginx (TLS) ──> emo-registry (:1905)
                              ├── PostgreSQL / MySQL / SQLite
                              ├── Redis(可选)
                              └── 存储:本地目录或内网 S3/SeaweedFS
```

### 1. 运行服务

```bash
docker build -t emo-registry .

# 可选:在发布前先执行迁移。entrypoint 在每次启动服务时也会迁移;
# 提前执行可以避免多副本同时启动时互相竞争。
docker run --rm \
  -e DSN="postgres://registry:secret@db.internal:5432/registry" \
  emo-registry db:migrate

docker run -d --name emo-registry -p 1905:1905 \
  -e AIRWAY_ENV=production \
  -e DSN="postgres://registry:secret@db.internal:5432/registry" \
  -e REDIS="redis://redis.internal:6379/0" \
  -e STORAGE_DRIVER="local" \
  -v registry-data:/app/data \
  emo-registry
```

环境变量(完整列表见 `.env.example`):

| 变量 | 必填 | 说明 |
|---|---|---|
| `DSN` | 是 | 生产用 PostgreSQL/MySQL;小团队用 SQLite 即可 |
| `REDIS` | 否 | 启用 Redis 后端的限流与下载计数;不配则两者退回进程内模式 |
| `STORAGE_DRIVER` | 否 | `local`(默认)、`s3`、`r2`、`cos`;SeaweedFS 用 `s3` + `STORAGE_ENDPOINT` 接入 |
| `URL_PREFIX` | 否 | 在代理后以子路径提供服务时设置,如 `/registry` |
| `LISTEN` | 否 | 默认 `127.0.0.1:1905`;Docker 镜像里为 `:1905` |
| `ADMIN_USERNAME`/`ADMIN_PASSWORD` | 否 | 两者都设置时,可用它们在 `/admin/login` 登录;任一改动会使所有已登录的管理员会话失效 |

多副本部署必须配置 `REDIS`——否则每个副本各自持有独立的限流配额和
下载计数。

### 2. 登录管理后台

管理后台 `/admin` 列出全部已发布的包——私有包也在内——按创建时间倒序,
并在 `/admin/reserved` 管理保留的顶级短名(`net`、`http` 等)。每行提供
"Force delete"操作:精确输入 `I want to purge this package <owner>/<name>`
确认后,该包的全部版本、下载统计与存储文件将被彻底删除,不可恢复。
`/admin/accounts` 列出全部用户账号与组织,并提供锁定开关:锁定后该 scope
的包在站点上照常展示,但版本列表、依赖查询、归档下载与该 scope 下的所有
写入在解锁前一律返回 `403 owner_locked`(message
`该账号(组织）已被锁定`);个人主页与组织页会显示「已锁定」徽标。
有两种登录方式:

- 在环境(`.env`)里设置 `ADMIN_USERNAME`/`ADMIN_PASSWORD`,然后在
  `/admin/login` 登录。
- 提升一个站点账号,用它的常规登录:

  ```bash
  docker exec emo-registry /app/app admin:grant <用户名>
  ```

未配置凭据时 `/admin/login` 返回 404,该登录入口保持不可见。

### 3. 内部代码保持私有

专有包发布为私有(`PATCH /api/v1/packages/:owner/:name` 传
`{"visibility":"private"}`,或用包详情页上的切换按钮)。私有包对匿名用户
完全不可见——连存在性都不暴露(404,与不存在的包一致)。开发者用带
`read` scope 的 token 拉取:

```bash
export EMO_REGISTRY=https://registry.internal.example.com
export EMO_TOKEN=emo_...        # 带 read scope 的 token
emo deps resolve
```

用组织(`/orgs`)让整个团队在共享 scope(如 `acme/widgets`)下获得
发布/读取权限。

### 4. 用静态镜像承载只读流量(可选)

协议 A 是纯文件语义,整个公开注册表可以导出后托管到内网任意静态文件
服务器或 CDN:

```bash
docker exec emo-registry /app/app registry:export /app/data/export
# 把 /app/data/export 同步到 nginx / S3;EMO_REGISTRY 指向它
```

私有包永不导出。可用 cron 周期执行,或在每次发布后执行。

### 5. 备份

全部状态只存在于两处:数据库和存储目录(local 驱动下为
`/app/data/storage`)。两者都要备份;归档不可变,增量复制即可。
