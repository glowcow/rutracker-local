# rutracker-local

Self-hosted browser for an offline rutracker XML dump. Streams the official `~30 GB` XML dump into Postgres, then serves a Russian full-text search UI + JSON API from a single Go binary. Built for one machine, one user, one home network.

The code is developed in a private GitLab project; this repository is its public snapshot, refreshed on releases.

## Contents

- [Features](#features)
- [Stack](#stack)
- [Quick start (Docker)](#quick-start-docker)
- [Configuration](#configuration)
  - [Live seeders/leechers (off by default)](#live-seedersleechers-off-by-default)
  - [Send to Transmission (optional)](#send-to-transmission-optional)
- [API](#api)
- [How it works](#how-it-works)
- [Project layout](#project-layout)
- [Local development](#local-development)
- [Operations](#operations)
  - [Loading a dump](#loading-a-dump)
  - [Reclaim Postgres bloat (one-shot VACUUM FULL)](#reclaim-postgres-bloat-one-shot-vacuum-full)
- [Releases](#releases)
- [Security model](#security-model)
- [Disclaimer](#disclaimer)
- [License](#license)

## Features

- Full-text search over titles and forum names (Postgres `tsvector`, Russian config), sorted by relevance, date or size, 25 a page.
- The forum tree as a three-level filter, with torrent counts.
- A torrent's page: description rendered from BBCode, the file tree from the dump, a magnet link, a link to the topic.
- Favourites, shared by everyone who can reach the app.
- An in-app Parser panel that loads a dump and streams its progress and log.
- Optional: send a torrent to a Transmission daemon; live seeders and leechers (off by default).
- English and Russian UI, light and dark themes, two colour schemes — picked in the settings menu behind the gear.

## Stack

| Area | Choice |
|---|---|
| Backend | Go, `net/http` pattern routing, [pgx/v5](https://github.com/jackc/pgx), [goose](https://github.com/pressly/goose) migrations run at boot under an advisory lock, `log/slog`, Prometheus metrics at `/metrics` |
| Database | PostgreSQL 17, a `tsvector` (Russian config) GIN index for full-text search |
| Parser | `xz -dc --threads=4` (native binary) → `encoding/xml` → 4 workers that `COPY` into a session `TEMP` table, then `INSERT … SELECT … ON CONFLICT DO UPDATE` |
| Frontend | React 19, Vite 8, Tailwind 4, TanStack Query 5, Radix Tooltip, lucide icons; animation is CSS only |
| Delivery | The built `web/dist/` is embedded into the Go binary — one image, one container |

The parser lands a 2.78M-row dump in ~14 minutes on a Raspberry Pi-class host.

## Quick start (Docker)

The image is on Docker Hub, published per release as `glowcow/rutracker:<vX.Y.Z>` — pin a version (no moving `latest` tag is published).

**Get a dump.** rutracker publishes the full catalogue as a torrent on the tracker itself — an XML dump named `rutracker-YYYYMMDD.xml.xz` (~30 GB unpacked), refreshed every few months. Download it into a directory, e.g. `./dumps`; it's mounted read-only below.

Minimal compose:

```yaml
services:
  postgres:
    image: postgres:17.11-alpine
    environment:
      POSTGRES_DB:       rutracker
      POSTGRES_USER:     rutracker
      POSTGRES_PASSWORD: change-me
    volumes:
      - pgdata:/var/lib/postgresql/data

  api:
    image: glowcow/rutracker:v1.9.0   # pick a published version — no :latest tag
    depends_on: [postgres]
    environment:
      POSTGRES_HOST:     postgres
      POSTGRES_DB:       rutracker
      POSTGRES_USER:     rutracker
      POSTGRES_PASSWORD: change-me
      RT_ADMIN_TOKEN:    change-me-too   # unlocks "start parse" in the Parser panel
    volumes:
      - ./dumps:/dumps:ro
    ports:
      - "127.0.0.1:8080:8080"   # see Security model before exposing it wider
    # The binary auto-runs goose migrations on startup; no init job needed.

volumes:
  pgdata:
```

Then open `http://localhost:8080`, click the database icon in the header, paste the admin token, pick the dump and start a parse with **sweep** on — see [Loading a dump](#loading-a-dump). A full dump lands in ~15 minutes.

## Configuration

Every app-owned variable is `RT_`-prefixed. The only exceptions are `POSTGRES_*` — the postgres image's own names, shared with the db container through one `env_file`. The binary reads `RT_DATABASE_URL` if set, otherwise builds the DSN from the `POSTGRES_*` vars. The password is URL-escaped via `net/url.UserPassword`, so dollars / spaces / special chars are fine.

| Variable | Default | Notes |
|---|---|---|
| `RT_DATABASE_URL` | _(built from below)_ | Wins if set. |
| `POSTGRES_HOST` | `rt_postgres` | Compose service name; override for local dev. |
| `POSTGRES_PORT` | `5432` | |
| `POSTGRES_DB` | _(required)_ | |
| `POSTGRES_USER` | _(required)_ | |
| `POSTGRES_PASSWORD` | _(required)_ | |
| `POSTGRES_SSLMODE` | `disable` | |
| `RT_HTTP_ADDR` | `:8080` | Bind address. |
| `RT_ADMIN_TOKEN` | _(empty → start disabled)_ | Bearer token gating `POST /api/admin/parse` (start a dump load). Empty = fail-closed; read-only admin endpoints stay open on the LAN. |
| `RT_DUMP_DIR` | `/dumps` | Directory the in-app Parser panel lists dumps from; matches the container mount. |

### Live seeders/leechers (off by default)

> **Currently non-functional.** Since ~2026-07-25 rutracker serves the forum behind a Cloudflare **managed challenge** (`cf-mitigated: challenge`, `Just a moment...`), so every scrape — and the login itself — returns `403`. The feature ships **off**; turning it on only refills the log with `403`s. Alternatives were ruled out: the public API is disabled, `bt*.t-ru.org` is WAF-blocked, and public UDP trackers under-report by 50–100×.

Peer counts aren't in the dump — they're scraped live from the rutracker topic page. Off → the `/peers` endpoint reports `configured:false`, the UI hides the drawer row, the list badge and the dashboard's peer counter, and nothing goes out to rutracker.

| Variable | Default | Notes |
|---|---|---|
| `RT_PEERS_ENABLED` | `false` | Master switch. Unset or unparseable → off (fail-closed). Needs `RT_USER`+`RT_PASS` too. |
| `RT_USER` | _(empty → off)_ | rutracker login; used to mint a `bb_session` cookie. |
| `RT_PASS` | _(empty → off)_ | rutracker password. Keep it in a server-side env file, never in git. |
| `RT_BASE_URL` | `https://rutracker.org` | Site root scraped for peers. |
| `RT_PEERS_TTL` | `24h` | A torrent's counts are re-scraped at most once per this window. |
| `RT_PEERS_MIN_INTERVAL` | `2s` | Minimum spacing between outbound scrapes (politeness rate limit). |

When on, counts are scraped **lazily** — only when a torrent's detail is opened — cached in `torrent_peers` (24h TTL), and reused by the list/favorites badges. The login cookie is cached in `rutracker_session` (survives restarts) and re-minted on expiry (single-flight). Nothing is fetched in bulk: at most one request per opened torrent per day, globally rate-limited.

### Send to Transmission (optional)

The drawer's **Transmission** button pushes a torrent's magnet straight into a Transmission daemon (`torrent-add` over RPC). Set the RPC URL to enable; leave it empty and the button is hidden. The magnet is built server-side from the stored info-hash. Reachability is probed (cached ~20s) and surfaced at `GET /api/transmission/status` — a dead daemon greys the button out instead of failing on click.

| Variable | Default | Notes |
|---|---|---|
| `RT_TRANSMISSION_RPC_URL` | _(empty → off)_ | Daemon RPC endpoint, e.g. `http://transmission_dc:9091/transmission/rpc`. |
| `RT_TRANSMISSION_HOST` | _(URL hostname)_ | `Host` header for the daemon's `rpc-host-whitelist`; defaults to the RPC URL's hostname (port stripped). |
| `RT_TRANSMISSION_USER` | _(empty)_ | Basic-auth user — only if `rpc-authentication-required` is on. |
| `RT_TRANSMISSION_PASS` | _(empty)_ | Basic-auth password. |

## API

All responses are JSON. The server logs request method, path, status, and duration in JSON-line format.

Transport notes that apply across endpoints:

- Responses are **gzip-compressed** when the client sends `Accept-Encoding: gzip` (JSON, HTML, JS/CSS; fonts and other pre-compressed types pass through).
- `/api/forums` carries an **ETag keyed on the dump version** (latest succeeded `parser_run` id) — send `If-None-Match` back and get a `304` instead of the body. (`/api/stats` dropped its ETag so its live `peers_cached` count isn't 304'd away.)
- Mutating endpoints reject **cross-origin browser requests** (`Origin`/`Sec-Fetch-Site` mismatch → `403`); header-less clients like `curl` are unaffected.
- Every `/api/*` handler runs under a **10 s deadline**.

<details>
<summary><code>GET /healthz</code> — liveness probe (Postgres ping)</summary>

Liveness probe with a Postgres ping. Used by docker compose healthchecks.

```bash
$ curl -s http://localhost:8080/healthz
{"status":"ok"}
```

</details>

<details>
<summary><code>GET /metrics</code> — Prometheus metrics reference</summary>

Prometheus exposition format. Scrape this from your monitoring stack.

Custom metrics on top of the Go-process defaults:

| Metric | Type | Labels | Source |
|---|---|---|---|
| `rutracker_http_requests_total` | counter | `method`, `route`, `status_class` | Per-request middleware. Route is the normalized endpoint (`/api/torrents/{id}` etc), status_class is `2xx`/`3xx`/`4xx`/`5xx`. |
| `rutracker_http_request_duration_seconds` | histogram | `method`, `route` | Per-request middleware. Buckets tuned for sub-50 ms p50 + the cold-cache stats endpoint. |
| `rutracker_torrents_total` | gauge | — | `SELECT COUNT(*) FROM torrents`, recomputed only when a new succeeded `parser_run` lands (same versioned cache as `/api/stats`); other gauges still refresh every 30 s. |
| `rutracker_forums_total` | gauge | — | Same versioned refresh — distinct `forum_id` count. |
| `rutracker_torrents_size_bytes` | gauge | — | Sum of `size_bytes`. |
| `rutracker_torrents_live_tuples` | gauge | — | `n_live_tup` from `pg_stat_user_tables` for the `torrents` table. |
| `rutracker_torrents_dead_tuples` | gauge | — | `n_dead_tup` from `pg_stat_user_tables`. Plot `dead/(dead+live)` and watch for a sustained > 20 % — that's the autovacuum-falling-behind threshold. The dashboard's "PG — torrents dead tuple ratio" panel does this already. |
| `rutracker_pg_database_size_bytes` | gauge | — | `pg_database_size(current_database())` — total on-disk footprint (heap + indexes + TOAST + dead space). Climbing while `torrents_total` is flat is the bloat early-warning. |
| `rutracker_parser_last_run_*` | gauge | — | Snapshot from the latest `parser_runs` row: `started_timestamp_seconds`, `finished_timestamp_seconds`, `duration_seconds`, `rows_inserted`, `rows_swept`. |
| `rutracker_parser_runs_total` | gauge | `status` | Lifetime row count from `parser_runs` per status (`running`/`succeeded`/`failed`). |

A starter Grafana dashboard JSON lives at `grafana/rutracker-overview.json`. It's data-source-templated against Prometheus / VictoriaMetrics — import via Dashboards → New → Import.

</details>

<details>
<summary><code>GET /api/stats</code> — aggregate counts, cached per dump</summary>

Aggregate counts for the dashboard. The dump-derived fields are cached in-process and keyed on the latest succeeded `parser_run` id — full-scan SQL runs once per parser sweep, every subsequent request returns the memo'd snapshot. `peers_cached` (how many torrents have a cached seeders/leechers snapshot) grows as browsing populates the cache, so it's recomputed fresh per request and sits outside that cache (hence no ETag on `/api/stats`). `peers_enabled` mirrors `RT_PEERS_ENABLED` so the UI can drop the whole peer layer when it's off. `dump_updated_at` is when the last successful ingest finished (the UI's `@ dd-mm-yyyy` freshness stamp); it's omitted entirely until a first sweep succeeds.

```bash
$ curl -s http://localhost:8080/api/stats | jq
{
  "torrents_total": 2797665,
  "total_size_bytes": 7897016228601515,
  "forums_count": 1341,
  "peers_cached": 12043,
  "peers_enabled": false,
  "dump_updated_at": "2026-08-08T04:12:31.884Z"
}
```

</details>

<details>
<summary><code>GET /api/forums</code> — forum list with row counts, cached per dump</summary>

Every forum that has at least one torrent, with row count. Forum names are the original rutracker breadcrumbs (`"Top - Mid - Leaf"`). Same `parser_run`-keyed cache as `/api/stats` — the `GROUP BY` aggregate runs at most once per dump.

```bash
$ curl -s 'http://localhost:8080/api/forums' | jq '.items[:3]'
[
  { "id": 313,  "name": "Зарубежные фильмы (HD Video)", "count": 88412 },
  { "id": 1868, "name": "EBM lossless",                  "count":  2391 },
  { "id": 1576, "name": "Зарубежная музыка (lossless)",  "count": 11254 }
]
```

</details>

<details>
<summary><code>GET /api/search</code> — full-text search + filters + sort + pagination</summary>

Full-text + filter + sort. Query params:

| Param | Type | Default | Notes |
|---|---|---|---|
| `q` | string | _empty_ | Russian FTS over `title` (weight A) + `forum_name` (weight B). Empty → no FTS filter. |
| `forum_id` | int | `0` | `0` or omitted → no forum filter. |
| `sort` | enum | `date` | One of `relevance` / `date` / `size`. Unknown values fall back to `date`. |
| `dir` | enum | `desc` | `asc` or `desc`. |
| `offset` | int | `0` | Capped at `100000` (values past the cap reset to 0). |
| `limit` | int | `50` | Capped at `200`. |

`q` is truncated to 256 chars before hitting the FTS parser.

`sort=relevance` with an empty `q` silently degrades to `date desc` (no relevance signal without a query).

Response carries `items` plus the full `total` for pagination math. The total runs as a separate `COUNT(*)` query, memoised per `(q, forum_id)` for the lifetime of the current dump (keyed on the latest succeeded `parser_run` id) — paging through a result set pays the count once, and the page query itself stays a cheap index-ordered top-N.

```bash
$ curl -s 'http://localhost:8080/api/search?q=Pink+Floyd&sort=relevance&limit=2' | jq
{
  "items": [
    {
      "id": 4287419,
      "title": "Pink Floyd - The Dark Side of the Moon (1973) FLAC",
      "forum_id": 1576,
      "forum_name": "Зарубежная музыка (lossless)",
      "size_bytes": 4287419002,
      "registered_at": "2023-04-03T00:00:00Z",
      "hash": "BBBB1234567890ABCDEFABCDEFABCDEFABCDEF02"
    },
    { ... }
  ],
  "total": 412
}
```

Filter by forum, sort by size descending:

```bash
curl -s 'http://localhost:8080/api/search?forum_id=313&sort=size&dir=desc&limit=10'
```

Paginate:

```bash
# page 3 of 25-per-page
curl -s 'http://localhost:8080/api/search?q=Linux&offset=50&limit=25'
```

</details>

<details>
<summary><code>GET /api/torrents/{id}</code> — single torrent with rendered description + magnet recipes</summary>

Single torrent with the BBCode body rendered to safe HTML. `[img]` tags are dropped entirely; URLs are restricted to `http`, `https`, `magnet`, `ftp`; everything else is HTML-escaped before tag conversion. 404 on unknown id.

```bash
$ curl -s http://localhost:8080/api/torrents/4287419 | jq
{
  "id": 4287419,
  "title": "Pink Floyd - The Dark Side of the Moon (1973) FLAC",
  "forum_id": 1576,
  "forum_name": "Зарубежная музыка (lossless)",
  "size_bytes": 4287419002,
  "registered_at": "2023-04-03T00:00:00Z",
  "hash": "BBBB1234567890ABCDEFABCDEFABCDEFABCDEF02",
  "content_html": "<p><strong>Pink Floyd</strong> ...</p>"
}
```

The `hash` is a 40-char hex SHA-1 (uppercase). The API doesn't hand back a ready-made magnet URI — clients build it from `hash` + `title`:

```
magnet:?xt=urn:btih:<hash>&dn=<urlencoded title>
```

Shell one-liner — pipe the torrent endpoint into a magnet URI:

```bash
$ curl -s http://localhost:8080/api/torrents/4287419 \
  | jq -r '"magnet:?xt=urn:btih:\(.hash)&dn=\(.title | @uri)"'
magnet:?xt=urn:btih:BBBB1234567890ABCDEFABCDEFABCDEFABCDEF02&dn=Pink%20Floyd%20-%20The%20Dark%20Side%20of%20the%20Moon%20%281973%29%20FLAC
```

That's exactly how the UI's "Скачать" button builds the magnet — no tracker URL list baked into the response. The base metadata comes straight from the dump; live seeder/leecher counts are a separate, opt-in endpoint (`GET /api/torrents/{id}/peers`, below).

The response also carries `files_count` when the dump shipped a file listing for the torrent, so the drawer can label its (collapsed) Files section without fetching the tree — the listing itself lives at `GET /api/torrents/{id}/files`.

</details>

<details>
<summary><code>GET /api/torrents/{id}/files</code> — the torrent's file listing</summary>

The file tree the dump ships with every torrent (`<dir>`/`<file>` elements), flattened to slash-joined paths. Entries are `[path, size]` tuples: the listing is stored pre-rendered as deflated JSON, so a request decompresses straight into the response body without re-marshalling. Unknown id — or a torrent the dump gave no listing for — returns `404`, and the drawer then hides its Files section.

Listings are immutable between dumps, so the response carries the same **dump-version ETag** as `/api/forums`; send `If-None-Match` back for a `304`.

Large listings are capped at **1000 entries** (`truncated: true`); `files_count` always reports the real total. The dump's median torrent holds 10 files and its p99 holds 429, but the tail reaches ~33k — enough JSON to stall the drawer.

```bash
$ curl -s http://localhost:8080/api/torrents/3261256/files | jq
{
  "files_count": 9,
  "truncated": false,
  "files": [
    ["The Cure . 1979 . Boys Don't Cry/The Cure - Boys Don't Cry.cue", 1512],
    ["The Cure . 1979 . Boys Don't Cry/The Cure - Boys Don't Cry.flac", 195885721],
    ["The Cure . 1979 . Boys Don't Cry/Artwork/Front.jpg", 9249971]
  ]
}
```

</details>

<details>
<summary><code>GET /api/torrents/{id}/peers</code> — live seeders/leechers (scraped, cached)</summary>

Cached peer counts for one torrent, refreshed from the rutracker topic page when the cache is missing or older than `RT_PEERS_TTL` (24h). Requires `RT_PEERS_ENABLED=true` **and** credentials (see [Live seeders/leechers](#live-seedersleechers-off-by-default)) — otherwise every call returns `{"configured": false}` and nothing is scraped. The feature ships off, so this is the default response.

On a cache miss/stale entry the server confirms the id is a real torrent (`404` otherwise), then scrapes under a global rate limit and a per-id single-flight. A failed scrape returns the stale cache (if any) plus an `error` reason instead of dropping the numbers:

- `error: "auth"` — login/cookie couldn't be established (bad credentials or a captcha wall).
- `error: "unavailable"` — rutracker unreachable (network / Cloudflare / non-200).

```bash
$ curl -s http://localhost:8080/api/torrents/4287419/peers | jq
{
  "configured": true,
  "seeders": 128,
  "leechers": 14,
  "checked_at": "2026-07-05T13:20:58Z"
}
```

The same cached counts are embedded in list responses (`/api/search`, `/api/favorites`) as optional `seeders` / `leechers` / `peers_checked_at` fields (omitted for torrents never checked), so the list/favorites badges render without a per-row request.

</details>

<details>
<summary><code>POST /api/torrents/{id}/download</code> — queue in Transmission</summary>

Builds the magnet server-side from the stored info-hash and sends it to the configured Transmission daemon (`torrent-add`). Requires `RT_TRANSMISSION_RPC_URL` (see [Send to Transmission](#send-to-transmission-optional)); unset → `503`. Unknown id → `404`; a torrent with no clean 40-hex hash → `422`; the daemon unreachable → `502`.

```json
{ "status": "added",     "name": "Ubuntu 24.04 LTS" }
{ "status": "duplicate", "name": "Ubuntu 24.04 LTS" }   // already in the queue
```

</details>

<details>
<summary><code>GET /api/transmission/status</code> — feature state</summary>

`{ "configured": bool, "online": bool }`. `configured` gates rendering the drawer button; `online` is a reachability probe (a `session-get` RPC, cached ~20s) that greys the button when the daemon is down. Both `false` when the feature is off.

</details>

<details>
<summary><code>GET /api/favorites</code> — global favorites list</summary>

Returns every starred torrent joined with its current row in `torrents`. The service is single-tenant (no auth) so this is one global list. Items are ordered `added_at DESC` (newest star first) — clients can render directly without sorting.

```bash
$ curl -s http://localhost:8080/api/favorites | jq
{
  "items": [
    {
      "id": 4287419,
      "title": "Pink Floyd - The Dark Side of the Moon (1973) FLAC",
      "forum_id": 1576,
      "forum_name": "Зарубежная музыка (lossless)",
      "size_bytes": 4287419002,
      "registered_at": "2023-04-03T00:00:00Z",
      "hash": "BBBB1234567890ABCDEFABCDEFABCDEFABCDEF02",
      "added_at": "2026-06-07T11:38:34.097818Z"
    }
  ]
}
```

The `favorites` table FK points at `torrents(id)` with `ON DELETE CASCADE`, so a sweep that drops the underlying row also removes it here — the list never shows orphans.

</details>

<details>
<summary><code>POST /api/favorites/{id}</code> — star a torrent</summary>

Idempotent star. `204` on success, `404` if the torrent id doesn't exist, `400` on malformed id.

```bash
$ curl -sS -X POST http://localhost:8080/api/favorites/4287419 -w '%{http_code}\n'
204
```

</details>

<details>
<summary><code>DELETE /api/favorites/{id}</code> — unstar a torrent</summary>

Idempotent unstar (un-starring a non-starred id is `204`, not `404`).

```bash
$ curl -sS -X DELETE http://localhost:8080/api/favorites/4287419 -w '%{http_code}\n'
204
```

</details>

<details>
<summary><code>DELETE /api/favorites</code> — clear all favorites</summary>

Clear all favorites. Used by the UI's "Очистить избранное" button (after a confirm prompt).

```bash
$ curl -sS -X DELETE http://localhost:8080/api/favorites -w '%{http_code}\n'
204
```

</details>

<details>
<summary><code>/api/admin/*</code> — parser control (dump loading)</summary>

Backs the in-app Parser panel. **Reads are open** (LAN); the mutating start requires `Authorization: Bearer <RT_ADMIN_TOKEN>` — an unset token on the server makes it `503` (fail-closed).

| Endpoint | Auth | Purpose |
|---|---|---|
| `GET /api/admin/dumps` | open | List dump files in the `/dumps` mount (`name`, `size_bytes`, `mtime`). |
| `GET /api/admin/parse/runs?limit=N` | open | Recent `parser_runs` (default 3), newest first. |
| `GET /api/admin/parse/status` | open | `{running, run}` — the in-flight or last run. |
| `GET /api/admin/parse/stream` | open | **SSE** stream of `status` / `progress` / `log` events. Replays the current run's buffer on connect, then streams live (exempt from the 10 s `/api/*` deadline and from gzip). |
| `POST /api/admin/parse` | **bearer** | Start a parse — body `{source, batch_size, sweep, workers}`. `202` on start, `409` if one is already running (single-flight). Runs in the background, cancelled only on server shutdown. |

```bash
$ curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
    http://localhost:8080/api/admin/parse \
    -d '{"source":"rutracker-20260627.xml.xz","batch_size":1000,"sweep":true}'
{"status":"started"}
```

The parse runs server-side and is tracked in `parser_runs`; a `running` row left over from a crash is reconciled to `failed` on the next startup.

</details>

## How it works

One Go module, one binary that takes no commands: it migrates the schema and serves. Nothing is plugged in dynamically — adding a feature usually means a new exported function in one of these packages plus a wire-up line in `cmd/rutracker/`.

| Package | What it owns | Built on |
|---|---|---|
| `cmd/rutracker/` | Entry point: runs the migrations, then the HTTP server. Owns `log/slog` setup, SIGTERM context plumbing and the `-version` flag. | `flag`, `log/slog` |
| `internal/config/` | Reads `RT_DATABASE_URL` or assembles a DSN from the `POSTGRES_*` env vars. URL-escapes the password via `net/url.UserPassword` so special chars don't break the connection string. | stdlib only |
| `internal/db/` | Builds the `pgxpool.Pool` and runs goose migrations from an `embed.FS` so the binary needs no external `.sql` files at runtime. Appliers are serialised via `pg_advisory_lock` so concurrent boots can't race the same DDL. | [`pgx/v5`](https://github.com/jackc/pgx), [`pressly/goose`](https://github.com/pressly/goose) |
| `internal/parser/` | Streams the XML dump. Spawns native `xz -dc --threads=4` via `os/exec` and feeds its stdout to `encoding/xml.Decoder`; for plain `.xml`/`.xml.gz` falls back to a direct read or `compress/gzip`. Emits each `<torrent>` element via callback, including its `<dir>`/`<file>` tree flattened to slash-joined paths. | `encoding/xml`, `os/exec`, `compress/gzip` |
| `internal/filelist/` | Encodes a torrent's file listing for storage: a compact JSON array of `[path, size]` pairs, deflated. JSON is what the API hands to the browser, so stored bytes decompress straight into the response. Hand-rolled marshaller (the dump carries ~150M entries) with a decompression bound. | `compress/flate`, `encoding/json` |
| `internal/store/` | The canonical `Torrent` type plus all Postgres reads/writes — `GetStats`, `ListForums`, `Search` (with `ts_rank` + sort whitelist to keep ORDER BY off user input), `Get`, `CopyIngester` (per-worker handle: `SET synchronous_commit=off`, session-local `TEMP TABLE`, `COPY` → `INSERT…SELECT…ON CONFLICT`), and the `parser_runs` lifecycle + `Sweep` (mark-and-sweep deletion of torrents missing from the new dump). | `pgx/v5` |
| `internal/peers/` | Scrapes seeders/leechers from a logged-in rutracker topic page: holds a `bb_session` cookie, parses the `<span class="seed/leech">` block, re-logins (single-flight) when the cookie dies, and rate-limits every outbound request. Store side (`UpsertPeers`/`GetPeers`/`CountCachedPeers`, `torrent_peers`, `rutracker_session`) lives in `internal/store/`. | `net/http`, [`golang.org/x/sync/singleflight`](https://pkg.go.dev/golang.org/x/sync/singleflight) |
| `internal/bbcode/` | Tiny BBCode → HTML renderer. HTML-escapes first, drops `[img]` entirely, restricts URL schemes to `http`/`https`/`magnet`/`ftp`, supports `[b/i/u/s/quote/spoiler/list/code/pre]`, and placeholder-protects generated `<a>` tags so bracketed tokens inside URLs survive the tag passes. Unit-tested against XSS-via-raw-HTML, XSS-via-`javascript:` URL and placeholder forgery. | stdlib only |
| `internal/server/` | `net/http` server (Go 1.22+ pattern routing). Handlers for `/api/*` and `/healthz`, Prometheus at `/metrics` with a per-request `method/route/status_class` middleware and a background gauge refresher (cheap gauges every 30 s; the full-scan content stats only when a new succeeded `parser_run` lands). Middleware chain: access log → metrics → security headers (CSP etc.) → cross-origin guard → gzip → 10 s API deadline. Versioned in-process caches (stats, forums, search totals) keyed on the dump version; `/api/forums` serves a matching ETag. An unknown `/api` path answers a JSON 404; any other unknown path falls back to the embedded Vite `dist/`'s index.html for client-side routing; hashed assets get `immutable` cache headers. | `net/http`, [`prometheus/client_golang`](https://github.com/prometheus/client_golang) |
| `web/` (Go side) | Single Go file with `//go:embed all:dist` that exposes an `fs.FS` consumed by `server`. The whole SPA ships inside the binary — one image, one container, no static-asset CORS dance. | `embed` |

Each parse worker holds its own `*pgxpool.Conn` for the lifetime of the run; the pool is sized to `numWorkers + 1` so the workers don't fight for a connection. The parser side uses one decoder goroutine and `chan []store.Torrent` (buffered to `numWorkers`) to hand batches off without copying.

## Project layout

```
cmd/rutracker/        # entry point: migrate, then serve
internal/
  bbcode/             # BBCode -> safe HTML renderer + unit tests
  config/             # env loading + Postgres DSN
  db/                 # pgxpool + goose migrations (internal/db/migrations, embed.FS)
  filelist/           # file-listing codec (JSON + deflate) for torrent_files
  parser/             # streaming XML decoder + native xz spawn
  peers/              # live seeders/leechers scraper (login + topic-page parse)
  server/             # net/http handlers, SPA fallback, /metrics middleware
  store/              # Torrent type, CopyIngester, parser_runs, Sweep, peers, files, queries
web/                  # Vite + React + Tailwind v4 SPA, embedded into the binary
grafana/              # importable Grafana dashboard JSON
Dockerfile            # multi-stage: node → golang → alpine (in the GitHub snapshot)
```

## Local development

Backend:

```bash
# from repo root, with a Postgres running somewhere
export RT_DATABASE_URL='postgres://rutracker:pw@localhost:5432/rutracker?sslmode=disable'

go run ./cmd/rutracker           # migrates, then serves http://localhost:8080
```

Frontend (separate terminal, hot reload):

```bash
cd web
npm ci
npm run dev                      # http://localhost:5173
# Vite proxies /api/* to localhost:8080 — set VITE_API_HOST to override.
```

Tests:

```bash
go test ./...                    # Postgres integration tests skip without DATABASE_URL (the test DSN, not RT_DATABASE_URL)
cd web && npx tsc --noEmit && npx eslint .
```

Linting matches CI: `gofmt -l` over the tracked Go files, `go vet ./...`, `staticcheck ./...`.

## Operations

### Loading a dump

Dumps are loaded from the **Parser panel in the app** (the database icon in the header) — the parse runs inside the live server and streams progress and logs back over SSE (see the `/api/admin/*` endpoints above).

- **Watching is open** on the LAN: anyone can see a running parse's progress bar + live log tail and the last few loads. State lives server-side, so a page reload or another device re-attaches to a run in flight.
- **Starting a parse needs the admin token** — `RT_ADMIN_TOKEN` in the server's environment (empty → the start endpoint is disabled). Paste it once in the panel; it's kept in the browser's `localStorage` and sent as a bearer token on the start call only.

Panel options: the **dump** (a picker over the container's `/dumps` mount), **batch size** (`500` for top-ups, `1000` for a cold full load), and **sweep** — turn it on **only for a full dump** (it deletes torrents missing from the new dump; a partial re-parse with sweep on wipes legitimate rows). A full dump lands in ~14 min.

### Reclaim Postgres bloat (one-shot VACUUM FULL)

Identical re-parses are no-ops at the heap/TOAST level (see `internal/store/torrents.go` — the `INSERT … ON CONFLICT DO UPDATE … WHERE … IS DISTINCT FROM …` skips writes when nothing changed), so day-to-day TOAST growth is small and autovacuum keeps up. But:

- A change to how `content` is rendered (e.g. a new BBCode tag, an HTML pass) rewrites every row's TOAST chunk on the next parse — that's ~5 GB of fresh TOAST on the 2.78 M-row corpus.
- The dashboard's **PG — torrents dead tuple ratio** panel climbing past ~20 % is the signal that autovacuum isn't catching up.

When that happens, run `VACUUM (FULL, ANALYZE) torrents` once. It takes an exclusive lock on the table for the duration (a few minutes on the 2.78 M-row table — ~4 min on a Raspberry Pi-class host), so the API can't read from `torrents` until it finishes. There's no migration / image change; this is a one-shot maintenance op against the running Postgres.

Against the compose stack from [Quick start](#quick-start-docker):

```bash
docker compose exec postgres \
  psql -U rutracker -d rutracker \
  -c "VACUUM (FULL, VERBOSE, ANALYZE) torrents;"
```

Sanity-check the result:

```bash
docker compose exec postgres psql -U rutracker -d rutracker -c "
  SELECT pg_size_pretty(pg_total_relation_size('torrents'))                                            AS total,
         pg_size_pretty(pg_relation_size('torrents'))                                                  AS heap,
         pg_size_pretty(pg_indexes_size('torrents'))                                                   AS idx,
         pg_size_pretty(pg_total_relation_size('torrents') - pg_relation_size('torrents') - pg_indexes_size('torrents')) AS toast,
         pg_size_pretty(pg_database_size('rutracker'))                                                 AS db_total;"
```

Online alternative — `pg_repack` (via triggers + shadow copy) avoids the exclusive lock at the cost of more disk headroom and an extension install. For a one-off cleanup the simple `VACUUM FULL` is cheaper than setting it up.

## Releases

Every `vX.Y.Z` tag publishes the image `glowcow/rutracker:vX.Y.Z` on Docker Hub, after the code has passed the linters and the tests against a real Postgres 17.

Each tag also lands here as a [GitHub release](https://github.com/glowcow/rutracker-local/releases): one source-snapshot commit plus notes built from the commit subjects since the previous tag.

Only the version tag is published — no moving `:latest`. (A `:latest` sharing a digest with the pinned `:vX.Y.Z` is a footgun: `docker system prune -a` on the host strips the version tag off the shared image, leaving a running container floating on an untagged id.)

## Security model

The app is built for a trusted home network and has **no user accounts**. Only starting a parse is gated (by `RT_ADMIN_TOKEN`); everything else is open to anyone who can reach the port:

- search, torrent details and file lists;
- the shared favorites list (add / remove / clear);
- `POST /api/torrents/{id}/download` — queues a torrent in your Transmission daemon, if one is configured;
- parser status, logs and the list of dump files under `/dumps`;
- Prometheus metrics at `/metrics`.

Mutations from other origins are rejected (`Sec-Fetch-Site` / `Origin` check), so a random web page can't drive the API through your browser. Do **not** publish the port to the internet as is — put it behind a reverse proxy with authentication (basic auth, an SSO forward-auth, a VPN) if you need remote access.

## Disclaimer

This project ships **no rutracker content**: it is a viewer for the dump that rutracker itself publishes, and the test fixtures are synthetic. Access to rutracker is restricted in some countries — check what applies where you live. The optional peer scraper logs in with your own account and is off by default; using it is subject to rutracker's rules.

## License

[MIT](LICENSE) © 2026 Anton Sediuk
