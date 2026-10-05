# Emo Registry 设计文档

Emo Registry 是 Emo 编程语言的包管理中心注册表——对标 Ruby 的 rubygems.org、Node.js 的
npmjs.com。本文档是包发布与分发机制的权威设计，包含 Emo 编译器 HTTPS 客户端将来实现时
所依赖的协议契约。

Emo 编译器实现在同级仓库(`../emo`)。客户端包机制(manifest、解析、lockfile、缓存)
已在那里实现完毕；本服务是缺失的服务端。

## 1. 语言特点决定的约束

| Emo 的特点 | 对 registry 的影响 |
|---|---|
| `package.emo` manifest 严格固定 4 个字段(`name`、`version`、`targets`、`deps`);旧编译器对未知字段报 E5100 | 描述、license、作者、README 等元数据**只存服务端**,经 API 返回——绝不写进 manifest |
| 版本只有精确 semver(`major.minor.patch`,纯数字),无版本范围;MVS(最小版本选择)解析在客户端完成 | 服务端只需提供版本列表和每版本依赖数据,不需要服务端依赖求解器 |
| 包 = 纯 `.emo` 源码集合,必须含 `package.emo` | 发布校验简单:manifest 能解析、`name`/`version` 匹配、全是源码文件即可 |
| checksum 写入 `emo.lock` 并用于客户端内容寻址缓存(`name-version-checksum` 目录) | 服务端必须复现完全一致的 digest 算法,以校验发布并让 lockfile 可验证 |
| `targets` 字段(`native`、`wasm`、`typescript`、`beam`、`riscv64`) | 版本记录携带 target 元数据,索引可按 target 过滤 |
| 作用域命名 `owner/name`;顶级短名(`net`、`http`)保留给官方 stdlib | 所有权模型 = scope 即账号/组织;短名由服务端保留名单硬控制 |
| registry 端点由 `EMO_REGISTRY` 环境变量配置;私有 registry 是一等场景 | 协议必须简单到可自建,甚至可以作为静态文件树托管 |

### Checksum 算法:一开始就用 SHA-256

内容 digest = **对所有 `.emo` 文件按路径排序后的 `(路径, 内容)` 对做 SHA-256**,连接格式
`path \0 content \0`(与编译器 `Registry.digest` 相同的构造方式,在任何版本发布之前
从 MD5 升级为 SHA-256)。内容 digest(写进 `emo.lock` 的那个)与归档完整性哈希用同一
算法。没有算法前缀,没有 MD5 兼容路径——编译器侧的修改与本服务同窗口落地。

## 2. 包格式:`.emoji` 归档

发布产物是单个 gzip 压缩的 tar 归档,命名为:

```
<owner>--<name>--<version>.emoji
```

(类比 RubyGems 的 `<name>-<version>.gem`。)内容:

```
foo--json_tools--1.2.0.emoji  (tar.gz)
├── package.emo          # manifest;name/version/targets/deps 的唯一权威来源
├── json_tools.emo       # 源码
├── internal/...         # 私有子树原样保留
├── README.md            # 可选,仅根部;在包详情页渲染,不参与 digest
└── EMO-METADATA.json    # 服务端生成:描述/license/作者、逐文件 SHA-256 清单
```

不变式:

- **内容 digest 只对 `.emo` 文件计算**;客户端解包时忽略 `EMO-METADATA.json` 与
  `README.md`,因此解包归档的 digest 永远与 lockfile checksum 一致。(编译器的
  `collect_files` 只收集 `.emo` 文件,归档里多带 README 对现有客户端是安全的。)
- `package.emo` 由作者编写,是权威来源;`EMO-METADATA.json` 由服务端生成(类比
  RubyGems 的 `metadata.gz`),绝不回写进 manifest——旧编译器不受影响。

## 3. 所有权与命名

- 包名形如 `owner/name`。`owner` 必须是发布账号本人或其所属组织。
- 顶级短名(不含 `/`)保留给官方 stdlib(`net`、`http`,以及为未来 stdlib 预留的
  名单),普通账号注册时直接拒绝。
- 命名规则:小写 ASCII 字母、数字、`_`、`-`;owner 与 name 各 1–64 字符。

## 4. 发布(类比 `gem push`)

