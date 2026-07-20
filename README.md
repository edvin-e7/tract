# Tract

A self-hosted **read-later / research reader** — an owned, $0 alternative to
**Pocket + Readwise + Feedly**. Save any URL, get a clean distraction-free
reader copy, full-text search everything you've saved, and (next) keep
highlights — all from a single binary you run yourself. No accounts, no cloud,
no paid APIs.

> Status: early. The architecture, API, search, and storage are real and
> tested; the UI is a minimal functional shell pending a dedicated design pass.
> See [Roadmap](#roadmap).

## What it is

When you save a link, Tract fetches the page, runs a readability pass to strip
nav/ads/boilerplate down to the article, and stores the title + clean text +
cleaned HTML in a local SQLite database. That text is mirrored into an FTS5
index, so search is instant and runs over the *full body*, not just titles.
Everything is local and self-hostable.

## Architecture & decisions

### The problem
Pocket/Readwise/Feedly are SaaS: your reading lives on someone else's server,
behind a subscription, exportable only on their terms. The goal is the same
capability — save, read clean, search, highlight — but **owned**: one binary,
one local database, runs anywhere, costs nothing.

### Constraints
- **$0, no paid APIs.** Extraction and search must be fully local/offline.
- **macOS-portable** (primary dev machine), Linux-deployable for hosting.
- **Single deployable artifact** — easy to host, easy to demo from one URL.

### Shape
A layered Go service serving a Vite/React SPA from the *same* binary:

```
cmd/tract/main.go      wiring: open DB, mount API + embedded frontend, listen
internal/store         SQLite + FTS5 (persistence, search)
internal/extract       URL fetch + readability (clean article)
internal/api           HTTP handlers (net/http ServeMux, method routing)
frontend/              Vite + React + TypeScript SPA
```

### Key trade-offs

**Pure-Go SQLite (`modernc.org/sqlite`), not the CGO `mattn` driver.**
The CGO driver is faster but needs a C toolchain and cross-compiles painfully.
A read-later tool is not write-throughput-bound, so the pure-Go engine's
cost is irrelevant — and in exchange the binary builds and cross-compiles with
just `go build`, no C compiler in the deploy image. Gotchas honored: driver name
is `"sqlite"` (not `"sqlite3"`); FTS5 is compiled in but *loadable extensions
are not available*, so search uses the built-in FTS5 module; the DB is opened
with `?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)` and a single open
connection to keep WAL writes serialized and dodge `database is locked`.

**FTS5 with content-table triggers, not a hand-rolled `LIKE` search.**
`LIKE '%term%'` can't rank, is slow, and tokenizes nothing. FTS5 ships with
SQLite, gives real tokenized matching + prefix search for type-ahead, and stays
in lockstep with `items` via `AFTER INSERT/UPDATE/DELETE` triggers — so a
deleted article also leaves the index (there's a test that falsifies exactly
this). User input is sanitized into a phrase-AND MATCH expression so punctuation
can't trigger FTS5 operator-syntax errors.

**stdlib `net/http` with Go 1.22 ServeMux method routing, no router dep.**
Since Go 1.22 the standard mux understands `"POST /api/items"` and
`"/api/items/{id}"` with `r.PathValue("id")`. For a handful of routes that's the
entire feature set a third-party router would add — so we take zero router
dependencies and stay on the standard library.

**`go-readability` for extraction.** A Go port of Mozilla's Readability; runs
offline, no API key, no cost. (Upstream has since been renamed; the pinned
module still builds and works — swapping to the new import path is a trivial,
isolated change behind the `internal/extract` boundary.)

**Frontend embedded via `//go:embed`, served by the same binary.** One artifact
to deploy. Vite is configured with `base: "./"` so assets resolve relatively and
don't 404 when served from a subpath. A committed **self-contained** placeholder
`index.html` keeps the embed directive valid on a fresh clone (and renders a
"frontend not built yet" page instead of a blank one, since built assets are
gitignored); `scripts/build.sh` stages the real build over it — that overwrite
shows up as a local modification and should not be committed.

**Server-side fetch is SSRF-guarded and size-capped.** `POST /api/items` makes
the server request an arbitrary caller-supplied URL, so the fetcher's dialer
refuses non-global addresses (private/loopback/link-local/metadata/CGNAT) at
connect time — on the first hop, on every redirect hop, and after DNS
resolution — and reads at most 10 MiB. See [Security](#security).

**Rendering article HTML (`dangerouslySetInnerHTML`).** The content is third-party
HTML, so it is **sanitized server-side with bluemonday before it is ever stored or
served** (a UGC-policy allow-list), not just cleaned by readability. There is a test
that falsifies this — it feeds a `<script>`/`onerror` payload through extraction and
asserts it does not survive. `dangerouslySetInnerHTML` then renders already-sanitized
markup.

**Highlighting is the product's verb.** In the reader, select any passage and a
floating **Highlight** pill appears; one click saves it. Saved passages are rendered
back into the article body as a translucent chartreuse `<mark>` — re-derived from the
store on every load, so highlights survive reloads and restarts. The wrap runs on the
DOM's *text nodes* via a `Range` (whitespace-normalized), so a passage that crosses an
`<a>`/`<em>` boundary is marked correctly and the sanitized markup is never disturbed.
A side "Highlights" index lists every mark; each row jumps to its place in the text
(with a brief flash) or removes it. The store keeps a highlight as its text, not DOM
offsets — the pragmatic choice for a reading tool, and the reason re-render is a pure
function of `(article, saved passages)`.

**Frontend design — distinct layout, shared craft system.** Tract intentionally does
*not* reuse the master-detail "ledger" layout from the sibling apps. It shares the
design *system* (tokens, type scale, the chartreuse-highlighter signature) but takes
its own *layout archetype*: the library is a spatial **reading queue** (resume-first
cards), the reader is an editorial **index spread** (centered measure, drop-cap, a
live reading-progress hairline, a first-class highlights index). The reasoning: a
portfolio should read as one studio's hand (shared system) without every app
collapsing into the same skeleton (distinct layout). Reading progress is tracked
client-side so "Continue reading" reflects where you actually stopped — never a faked
bar. A deterministic anti-slop linter (`make lint`, wired into CI) gates the CSS: no
hover-lift, no AI-default gradients, tokenized shadows. UI copy is English by default
(this is a global product); a selectable UI language is a tracked follow-on.

## API

| Method | Path | Auth* | Purpose |
|--------|------|-------|---------|
| `POST` | `/api/items` | 🔒 | `{url}` → fetch, extract, store; returns the item |
| `GET` | `/api/items` | — | list items, newest first |
| `GET` | `/api/items/{id}` | — | full item (body + html + highlights) for the reader |
| `DELETE` | `/api/items/{id}` | 🔒 | delete an item (cascades highlights + FTS) |
| `GET` | `/api/search?q=` | — | FTS5 search over title + body |
| `POST` | `/api/items/{id}/highlights` | 🔒 | `{text}` → attach a highlight |
| `DELETE` | `/api/items/{id}/highlights/{hid}` | 🔒 | remove a highlight (item-scoped) |
| `GET` | `/api/health` | — | liveness probe |

\* 🔒 = requires `Authorization: Bearer $TRACT_TOKEN` when the token is
configured; open when it isn't. See [Security](#security).

## Run locally

Prereqs: Go ≥ 1.25 (per `go.mod`), Node ≥ 20 (CI builds on 22).

```bash
# one-shot: build frontend + binary, then run on :8080
make run
# open http://localhost:8080
```

Two-terminal dev loop (hot-reload frontend, live backend):

```bash
# terminal 1 — Go API on :8080
go run ./cmd/tract
# terminal 2 — Vite dev server, proxies /api to :8080
make frontend-dev
```

Environment: `TRACT_ADDR` (default `:8080`), `TRACT_DB` (default `tract.db`),
`TRACT_TOKEN` (default unset — see [Security](#security)). If `TRACT_ADDR` is
unset but `PORT` is (the convention PaaS platforms inject), Tract binds
`:$PORT` — so the same binary drops into a host with zero config.

## Security

**Tract has no accounts.** Access control is a single bearer token:

- **`TRACT_TOKEN` unset (default):** every route is open. That is the intended
  zero-config mode for `localhost` — and **only** for localhost. The server
  logs a loud warning at startup in this mode.
- **`TRACT_TOKEN` set:** every *mutating* route — `POST /api/items` (which also
  makes the server fetch a caller-supplied URL), `DELETE /api/items/{id}`, and
  both highlight routes — requires `Authorization: Bearer <token>`. Read-only
  `GET`s (list, item, search, health) stay open, so a public URL works as a
  read-only demo while writes stay yours.

> ⚠️ **A public deploy without `TRACT_TOKEN` is world-writable.** Anyone who
> finds the URL can delete your whole library and use `POST /api/items` to make
> your server issue requests on their behalf. If it's reachable from the
> internet, set the token — no exceptions.

Independent of the token, the article fetcher is hardened (always on, not
configurable off): it refuses to connect to private, loopback, link-local,
CGNAT and cloud-metadata addresses — checked at **dial time on every hop**, so
DNS names that resolve to internal ranges and redirect chains that end on
`169.254.169.254` are rejected the same as literal private URLs (this also
means Tract cannot save pages from your own LAN/intranet — by design). Fetched
pages are capped at 10 MiB so a hostile response can't OOM a small VM.

**Using the web UI against a protected deploy:** click the key button in the
top bar, paste the token, Save. It is kept in that browser's `localStorage`
(a chartreuse dot on the key marks it as set; Clear removes it) and sent as
the `Authorization: Bearer` header on mutating calls only — reads never carry
it, matching the server contract above. Until a token is set, saves/deletes
fail with a visible "this server requires an access token" error, and the API
works directly too:

```bash
curl -X POST https://<app>.fly.dev/api/items \
  -H "Authorization: Bearer $TRACT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://go.dev/blog/gob"}'
```

## Deploy

Tract is one CGO-free static binary with the frontend embedded, so it containers
to a distroless image with no libc and no shell. The included `Dockerfile` mirrors
the local build (frontend → embed → `CGO_ENABLED=0` build); it works on any
Docker host.

**Fly.io (recommended, $0):** the `fly.toml` provisions one scale-to-zero machine
plus a small volume so the SQLite library survives redeploys. **Set `TRACT_TOKEN`
before the first deploy** (see [Security](#security)):

```bash
fly launch --no-deploy               # confirm app name + region (defaults: Stockholm)
fly volumes create tract_data --region arn --size 1
fly secrets set TRACT_TOKEN=$(openssl rand -hex 32)   # REQUIRED for a public URL
fly deploy
# → https://<app>.fly.dev
```

**Any other host:** point it at the `Dockerfile` and mount a writable volume at
`/data` (that's where `TRACT_DB` defaults inside the image). Hosts that inject
`$PORT` (Render, Cloud Run) need no extra config — the server honors it.

## Tests

```bash
go test ./... -race   # store/FTS5 round-trip + all 7 HTTP handlers, falsify-first
go vet ./...
```

Coverage is falsify-first throughout — every test carries a load-bearing negative
(absent search term returns zero rows; delete actually unindexes; fetch failure
maps to 502 not a swallowed 500). The `internal/api` suite drives the real Go 1.22
method+path mux end-to-end, so the routing table itself is under test. CI
(`.github/workflows/ci.yml`) runs `go vet` + race tests and a frontend
typecheck+build on every push/PR.

## Roadmap

**Done:** layered Go service · pure-Go SQLite + FTS5 with trigger-kept index ·
readability extraction · bluemonday HTML sanitization before store/serve · all 8
endpoints wired · single-binary static serving · **select-to-highlight in the reader
with in-body chartreuse marks that survive reload, a highlights index, and delete** ·
editorial design (library "queue" + reader "index spread", light/dark, reading
progress) · anti-slop CSS linter in CI · FTS5 round-trip + full HTTP-handler suite
(positive + falsifying negatives, race-clean) · CI (vet + race tests + CSS lint +
frontend typecheck/build) · **selectable UI language (English default + Svenska,
qbar picker, locale-aware dates)** · containerized deploy (distroless static image
+ Fly config) · **hardening: `TRACT_TOKEN` bearer gate on all mutating routes,
dial-time SSRF guard (redirect- and DNS-safe) + 10 MiB fetch cap, self-contained
fresh-clone placeholder** · **web-UI token entry (key popover in both bars,
localStorage-persisted, `Bearer` on mutating calls only, honest 401 guidance)**.

**Next blocks:**
- **Tags & filtering** — organize the library beyond search.
- **Feeds (the Feedly leg)** — subscribe to RSS, auto-ingest into the library.
- **Hosting** — `Dockerfile` + `fly.toml` are in the repo (see [Deploy](#deploy));
  the remaining step is running the deploy against a host.

## License

Personal project. Not yet licensed for redistribution.
