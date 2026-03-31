# Forest: Implementation TODO

## Rules (apply to every task)

- **Test-driven:** write the test first, then the implementation. No code merges without passing tests.
- **Minimal code:** implement exactly what the task requires. No speculative abstractions, no extra configurability.
- **Docs in the same commit:** any task that changes behaviour must update README.md, IMPLEMENTATION.md, or inline godoc as appropriate. Documentation is not a follow-up task.
- **Go best practice:** follow the current Go style guide. `gofmt`, `go vet`, and `golangci-lint` must pass. Exported symbols get godoc. Errors are returned, not logged and swallowed.
- **Makefile is the interface:** every operation (build, test, lint, reindex, run) must have a Makefile target. No one-off shell commands in documentation.
- **GitHub Actions calls the Makefile:** each workflow step is `make <target>`. The workflow file is thin — no logic lives in YAML.
- **Branch per task, git worktree per branch:** each task is done on a feature branch created as a git worktree (`git worktree add ../forest-<task-name> -b <task-name>`). Merge to `master` via PR when tests pass and docs are updated.
- **Commit messages explain why**, not what. The diff explains what.
- **New external Go module imports require approval:** before adding any new entry to `go.mod` (other than the standard library), stop and ask. Present 5 alternatives — including a stdlib-only or zero-dependency option where one exists — with pros and cons for each. Wait for a choice before writing any code that imports the new module. This applies to indirect dependencies introduced by a new direct dependency: if a proposed module pulls in a large transitive graph, that must be disclosed as part of the comparison.

---

## Module and project details

- **Module:** `github.com/svenjigrun/forest`
- **Go version:** 1.24+ (use toolchain directive in go.mod)
- **Primary on-disk format:** Markdown + YAML frontmatter (source of truth); DuckDB is a persistent derived index, kept warm between runs and updated incrementally on file changes while the TUI is running
- **Embeddings:** Ollama optional — system degrades gracefully to BM25-only when Ollama is absent
- **CI:** GitHub Actions, each job calls a Makefile target

---

## Stage 0 — Scaffold

These tasks produce no user-visible functionality. They establish the
conventions everything else builds on.

### 0.1 — Initialise the Go module and Makefile

- `go mod init github.com/svenjigrun/forest`
- Makefile targets: `build`, `test`, `lint`, `vet`, `clean`
- `build` produces a single binary `./bin/forest`
- `lint` runs `golangci-lint run`
- CI workflow: `.github/workflows/ci.yml` with jobs `test`, `lint`, each calling the Makefile target
- **Acceptance:** `make build` produces a binary that prints `forest: no command given` and exits 1; `make test` passes (trivially, no tests yet); CI is green

### 0.2 — Define the node data model (Go structs + YAML/Markdown schema)

- Package `internal/node`: `Node`, `Link`, `Provenance`, `ContentType` types
- Document the Markdown + YAML frontmatter format in `docs/node-format.md`
- No storage yet — just the types and their (un)marshal logic
- Tests: round-trip marshal/unmarshal for all node types (prose, structured, executable, schema, context)
- **Acceptance:** `make test` passes; `node.Node` correctly round-trips a YAML frontmatter file

### 0.3 — ULID generation utility

- Package `internal/id`: `New() string` (generates a ULID), `IsValid(s string) bool`
- Dependency: `github.com/oklog/ulid/v2`
- Tests: uniqueness (1000 calls produce 1000 distinct IDs), monotonic order, valid format
- **Acceptance:** `make test` passes; IDs sort lexicographically in creation order

---

## Stage 1 — Node store (file system + DuckDB index)

All user data lives in Markdown + YAML files. DuckDB is a warm, persistent
index rebuilt from those files, updated incrementally while the app runs.

### 1.1 — File store: read and write nodes

- Package `internal/store`: `FileStore` — reads and writes node files from a
  configurable root directory (`nodes/`, `schemas/`, `contexts/`)
