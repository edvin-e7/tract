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
don't 404 when served from a subpath. A committed placeholder `index.html` keeps
the embed directive valid before the first frontend build; `scripts/build.sh`
stages the real build over it.

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

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/items` | `{url}` → fetch, extract, store; returns the item |
| `GET` | `/api/items` | list items, newest first |
| `GET` | `/api/items/{id}` | full item (body + html + highlights) for the reader |
| `DELETE` | `/api/items/{id}` | delete an item (cascades highlights + FTS) |
| `GET` | `/api/search?q=` | FTS5 search over title + body |
| `POST` | `/api/items/{id}/highlights` | `{text}` → attach a highlight |
| `DELETE` | `/api/items/{id}/highlights/{hid}` | remove a highlight (item-scoped) |
| `GET` | `/api/health` | liveness probe |

## Run locally

Prereqs: Go ≥ 1.22, Node ≥ 18.

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

Environment: `TRACT_ADDR` (default `:8080`), `TRACT_DB` (default `tract.db`).

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
frontend typecheck/build).

**Next blocks:**
- **Tags & filtering** — organize the library beyond search.
- **Selectable UI language** — English default is wired; add the lightweight i18n
  layer + language picker (the shared clipboard-manager pattern).
- **Feeds (the Feedly leg)** — subscribe to RSS, auto-ingest into the library.
- **Hosting** — deploy the single binary to a live URL.

## License

Personal project. Not yet licensed for redistribution.
