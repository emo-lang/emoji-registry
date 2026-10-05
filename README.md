# Emo Registry

The official package registry for the [Emo programming language](../emo) —
the counterpart of rubygems.org for Ruby or npmjs.com for Node.js. It serves
package publishes, yanks, version and dependency indexes, archive downloads,
and a web UI for browsing and token management.

The wire protocol the Emo compiler talks to is specified in
[docs/design.md](docs/design.md) ([中文](docs/design.zh-CN.md)) — that document
is the authoritative contract; this README only covers running and hacking on
the service.

Built with [Airway](https://github.com/daqing/airway) (Gin + templ).

## Features (phase 1)

- Accounts with bcrypt passwords, web sessions, and API tokens
  (`emo_<48 hex>`, only the SHA-256 hash is stored, scoped `push`/`yank`)
- Organizations: members publish, yank and edit metadata under the org scope;
  owners manage members. Usernames and org names share one namespace
- Reserved short names live in the `reserved_names` table (seeded from the
  stdlib list at boot) plus a hardcoded fallback; admins manage the DB list at
  `/admin/reserved` — grant the first admin with `go run . admin:grant <username>`
- Publishing: upload a `.emoji` archive (gzip tar), server-side manifest
  validation, SHA-256 content digest shared with the compiler, immutable
  versions, reserved stdlib names
- Yanking: soft delete — yanked versions leave the resolution indexes but
  stay downloadable for pinned lockfiles
- Protocol A, the bare-file compatibility layer (`GET /:owner/:name/versions`,
  per-version manifest and source files), so the stock compiler works with
  `EMO_REGISTRY` pointed at a self-hosted instance
- Protocol B, the frozen JSON API under `/api/v1` (version lists, batch
  dependency queries, package metadata, publish/yank)
- Archive downloads at `/downloads/<owner>--<name>--<version>.emoji` with
  `Digest` / `ETag` integrity headers
- Download statistics: a per-version, per-day `downloads` aggregate; with
  `REDIS` configured, downloads are counted in Redis and flushed to the
  database every minute (30-day chart on the package page, `downloads_last_30d`
  in the metadata API)
- Rate limiting ([airway-ratelimit-plugin](https://github.com/daqing/airway-ratelimit-plugin)):
  publish 30/hour per token user, signup/login 10/minute and search 60/minute
  per IP; Redis-backed when `REDIS` is set, in-process otherwise
- Redis integration via
  [airway-redis-plugin](https://github.com/daqing/airway-redis-plugin)
  (`REDIS` env, PING-verified setup, health check)
- Private packages: `visibility` per package (API PATCH or the package page);
  readable by the owner, org members, and their `read`-scoped tokens — the
  toolchain authenticates with `EMO_TOKEN`; unauthorized requests get the same
  404 as unknown packages
- Static export: `go run . registry:export <dir>` renders the whole registry
  (public packages only) as a protocol A file tree plus archives — serve it
  from nginx/CDN/S3 and point `EMO_REGISTRY` at it
- README support: archives may ship a root `README.md` (never digested),
  rendered as sanitized HTML on the package page
- Web UI: home page with recent and most-downloaded packages, paginated
  search, package detail pages, signup/login, token and organization
  management

## Quick start

Requirements: Go 1.27+. SQLite needs nothing else; PostgreSQL and MySQL are
also supported via `DSN`.

```bash
cp .env.example .env
```

Edit `.env`:

```
AIRWAY_ENV="local"
DSN="sqlite://data/registry.db"
LISTEN="127.0.0.1:1905"
```

Then migrate and run:

```bash
go run . db:migrate
go run . server          # http://127.0.0.1:1905
```

### Publish your first package

```bash
# 1. Create an account
curl -X POST http://127.0.0.1:1905/api/v1/signup \
  -H 'Content-Type: application/json' \
  -d '{"username":"foo","email":"foo@example.com","password":"super-secret"}'

# 2. Mint an API token (basic auth with email + password; the plaintext
#    token is returned exactly once)
curl -X POST http://127.0.0.1:1905/api/v1/tokens \
  -u foo@example.com:super-secret \
  -H 'Content-Type: application/json' \
  -d '{"name":"cli","scopes":["push","yank"]}'
# => {"token":"emo_..."}

# 3. Build a .emoji archive — a gzip tar whose name is
#    <owner>--<name>--<version>.emoji, containing package.emo + .emo sources
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

# 4. Publish
curl -X POST http://127.0.0.1:1905/api/v1/packages \
  -H "Authorization: Bearer emo_..." \
  -H 'Content-Type: application/octet-stream' \
  --data-binary @foo--hello--0.1.0.emoji
```

### Point the Emo toolchain at it

```bash
export EMO_REGISTRY=http://127.0.0.1:1905
emo deps resolve     # talks protocol A; works against any self-hosted instance
```

## API summary

Errors share one shape: `{"error":{"code":"...","message":"..."}}` with a
real HTTP status. Full request/response schemas: docs/design.md §5.

| Endpoint | Auth | Purpose |
|---|---|---|
| `POST /api/v1/signup` | — | create an account |
| `POST /api/v1/login` | — | web session cookie |
| `POST /api/v1/tokens` | session or basic | mint an API token (shown once) |
| `GET /api/v1/tokens` | session or basic | list your tokens |
| `DELETE /api/v1/tokens/:id` | session or basic | revoke a token |
| `POST /api/v1/packages` | Bearer `push` | publish a `.emoji` archive |
| `DELETE /api/v1/packages/:owner/:name/versions/:version` | Bearer `yank` (owner only) | yank a version (idempotent) |
| `GET /api/v1/packages/:owner/:name` | — | package metadata (B.4) |
| `PATCH /api/v1/packages/:owner/:name` | Bearer `push` (owner only) | edit description/license/homepage/repository |
| `GET /api/v1/packages/:owner/:name/versions` | — | version list, ascending semver (B.1) |
| `GET /api/v1/dependencies?packages=a/b,c/d` | — | batch dependency query (B.2) |
| `GET /downloads/:owner--:name--:version.emoji` | — | archive download (B.3) |
| `POST /api/v1/orgs` | session or basic | create an organization |
| `GET /api/v1/orgs/:name` | — | organization info + members |
| `POST /api/v1/orgs/:name/members` | session or basic (org owner) | add a member |
| `DELETE /api/v1/orgs/:name/members/:username` | session or basic (org owner) | remove a member (last owner protected) |
| `GET /:owner/:name/versions` | — | protocol A: JSON array of versions |
| `GET /:owner/:name/:version/package.emo` | — | protocol A: manifest source |
| `GET /:owner/:name/:version/<path>.emo` | — | protocol A: individual source file |

Web pages: `/`, `/search`, `/p/:owner/:name`, `/signup`, `/login`,
`/tokens` and `/orgs` (login required).

## Development

```bash
just dev               # air (auto-rebuild) + templ --watch, via overmind
just generate          # go generate ./... — regenerate *_templ.go after editing .templ
just generate-watch    # keep views regenerated while you edit
go test ./...          # full test suite (sqlite in temp dirs, no services needed)
go run . repl          # REPL with the project's models loaded
```

Layout:

```
app/
  api/            # HTTP handlers, one package per namespace
  middlewares/    # token auth (Bearer), web session auth
  models/         # users, sessions, api_tokens, packages, versions
  services/emoji/  # manifest parser, content digest, .emoji archive handling
  views/          # templ pages (home, search, packages, auth, tokens, …)
db/migrate/       # Go DSL migrations (schema.RegisterChange)
config/routes.go  # the full route table
docs/design.md    # the authoritative protocol contract
```

## Docker

```bash
docker build -t emo-registry .
docker run -p 1905:1905 -e DSN="sqlite://data/registry.db" emo-registry db:migrate
docker run -p 1905:1905 -e DSN="sqlite://data/registry.db" emo-registry
```

Set `DSN` to a PostgreSQL/MySQL URL for anything beyond a single container;
mount a volume at `/app/data` to keep the SQLite file and stored archives.

## Deploying on a company intranet

A self-hosted instance is a first-class scenario: the Emo toolchain only needs
`EMO_REGISTRY` pointed at your server. A typical intranet setup looks like:

```
developers ──> nginx (TLS) ──> emo-registry (:1905)
                                  ├── PostgreSQL / MySQL / SQLite
                                  ├── Redis (optional)
                                  └── storage: local dir or internal S3/SeaweedFS
```

### 1. Run the service

```bash
docker build -t emo-registry .
docker run --rm \
  -e DSN="postgres://registry:secret@db.internal:5432/registry" \
  emo-registry db:migrate        # one-off: apply migrations

docker run -d --name emo-registry -p 1905:1905 \
  -e AIRWAY_ENV=production \
  -e DSN="postgres://registry:secret@db.internal:5432/registry" \
  -e REDIS="redis://redis.internal:6379/0" \
  -e STORAGE_DRIVER="local" \
  -v registry-data:/app/data \
  emo-registry
```

Environment variables (see `.env.example` for the full list):

| Variable | Required | Notes |
|---|---|---|
| `DSN` | yes | PostgreSQL/MySQL for production; SQLite is fine for a small team |
| `REDIS` | no | enables Redis-backed rate limiting and download counters; without it both fall back to in-process mode |
| `STORAGE_DRIVER` | no | `local` (default), `s3`, `r2`, `cos`; SeaweedFS works via `s3` + `STORAGE_ENDPOINT` |
| `URL_PREFIX` | no | set when serving under a sub-path behind a proxy, e.g. `/registry` |
| `LISTEN` | no | defaults to `127.0.0.1:1905`; the Docker image sets `:1905` |

Multi-replica deployments need `REDIS` — otherwise each replica has its own
rate-limit budgets and download counters.

### 2. Create the first admin

There is no web UI for granting admin. Use the CLI:

```bash
docker exec emo-registry /app/app admin:grant <username>
```

Admins manage the reserved top-level names (`net`, `http`, …) at
`/admin/reserved`.

### 3. Keep internal code internal

Publish proprietary packages as private (`PATCH /api/v1/packages/:owner/:name`
with `{"visibility":"private"}`, or the toggle on the package page). Private
packages are invisible to anonymous users — even their existence is hidden
(404, same as unknown packages). Developers pull them with a `read`-scoped
token:

```bash
export EMO_REGISTRY=https://registry.internal.example.com
export EMO_TOKEN=emo_...        # token with the read scope
emo deps resolve
```

Use organizations (`/orgs`) to give a whole team publish/read access under a
shared scope like `acme/widgets`.

### 4. Serve read-only traffic from a static mirror (optional)

Protocol A is plain files, so the whole public registry can be exported and
hosted on any static file server or CDN inside your network:

```bash
docker exec emo-registry /app/app registry:export /app/data/export
# sync /app/data/export to nginx / S3; point EMO_REGISTRY there
```

Private packages are never exported. Run the export periodically (cron) or
after each publish.

### 5. Backups

Two things hold all state: the database and the storage directory
(`/app/data/storage` with the local driver). Back up both; archives are
immutable, so an incremental copy is enough.