- `Put(node) error`, `Get(id string) (*node.Node, error)`, `List() ([]*node.Node, error)`
- Files named `<ULID>.md`, written to the appropriate subdirectory by content type
- Tests: create a temp dir, write a node, read it back, list it
- **Acceptance:** `make test` passes; no DuckDB dependency in this package

### 1.2 — DuckDB index: schema and initial load

- Package `internal/index`: `Index` wraps a DuckDB connection
- Schema: `nodes` table (id, content_type, content, created_at, recency_score),
  `edges` table (source_id, target_id, link_type), `contexts` table (node_id, context_tag)
- `Build(root string) error` — scans all `.md` files, parses YAML frontmatter,
  populates all three tables; idempotent (drops and recreates if called twice)
- DuckDB file stored at `<root>/.forest/index.duckdb`
- Dependency: `github.com/marcboeker/go-duckdb`
- Tests: build index from a fixture directory of 5 nodes; verify row counts; verify
  edges are present for nodes with links
- **Acceptance:** `make test` passes; `Build` on a clean dir is fast (<100ms for 5 nodes)

### 1.3 — Incremental index update (file-watch)

- `index.Watcher` — uses `fsnotify` to watch the node root for `.md` file changes
- On create/modify: parse the changed file, upsert the row in `nodes`, upsert edges
- On delete: remove the row and its edges
- Watcher runs in a goroutine; caller receives update events on a channel
- Dependency: `github.com/fsnotify/fsnotify`
- Tests: create a temp dir with 3 nodes; start watcher; write a new file; assert
  the index contains the new row within 500ms; modify a file; assert the row updated
- **Acceptance:** `make test` passes; no polling — event-driven only

### 1.4 — BM25 keyword search

- `index.Search(query string, limit int) ([]*node.Node, error)`
- Uses DuckDB's built-in full-text search (`PRAGMA create_fts_index`) over the
  `content` column
- Returns nodes ordered by BM25 score descending
- Tests: index 10 fixture nodes with known content; search for a term present in
  3 of them; assert correct nodes returned in score order
- **Acceptance:** `make test` passes; search returns results in <10ms on 10k node fixtures

### 1.5 — `forest init` and `forest reindex` CLI commands

- Package `cmd/forest`: cobra-based CLI (dependency: `github.com/spf13/cobra`)
- `forest init [dir]` — creates the directory layout (`nodes/`, `schemas/`,
  `contexts/`, `.forest/`) and writes a default `config.yaml`
- `forest reindex` — calls `index.Build`, prints progress, exits
- Makefile target `run` — `go run ./cmd/forest`
- Tests: `forest init` on a temp dir creates the expected layout; `forest reindex`
  on a pre-populated temp dir exits 0 and produces a valid DuckDB file
- **Acceptance:** `make test` passes; `./bin/forest init ./testforest && ./bin/forest reindex` works end-to-end

---

## Stage 2 — TUI: stream view and node writing

### 2.1 — Bubbletea application skeleton

- Package `internal/tui`: `App` struct implementing `tea.Model`
- Two-pane layout (Lipgloss): left = node stream, right = node detail
- Status bar showing active context label and Ollama availability indicator
- No data yet — render placeholder text in each pane
- Dependencies: `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`,
  `charm.land/glamour/v2`, `charm.land/bubbles/v2`
- Tests: model initialises without panic; `Update` handles `tea.WindowSizeMsg`
  correctly; rendered output contains the expected pane borders
- **Acceptance:** `make test` passes; `./bin/forest tui` opens a two-pane terminal UI
  that can be quit with `q` or `ctrl+c`

### 2.2 — Node stream list

- Left pane renders a scrollable list of nodes from `index.Search("", 50)`
  (all nodes, most recent first when no query)
- Each row: `[<ULID prefix>] <first 60 chars of content> (<link count> links)`
- Keyboard: `j`/`k` or arrow keys to move; `enter` selects (populates right pane)
- Glamour renders the selected node's Markdown content in the right pane
- Tests: model populated with 5 fixture nodes; arrow-key navigation selects
  the correct node; rendered right pane contains the node's content
- **Acceptance:** `make test` passes; live TUI shows real nodes from a `forest init`'d directory

### 2.3 — Inline node writing