```
emo publish
  → 本地校验 manifest、收集 .emo 文件、计算 SHA-256 digest、打 tar.gz
  → POST /api/v1/packages   (Authorization: Bearer <token>,body = .emoji 归档)
```

服务端校验流水线(全部通过才入库,否则 4xx 带稳定错误码):

1. token 有效且有 `push` scope。
2. 包名合法;owner 与 token 所属账号/组织匹配;短名未经保留名单批准一律拒绝。
3. version 是合法精确 semver 且尚不存在——**版本不可变**;已发布版本永不覆盖,
   只能 yank。
4. 归档能解开;`package.emo` 按编译器同款受限规则解析(仅字面量、无插值);
   manifest 的 `name`/`version` 与发布请求一致。
5. 重算的 SHA-256 内容 digest 与客户端上报一致。
6. 归档落存储(本地 `data/storage` 或经存储抽象的 S3/R2),写数据库,更新索引。

### Yank

`DELETE /api/v1/packages/:owner/:name/versions/:version`(或 `emo publish --yank`)。

软删除语义,刻意比 RubyGems 宽容:yanked 版本从解析索引中排除,但归档仍可下载,
lockfile 已钉住的项目不会断供。客户端在 lockfile 命中 yanked 版本时给出警告。

### API token

格式 `emo_<48 位十六进制>`;只存 SHA-256 哈希。scope 分 `push`、`yank`、`read`
(私有包,后期)。Web UI 创建/吊销,支持按包限定与过期时间。

## 5. 通信协议

Base URL 即 `EMO_REGISTRY` 指向的地址。公开只读端点无需 token。错误格式统一:

```json
{ "error": { "code": "version_exists", "message": "foo/json_tools 1.2.0 already published" } }
```

`code` 是稳定的机器可读枚举(`package_not_found`、`version_not_found`、
`version_exists`、`invalid_manifest`、`checksum_mismatch`、`unauthorized`、
`forbidden`、`name_reserved`);`message` 只给人看。

### 协议 A——裸文件兼容层

镜像当前编译器客户端已实现的目录协议(`Registry.versions` / `fetch` 作用于文件系统
树)。把 `EMO_REGISTRY=https://<主机>` 指向本服务即可立即工作:

```
GET /:owner/:name/versions                  → ["0.1.0", "0.2.0"]   (JSON 数组,不含 yanked)
GET /:owner/:name/:version/package.emo      → manifest 原文
GET /:owner/:name/:version/<path>.emo       → 单个源文件
```

协议 A 只提供 `.emo` 源文件——它是编译器的拉包通道,`README.md` 与
`EMO-METADATA.json` 有意不在此暴露。

这一层也可以原样导出为静态文件树扔到 CDN。

### 协议 B——JSON API(未来 HTTPS 客户端的契约)

以下 schema **定稿冻结**:编译器的 HTTPS 客户端将硬编码依赖它们。允许增量演进
(新增字段);改名或删字段即协议破坏。

#### B.1 版本列表——MVS 解析的核心输入

```
GET /api/v1/packages/:owner/:name/versions
```

```json
{
  "package": "foo/json_tools",
  "versions": [
    {
      "version": "1.2.0",
      "checksum": "9f2c...ab",
      "archive_sha256": "4d7e...01",
      "targets": ["native", "wasm"],
      "yanked": false,
      "published_at": "2026-09-30T12:00:00Z"
    }
  ]
}
```

- `checksum`——对排序后 `.emo` 文件的 `path\0content\0` 做 SHA-256 的内容 digest,
  正是客户端写进 `emo.lock` 的值。
- `archive_sha256`——`.emoji` 文件本身的哈希,仅用于下载完整性校验。
- `targets`——全量列表;按构建 target 过滤是客户端的事。
- `yanked`——yanked 版本**出现在列表里并打标记**,而不是消失:lockfile 钉住
  yanked 版本的老项目能得到明确警告,而不是莫名其妙的 `version_not_found`。
  解析时跳过它们。
- 数组按 semver 升序。

#### B.2 批量依赖查询——加速 MVS

```
GET /api/v1/dependencies?packages=foo/json_tools,baz/qux
```

```json
{
  "foo/json_tools": [
    { "version": "1.2.0", "deps": { "net": "0.1.0" }, "yanked": false }
  ],
  "baz/qux": []
}
```