- Writing surface at the bottom of the left pane (Bubbles `textarea` component)
- `n` key opens the write surface; `ctrl+s` saves; `esc` cancels
- On save: creates a new `node.Node` with a fresh ULID, writes the file via
  `FileStore.Put`, the `Watcher` fires and updates the index, the new node
  appears in the stream without restart
- Tests: simulate `n` keypress → textarea opens; simulate content + `ctrl+s` →
  a `.md` file is written to the temp dir with correct YAML frontmatter
- **Acceptance:** `make test` passes; user can write and save a node in the live TUI

### 2.4 — Context switcher

- `q` key opens a query input (Bubbles `textinput`) at the top of the left pane
- On submit: calls `index.Search(query, 50)` and re-renders the stream
- Active context label in status bar updates to show the current query string
- `esc` clears the query and returns to the full node list
- Tests: simulate a query → stream re-renders with filtered results; `esc` restores
  full list
- **Acceptance:** `make test` passes; context switching works in the live TUI with
  real data

### 2.5 — Node detail: links and version history

- Right pane adds two sections below the content: `LINKS` and `HISTORY`
- `LINKS`: lists each outbound link (type + target ULID prefix); pressing `l`
  then a link number navigates to that node
- `HISTORY`: reads the git log for the node's file
  (`git log --oneline -- nodes/<id>.md`); shows last 5 commit summaries
- Tests: fixture node with 2 links renders both in the links section; git log
  output is parsed correctly from a temp git repo
- **Acceptance:** `make test` passes; links and history visible in the live TUI

---

## Stage 3 — Semantic search (Ollama, optional)

### 3.1 — Ollama client

- Package `internal/ollama`: `Client` with `Embed(ctx, text string) ([]float32, error)`
  and `Generate(ctx, model, prompt string) (string, error)`
- Pure Go HTTP client (`net/http` only); no CGO
- `NewClient(baseURL string) *Client` — defaults to `http://localhost:11434`
- `IsAvailable(ctx context.Context) bool` — HEAD request to `/api/tags`; used
  for the graceful-degradation check
- Tests: uses `httptest.NewServer` to mock Ollama responses; tests embed, generate,
  and the unavailable path
- **Acceptance:** `make test` passes; package compiles with `CGO_ENABLED=0`

### 3.2 — Embedding store (sqlite-vec)

- Add `embedding` column (BLOB) to the DuckDB `nodes` table
- `index.Embed(client *ollama.Client) error` — iterates nodes missing embeddings,
  calls `client.Embed`, stores the float32 slice as a binary blob
- `index.SemanticSearch(embedding []float32, limit int) ([]*node.Node, error)` —
  cosine similarity over stored embeddings; returns top-k results
- When Ollama is unavailable, `SemanticSearch` returns an error and callers fall
  back to BM25
- Makefile target: `make embed` — runs embedding for all un-embedded nodes
- Tests: store 5 nodes with known content; embed them against a mock Ollama;
  semantic search returns the most similar node for a test query embedding
- **Acceptance:** `make test` passes; `CGO_ENABLED=0 go build ./...` succeeds

### 3.3 — Hybrid search (BM25 + semantic)

- `index.HybridSearch(query string, embedding []float32, limit int)` — runs BM25
  and semantic search in parallel (goroutines), merges results with a weighted
  sum (0.5 BM25 + 0.5 semantic), deduplicates, returns top-k
- When embedding is nil (Ollama absent), delegates to BM25 only
- Tests: 10 fixture nodes; verify merged results contain the correct top node;
  verify nil-embedding path returns BM25 results unchanged
- **Acceptance:** `make test` passes; TUI status bar shows `[semantic on]` or
  `[keyword only]` depending on Ollama availability

### 3.4 — Context inference

- On TUI startup and on each new node write: embed the current writing buffer
  content (if Ollama is available) and call `HybridSearch` with no explicit query
- The inferred context label in the status bar shows `auto: <top matching node prefix>`
- User can override at any time with an explicit query (`q` key from Stage 2.4)
- Tests: mock Ollama returns a fixed embedding; assert the inferred context label
  updates after a simulated write
- **Acceptance:** `make test` passes; inferred context visible in the live TUI

---

## Stage 4 — AI provider abstraction

Uses `charm.land/fantasy` (Tier 2 from IMPLEMENTATION.md §13) as the provider
abstraction. This brings `charm.land/fantasy` and Charm's own Anthropic/OpenAI
SDK forks (`github.com/charmbracelet/anthropic-sdk-go`,
`github.com/charmbracelet/openai-go`) as `go.mod` entries. All provider
implementations (Anthropic, OpenAI-compatible, Gemini, Bedrock, OpenRouter)
come from `charm.land/fantasy/providers/*`; Forest does not write HTTP client
code for individual providers.

### 4.1 — Provider config

- Package `internal/provider`: `Config` loaded from `config.yaml` under the
  forest root; maps model aliases to `charm.land/fantasy` provider+model pairs
- `NewProvider(cfg Config) (fantasy.ChatLanguageModel, error)` — resolves a
  model alias from the config and returns the appropriate `fantasy` provider
- Default config includes entries for `claude-sonnet-4-6` (Anthropic),
  `gpt-4o` (OpenAI), `local` (Ollama via the OpenAI-compatible provider)
- Dependency: `charm.land/fantasy` — **requires approval per the new-module
  rule above; present this task's module comparison before adding to go.mod**
- Tests: config round-trips YAML; `NewProvider` returns the correct fantasy
  provider type for each alias; missing alias returns a clear error
- **Acceptance:** `make test` passes; `CGO_ENABLED=0 go build ./...` succeeds

### 4.2 — OpenAI-compatible provider wiring

- Wire Ollama, GitHub Models, and OpenRouter through
  `charm.land/fantasy/providers/openai` with a custom `BaseURL` option — all
  three speak the OpenAI wire format
- Add config entries for each in the default `config.yaml`
- Tests: mock server using `httptest`; verify each alias resolves to the OpenAI
  fantasy provider with the correct base URL set
- **Acceptance:** `make test` passes; `forest prompt --model local "hello"` works
  against a running Ollama

### 4.3 — Anthropic provider wiring

- Wire Claude through `charm.land/fantasy/providers/anthropic`
- Falls back gracefully if `ANTHROPIC_API_KEY` is unset (clear error, no panic)
- Tests: mock server; test complete and stream paths; test missing API key path
- **Acceptance:** `make test` passes; `forest prompt --model claude-sonnet-4-6 "..."` works with a real API key in the environment

### 4.4 — Prompt node execution

- `forest exec <node-id>` — reads an executable node whose `content_type` is
  `application/x-prompt+llm`; resolves its input nodes from the store; builds
  the prompt; calls the configured fantasy provider; writes the output as an
  enrichment (version bump on the existing node, not a new node) pending user
  review
- Output shown as a diff in the TUI right pane with `[a]ccept / [e]dit / [d]iscard`
- Tests: fixture prompt node + fixture input nodes; mock fantasy provider returns
  known output; assert the enrichment diff is correct; assert accept writes the
  new version
- **Acceptance:** `make test` passes; end-to-end prompt node round-trip works

---

## Stage 5 — Federation and publishing

### 5.1 — `forest serve` HTTP server

- Package `cmd/forest serve`: starts a local HTTP server (default `:8080`)
- `GET /nodes/<id>` — content negotiation:
  - `text/html` → rendered HTML page for the node
  - `application/activity+json` → ActivityPub Object stub
  - `application/ld+json` → JSON-LD representation
  - `application/json` → FedWiki page JSON
- `GET /nodes` — returns a JSON array of all node IDs
- Tests: `httptest` against the handler; each content type returns the correct
  response and status code
- **Acceptance:** `make test` passes; `curl -H "Accept: application/json" http://localhost:8080/nodes/<id>` returns valid FedWiki JSON

### 5.2 — `forest publish` static export

- `forest publish --output ./public` — writes one FedWiki JSON file per node to
  `public/pages/<id>.json`; copies the FedWiki client JS to `public/`; writes
  `public/index.html`