- `deps` 原样就是 manifest 里 `deps {}` 块的内容(精确版本)——解析期间客户端
  不需要解析 manifest 原文。
- 不存在的包在响应里**直接缺席**,客户端按 `package_not_found` 处理。
- yanked 版本也列出(客户端解析时跳过)。

#### B.3 归档下载

```
GET /downloads/:owner--:name--:version.emoji
```

- 200:body 为 tar.gz,header 带 `Digest: sha-256=<base64>`(RFC 9530)与 `ETag`。
- yanked 版本仍可下载;响应带 `X-Emo-Yanked: true`,客户端据此警告。
- 404 走统一错误格式。

#### B.4 包元信息(Web UI、`emo search`、`emo info` 用;编译期不依赖)

```
GET /api/v1/packages/:owner/:name
```

```json
{
  "name": "foo/json_tools",
  "description": "JSON utilities for Emo",
  "license": "MIT",
  "homepage": "https://example.com",
  "repository": "https://github.com/foo/json_tools",
  "downloads": 12345,
  "downloads_last_30d": 987,
  "owners": ["foo"],
  "latest_version": "1.2.0",
  "created_at": "...", "updated_at": "..."
}
```

只增不减地演进。

#### B.5 发布 / yank

```
POST   /api/v1/packages
Authorization: Bearer <token>
Content-Type: application/octet-stream
Body: <.emoji 归档原始字节>
```

- 包名/版本从归档内的 `package.emo` 解析——**绝不**从 URL 或表单参数取;单一事实
  来源,不存在 URL 与 manifest 不一致的问题。
- 201 返回 B.1 的 version 对象。已存在返回 409 `version_exists`;manifest 校验
  失败返回 422 `invalid_manifest`(带具体原因)。

```
DELETE /api/v1/packages/:owner/:name/versions/:version
Authorization: Bearer <token>
```

- yank。200 返回该 version 对象(`yanked: true`);重复 yank 幂等返回 200,不报错。

#### 元信息维护

description、license、homepage、repository 不在归档里,也不在发布请求里。发布后
经 Web UI 或以下端点维护:

```
PATCH /api/v1/packages/:owner/:name   (Authorization: Bearer <token>)
```

这既保持 manifest 4 字段的纯净,也让 `emo publish` 保持极简。

## 6. 服务端数据模型

- `users`(username, email, password_digest)
- `organizations`(name, display_name, user_id)+ `memberships`(organization_id,
  user_id, role)——用户名与组织名共用一个命名空间:两者都能作为包的 scope,注册时
  跨两张表查重。组织的任何成员(owner 或 member)都能以组织 scope 发布、yank、
  改元信息;只有 owner 能管理成员。
- `api_tokens`(user_id, name, token_hash, scopes, expires_at, last_used_at)
- `packages`(owner_scope, name, description, license, homepage, repository, user_id,
  downloads)
- `versions`(package_id, version, checksum, archive_sha256, targets, deps, size,
  yanked_at, user_id, storage_key)
- `reserved_names`(name, reason)——短名保留名单,启动时从硬编码 stdlib 名单播种,
  运营可扩充
- `downloads`(version_id, date, count)——按版本按日的聚合表,每次下载同步写入
  ((version_id, date) 唯一索引保证行唯一;读-改-写竞争最坏只是少计)

## 7. 安全与治理

- 版本不可变 + yank 不删档 → 供应链可审计。
- 短名保留名单在注册时强制。
- 上传大小限制(纯源码包 10MB 足够);归档内只允许 `.emo` 文件白名单。
- 限流:发布按 token 用户(30 次/小时),signup/login 与搜索按 IP(10 次/分钟、
  60 次/分钟)。进程内固定窗口;Redis 后端留待三期。
- 只存 token 哈希;密码 bcrypt。

## 8. 分期路线

1. **一期(可用闭环)**:账号 + token、publish/yank、协议 A 与 B 双全、版本列表、
   归档下载、Web 包详情页。官方编译器把 `EMO_REGISTRY` 指过来即可经协议 A 拉包。
2. **二期**:批量 dependencies API 强化。
3. **三期**:静态导出到 CDN、私有包与 `read` scope token、Redis 化的下载统计与限流、
   `emo publish` / `emo search`
   CLI 合入编译器仓库。