- Makefile target: `make publish`
- Tests: run publish on a fixture forest; assert the expected files exist; assert
  each JSON file parses as a valid FedWiki page with correct journal entries
- **Acceptance:** `make test` passes; the output directory opens correctly in a
  browser using the FedWiki client

### 5.3 — ActivityPub actor and object endpoints

- `GET /.well-known/webfinger` — returns the WebFinger resource for the local actor
- `GET /actor` — returns the ActivityPub Actor JSON for the local Forest instance
- `GET /nodes/<id>` (ActivityPub content type) — full ActivityPub Object with
  correct `@context`, `type: Note` (or custom `forest:Node` type), and `url`
- `POST /inbox` — accepts `Follow` activities; logs them (no full delivery yet)
- Tests: each endpoint returns correctly structured JSON-LD; WebFinger resolves
  the actor; inbox accepts a well-formed Follow and returns 202
- **Acceptance:** `make test` passes; Mastodon's webfinger lookup tool resolves
  the actor correctly

---

## Stage 6 — DuckDB as query surface for structured nodes

### 6.1 — Structured node ingestion

- `content_type: application/json` nodes: their content is parsed and stored as a
  JSON column in DuckDB alongside the text content
- `forest query --sql "SELECT ..."` — executes read-only SQL against the DuckDB
  index; prints results as a table
- Makefile target: `make query SQL="..."`
- Tests: ingest 3 JSON-content nodes; SQL query returns the correct rows; `INSERT`,
  `UPDATE`, `DROP` are rejected
- **Acceptance:** `make test` passes; `forest query --sql "SELECT id FROM nodes WHERE content_type = 'application/json'"` works

### 6.2 — External reference nodes

- `content_type: forest/external-ref` nodes: YAML frontmatter contains a `source_url`
  and an optional DuckDB SQL `query` field
- `forest materialize <node-id>` — resolves the source URL (Parquet, CSV, or
  remote DuckDB), executes the query using DuckDB's `httpfs` extension, prints the
  result as a table
- Tests: fixture external-ref node pointing at a public Parquet URL; materialise
  returns the expected row count (requires network; skip with `go test -short`)
- **Acceptance:** `make test` passes (short mode); full test works against a real Parquet URL

---

## Deferred (not staged yet)

These are real tasks but belong after the stages above are working:

- **Executable node sandbox (wazero):** run `content_type: application/x-python`
  nodes in a WASM sandbox; requires deciding on Pyodide vs native wazero approach
- **Schema nodes and uplift:** schema versioning, uplift instructions, conflict UX
- **Ageing and recency decay:** implement the recency signal on nodes; surface the
  score in TUI and search ranking
- **Mobile / remote access via `wish`:** expose the TUI over SSH using
  Charmbracelet's `wish` library
- **hugot opt-in build tag:** in-process embedding via `github.com/knights-analytics/hugot`
  for users who accept CGO; gated behind `-tags hugot`
- **Multi-device sync:** CRDT-based sync of the node file tree across devices
- **ActivityPub full delivery:** outbox, signed HTTP requests, remote Follow/Accept

---

## Makefile reference (target list)

| Target | Purpose |
|---|---|
| `make build` | Compile `./bin/forest` |
| `make test` | Run all tests (`go test ./...`) |
| `make lint` | Run `golangci-lint run` |
| `make vet` | Run `go vet ./...` |
| `make clean` | Remove `./bin` |
| `make run` | Build and run `./bin/forest` |
| `make reindex` | Run `./bin/forest reindex` against `./testforest` |
| `make embed` | Run `./bin/forest embed` against `./testforest` |
| `make publish` | Run `./bin/forest publish --output ./public` |
| `make query SQL=...` | Run `./bin/forest query --sql "$(SQL)"` |
| `make ci` | Run `test lint vet` in sequence (what CI runs) |

## GitHub Actions reference

`.github/workflows/ci.yml` — runs on push and pull_request to `master`:

```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - run: make test

  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - run: make lint
```

No logic in YAML. All behaviour is in the Makefile.
