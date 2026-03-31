# Forest: Implementation Decisions

This document surveys the key technical decisions required to bring Forest to
life, comparing five options for each. It prioritises decisions that cascade
into later choices — getting these wrong is expensive to reverse.

---

## 1. Primary Language and Runtime

The language choice shapes the embedding story, the execution sandbox, the
desktop/server split, and how easily the system hosts multiple runtimes inside
executable nodes.

| Option | Prior use | Fit | Trade-offs |
|---|---|---|---|
| **Python** | Jupyter, Marimo, Hugging Face ecosystem | High for AI/data layer | Slow startup, GIL limits parallelism, packaging friction for end-users |
| **Rust** | Tauri, Oxigraph, DuckDB internals | High for core store | Steep learning curve, long compile cycles; but native WASM compilation path is excellent |
| **Go** | CockroachDB, Gitea, many CLI tools | Medium | Excellent concurrency, cross-compile, small binaries; embedding models need CGO or ONNX bridge |
| **TypeScript (Node/Bun)** | Obsidian (Electron), Observable | Medium | Ubiquitous; V8 is the natural JS runtime for executable nodes; ecosystem fragmentation |
| **Elixir/Erlang (OTP)** | LiveView, Livebook | Medium-High | Actor model maps well to node lifecycle; OTP supervision fits a long-running personal server; BEAM is hard to embed in desktop |

**Likely path:** A **Rust core** (node store, graph traversal, embedding
index) exposed via FFI or gRPC to a **Python layer** (AI/embedding models,
data pipelines) and a **TypeScript UI** (stream renderer, canvas, executable
JS nodes). This matches how Tauri + Python backends are increasingly structured
in 2025 AI-native apps. The cost is managing three language boundaries; the
payoff is not being constrained by any one ecosystem.

**Impact on executable nodes:** The host runtime for each executable node type
maps to whichever language layer is most natural. SQL and JSONata run inside
the Rust core. Python nodes spawn a managed Python worker. JS/TS nodes run in
V8 (already present for the UI). WASM nodes run in a sandboxed WASM runtime
(Wasmtime in Rust, no context escape).

---

## 2. Local Graph Store

The store is the canonical source of truth for nodes, edges, provenance, and
version history. This choice directly affects query capability, portability,
and the complexity of the sync/federation layer.

| Option | Prior use | Fit | Trade-offs |
|---|---|---|---|
| **SQLite + adjacency tables** | Obsidian (SQLite for search), Notion (Electron + SQLite), many mobile apps | High for prototype | Portable, zero-dependency; recursive CTEs handle moderate graph traversal; graph query depth is limited |
| **DuckDB** | Data notebooks, local OLAP, DuckLake | High for structured/query-heavy nodes | Columnar OLAP; excellent for external-reference nodes and structured data; not a graph store natively; best as a secondary engine rather than primary |
| **Kuzu** | Academic graph DB, increasingly used for RAG pipelines | High | Native property graph + Cypher; embedded, no server; Rust bindings; relatively new but actively developed |
| **Oxigraph** | Semantic web tooling, SPARQL endpoints | Medium | Native RDF/SPARQL; aligns with JSON-LD wire format; SPARQL is verbose; federation story is strong |
| **SQLite-vec / LanceDB** | Vector search layers for SQLite, local RAG | Low as primary | Excellent as an index layer on top of a graph store; not appropriate as the primary graph store alone |

**Likely path:** **SQLite as the primary store** (nodes table, edges table,
context memberships, version log) plus **DuckDB as a secondary engine** for
external-reference nodes and structured data queries. SQLite-vec or a small
FAISS index handles embeddings. When the graph grows complex enough to justify
it, Kuzu is the natural upgrade path for the graph traversal layer — and its
schema is close enough to the SQLite adjacency model that migration is feasible.

**Impact on future ideas:** Choosing SQLite now keeps the local-first,
zero-infrastructure property intact. Choosing DuckDB as the secondary engine
immediately enables the DuckLake external-reference story from the README
without any additional infrastructure. Schema nodes, record-per-node datasets,
and whole-dataset nodes all fall out of a well-designed SQLite schema with JSON
columns for node content.

---

## 3. Embedding and Vector Index

Relevance ranking, context inference, and the graph-first AI synthesis flow all
depend on fast, local vector search over node embeddings. The model and index
choice determines latency, accuracy, and offline capability.

| Option | Prior use | Fit | Trade-offs |
|---|---|---|---|
| **nomic-embed-text (local, GGUF/ONNX)** | Nomic Atlas, local RAG pipelines | High | Strong quality for its size; runs locally via llama.cpp or ONNX Runtime; 768-dim vectors; no API cost |
| **all-MiniLM-L6-v2 (sentence-transformers)** | Standard baseline for local semantic search; used in many open-source RAG systems | High | Smaller, faster, slightly lower quality; 384-dim; widely tested; available via ONNX |
| **OpenAI text-embedding-3-small** | Many SaaS RAG products | Medium | High quality; API latency; cost at scale; breaks offline-first; useful as a quality ceiling for evaluation |
| **SQLite-vec (sqlite-vec extension)** | Recently merged into SQLite as a first-party extension (2024) | High as index | Not a model — this is the index layer. Keeps everything in one file; no separate FAISS process; excellent for single-user scale |
| **FAISS (Facebook AI Similarity Search)** | Hugging Face, many embedding pipelines | Medium | Battle-tested; more operationally complex than sqlite-vec for single-user; better at billion-scale, which is not our problem |

**Likely path:** **nomic-embed-text via ONNX Runtime** (Rust bindings available
via ort) as the embedding model, **sqlite-vec** as the index. Both run locally,
both have a path to a remote fallback (OpenAI or Voyage AI) when online and
when higher quality matters. The 768-dim vectors from nomic-embed-text fit
comfortably in sqlite-vec at single-user scale (tens of thousands of nodes).

**Impact on future ideas:** The ageing recency signal can be composed with the
embedding similarity score at query time — a simple weighted product. Context
inference (infer the active context from what the user is writing) is just
nearest-neighbor over the context node embeddings. Both come for free once the
embedding layer is in place.

---

## 4. Node Identity and Addressing Scheme

Node IDs are permanent. They appear in URLs, in federation links, in
transclusion expressions, and in version history. The choice of ID format
determines sortability, collision resistance, federation mechanics, and how
"provisional local" IDs are promoted to globally addressable URLs.

| Option | Prior use | Fit | Trade-offs |
|---|---|---|---|
| **ULID (Universally Unique Lexicographic ID)** | Increasingly adopted in modern databases, event stores (e.g. Svix, Stytch) | High | Monotonically sortable (time-ordered); URL-safe; 128-bit; no central registry; human-inspectable prefix; easy to generate in any language |
| **UUIDv7** | Proposed standard (RFC 9562, 2024); adopted by PostgreSQL 17, MySQL 8.4 | High | Same time-ordered property as ULID; formal RFC backing; slightly less readable than ULID; broad library support |
| **Content-hash (SHA-256 prefix)** | IPFS CIDs, Git object IDs | Medium | Immutable by construction; content-addressed; but changes on every edit (versioning requires indirection); poor fit for a mutable node with history |
| **ActivityPub-style URL as ID** | Mastodon, ActivityPub objects generally | Medium-High | URL is the canonical ID; no separate UUID needed; but provisional local nodes need a non-URL placeholder until published; adds complexity to the local-first story |
| **Automerge/CRDT document ID** | Automerge, Yjs, local-first software patterns | Low as primary | Excellent for sync; but CRDT doc IDs are not human-readable or URL-mappable without additional indirection |

**Likely path:** **ULID as the local ID**, promoted to a full URL when the node
is published. The local form is `local:01HXZJ...`; the published form is
`https://alice.example/nodes/01HXZJ...`. The ULID becomes the path segment.
This preserves time-ordering (useful for version history and the ageing signal),
avoids UUID's lowercase-hex readability problem, and composes naturally with the
ActivityPub URL-as-ID model when federation is added.

**Impact on future ideas:** ULID's time-ordering means that a range scan over
node IDs is approximately a time scan — useful for the temporal context queries
("show me what I was thinking in Q1 2026"). Merge identity (which ID survives
when two nodes merge) is solved simply: the older ULID survives; the newer
ULID becomes a redirect, forever.

---

## 5. Federation and Sync Protocol

Forest nodes need to be shareable, citable from other instances, and
optionally synced across a user's own devices. These are different problems
that may or may not share a protocol.

| Option | Prior use | Fit | Trade-offs |
|---|---|---|---|
| **ActivityPub (W3C standard)** | Mastodon, Lemmy, PeerTube, Pixelfed | High | JSON-LD wire format matches the node model; Actor/Object/Activity maps to User/Node/Enrichment; inbox push delivery; extensible vocabulary; requires an HTTP server per instance |
| **AT Protocol (Bluesky)** | Bluesky Social, Jetstream | Medium | Stronger identity model (DIDs, PDS); repo-level sync via CAR files; lexicons for custom record types; more centralised identity assumptions; newer and less stable |
| **IPFS / Content Addressing** | Filecoin, IPFS websites, Fission WNFS | Low-Medium | Content-addressed permanence is appealing; but IPFS has poor latency for small nodes, no push delivery, and mutability is a second-class concept |
| **Git-inspired append-only log** | Dolt, Fossil, Pijul; local-first research | Medium | Well-understood; offline-first; merge semantics are familiar; but no built-in push delivery or actor model; would need to layer federation on top |
| **CRDTs over a sync server (Automerge/Loro + relay)** | Linear, Liveblocks, ElectricSQL | Medium | Excellent for real-time multi-device sync; handles conflicts automatically; but CRDT merge semantics may conflict with the "enrich, don't proliferate" principle |

**Likely path:** **ActivityPub as the inter-instance federation protocol**,
**CRDTs (Automerge or Loro) for multi-device sync of a single user's instance**.
These are different use cases. Federation (sharing with others) benefits from
ActivityPub's push model and JSON-LD semantics. Personal sync (laptop +
mobile) benefits from CRDT's automatic conflict resolution. The ActivityPub
inbox and outbox become the public face; the CRDT log is internal plumbing.

**Impact on future ideas:** ActivityPub's vocabulary extension mechanism
(custom `@context` entries) is the path to typed link relationships beyond
`inReplyTo` — the `cites`, `contradicts`, `corroborates`, and
`transcluded-from` types from the README are vocabulary extensions, not
protocol changes. Schema nodes and context nodes as ActivityPub Object types
follow the same path. The web-as-superset vision (every node is also a
human-readable web resource) is immediately achievable: the node URL returns
HTML for browsers and JSON-LD for ActivityPub clients via content negotiation.

---

## 6. UI Framework and Rendering

The rendering layer must handle a stream of mixed-content nodes (prose,
structured data, code, chart output), a force-directed canvas mode, live
transclusion, and inline editing. It must feel fast on a local graph.

| Option | Prior use | Fit | Trade-offs |
|---|---|---|---|
| **Tauri (Rust + WebView, TypeScript frontend)** | Growing desktop app ecosystem; Zed editor uses a custom Rust UI; Tauri is used by many local-first apps | High | Rust core + TypeScript UI boundary is clean; small binaries; WebView renders HTML, so existing web rendering work transfers; hot reload; weaker on Linux WebView consistency |
| **Electron (Node.js + Chromium)** | Obsidian, VS Code, Notion Desktop | High | Battle-tested; full Chromium means rendering consistency; large bundle size; memory heavier than Tauri; but Obsidian has proven it works for this exact use case |
| **Web app (progressive, local-first)** | Notion web, Roam Research | Medium | No install friction; but local graph access requires IndexedDB or OPFS (limited); background embedding computation is constrained; offline-first is harder |
| **Bevy / native GUI (egui, Slint)** | Game engines, Zed (GPUI) | Low-Medium | Best performance headroom; weakest for richly formatted text; not appropriate for the primary reading/writing surface; potentially relevant for canvas mode |
| **React Native / Flutter** | Mobile cross-platform apps | Low for desktop-first | Good for mobile companion; not the right primary surface for a desktop knowledge graph tool |

**Likely path:** **Tauri for the desktop application**, with a **React (or
Solid.js) + CodeMirror frontend**. Tauri gives access to the Rust core via
IPC, keeps bundle size small, and allows the UI to be a pure TypeScript
concern. CodeMirror handles the writing surface (it powers many modern editors
and supports WASM extensions, which matters for executable node editing).
Canvas mode is a separate panel using a force-directed graph library (Cytoscape.js
or a lightweight D3 force layout).

**Impact on future ideas:** The Tauri/TypeScript split means the UI and the
core can evolve independently. The JS node runtime for executable nodes is
already present in the WebView — a JS executable node is just an evaluated
script whose output is rendered into the node's slot in the stream. Observable
Plot or Vega-Lite can render chart nodes without any additional runtime.

---

## 7. Executable Node Sandbox

Executable nodes introduce arbitrary code. The sandbox determines what that
code can access and what damage it can do.

| Option | Prior use | Fit | Trade-offs |
|---|---|---|---|
| **WebAssembly (Wasmtime/Wasm-bindgen)** | Fastly Compute, Cloudflare Workers, Extism, most modern plugin systems | High | Language-portable; capability-restricted by construction; no ambient filesystem or network unless explicitly granted; Rust bindings are mature; compilation adds friction for ad-hoc scripts |
| **Deno (permission flags)** | Deno Deploy, many sandboxed script runners | High for JS/TS | Fine-grained permissions (--allow-read, --allow-net with specific hosts); already runs TypeScript; isolated V8 contexts; limited to JS/TS runtimes |
| **Docker / OCI containers** | Jupyter remote kernels, CI sandboxing, Code Interpreter | Medium | Language-agnostic; heavyweight for interactive use; slow startup; not appropriate for low-latency node execution |
| **OS process isolation (pledge/seccomp/sandbox-exec)** | Many Unix tools, browser sandboxing (Chromium), VS Code extension host | Medium | Platform-specific; seccomp on Linux is powerful but complex to configure correctly; no portable story |
| **Pyodide (Python in WASM)** | JupyterLite, Observable, Shinylive | High for Python specifically | Python running inside WASM; no system access; broad library support; slower than native Python; solves the sandbox + Python notebook problem in one move |

**Likely path:** **Wasmtime for the general sandbox** (any language that
compiles to WASM — Rust, C, Go, AssemblyScript), **Deno for JS/TS nodes**
(fine-grained permissions, already TypeScript-native), **Pyodide for Python
nodes** that need sandboxing. Native Python (via subprocess) remains available
for user-authored nodes with explicit trust elevation. This is a layered trust
model that matches the trust levels described in the README.

**Impact on future ideas:** The Extism plugin system (built on Wasmtime)
provides a structured way for community-contributed node runtimes to be
installed and sandboxed without modifying the Forest core. This is the path
to the "open runtime list" described in the README — a runtime is just a WASM
plugin that knows how to execute a content type.

---

## 8. AI Integration and Prompt Node Execution

The AI layer has three roles: embedding (already covered above), graph-first
synthesis (enrichment proposals), and prompt node execution. The model access
pattern affects cost, latency, privacy, and offline capability.

| Option | Prior use | Fit | Trade-offs |
|---|---|---|---|
| **Claude API (Anthropic)** | This project; Claude Code; many agentic apps | High | Best reasoning quality; structured output; tool use for diff generation; API cost; requires internet |
| **Ollama (local model runner)** | Common in local RAG stacks; used by Obsidian AI plugins, Continue.dev | High for offline-first | Runs models locally (Llama 3, Mistral, Gemma 2); no API cost; latency depends on hardware; quality ceiling below frontier models |
| **llama.cpp + GGUF models** | Most local model runners are built on this | Medium | Lower-level than Ollama; more control; less tooling; useful when embedding and generation share the same runtime |
| **OpenAI API** | Ubiquitous baseline | Medium | Quality comparable to Claude; same internet-dependency tradeoffs; less fit for agentic structured output patterns |
| **Dual-track (local for ambient, remote for deliberate)** | Pattern used by Cursor, Copilot, many hybrid AI apps | High | Ollama for context inference and enrichment suggestions (low-latency, privacy-preserving); Claude API for explicit prompt node execution (high quality, user-initiated) |

**Likely path:** **Dual-track**. The ambient AI layer (context inference,
embedding, enrichment suggestions as the user types) runs locally via Ollama
with a small, fast model. Explicit prompt node execution — where the user has
authored a prompt node and deliberately runs it — uses the Claude API (or any
configured remote model). This preserves the privacy and offline properties
for ambient use while giving the deliberate AI layer access to frontier model
quality.

**Impact on future ideas:** Prompt nodes store the model declaration alongside
the prompt template. A prompt node authored against Claude today can be
re-executed against a future local model if the quality gap closes. The model
is a parameter, not a dependency. This also means the system can execute
prompt nodes in federated contexts: another user can receive a prompt node,
inspect its model declaration, and decide whether to execute it against their
own model configuration.

---

## 9. Context Engine Implementation

The context engine is what makes Forest different from a search engine or a
note-taking app. It must maintain an active context, rank nodes continuously,
compose multiple context signals, and degrade gracefully when signals conflict.

| Option | Prior use | Fit | Trade-offs |
|---|---|---|---|
| **Weighted vector composition** | Standard in RAG re-ranking; used in multi-signal search (Pinecone hybrid search, Weaviate) | High | Combine embedding similarity + recency score + explicit context tags as a weighted sum; simple, fast, tunable; may not capture non-linear relationships |
| **Learning to rank (LTR) model** | Bing, Google, many search systems; used in Obsidian's AI search experiments | Medium | Can learn from user interaction which signals matter most; requires interaction data to train; cold-start problem is severe |
| **Graph-structured ranking (PageRank-style)** | Google's original PageRank; used in citation networks, knowledge graphs | Medium | Ranks nodes by link centrality within the active subgraph; captures "important" nodes naturally; not responsive to user's current writing signal |
| **BM25 + vector hybrid** | Elasticsearch, Vespa, many open-source RAG stacks | High | Keyword precision (BM25) plus semantic recall (vector); standard hybrid search pattern; well-understood degradation; sqlite-vec supports this pattern |
| **Rule-based filter + embedding re-rank** | Simple but effective pattern used in many personal tools | Medium-High | Apply explicit filters first (context tags, time range, provenance type), then re-rank the filtered set by embedding similarity; highly interpretable; fast on small graphs |

**Likely path:** **Rule-based filter + BM25 + embedding re-rank**, with recency
and ageing scores as multiplicative modifiers. Start with the simplest thing
that works: filter by explicit context signals, keyword-rank the results,
re-rank by embedding similarity, multiply by the recency score. This is
interpretable (the user can see exactly why a node is ranked where it is),
fast at prototype scale, and has a clear upgrade path to learning-to-rank
once there is interaction data.

**Impact on future ideas:** The "active context is always visible and mutable"
UX principle requires that each component of the ranking be exposable in the
UI. A pure neural ranker makes this hard. The rule-based + vector hybrid is
transparently decomposable: "this node ranks here because it matches your
keyword, is semantically similar to what you're writing, and was last touched
6 weeks ago."

---

## 10. Portability and Data Ownership

The user's graph must be exportable, restorable, and not dependent on Forest
being running to be readable. This affects the file format, the version
history representation, and how much of the graph's meaning survives outside
the application.

| Option | Prior use | Fit | Trade-offs |
|---|---|---|---|
| **Markdown + YAML frontmatter (per-node files)** | Obsidian, Jekyll, Hugo, Foam | High for human-readability | Every node is a readable file; git-friendly; version history is git history; loses graph structure without the application; doesn't handle structured data nodes well |
| **JSON-LD (per-node or collection)** | ActivityPub wire format; schema.org; Linked Data ecosystem | High for semantic completeness | Machine-readable graph semantics; full link and provenance preservation; less human-readable without tooling; the correct format for federation anyway |
| **SQLite file as the canonical format** | Datasette, many mobile apps, Fossil (which uses SQLite for everything including VCS) | High for single-file portability | One file is the entire instance; trivially copyable, restorable, and inspectable with standard tooling; not human-readable without SQL |
| **Append-only event log (JSON Lines)** | Event sourcing systems, Kafka, Dolt | Medium | Every change is a recorded event; the current state is derived by replaying the log; excellent audit trail; more complex to query |
| **Git-backed node directory** | Foam, Dendron, Logseq | Medium | Version history is git history; human-readable files; merge conflicts are a real risk for automated enrichment; git is not a graph database |

**Likely path:** **SQLite as the canonical operational store**, with **JSON-LD
export as the portability format**. The SQLite file is the thing the user backs
up and copies between devices. The JSON-LD export is the thing they share,
federate, or import into another system. Markdown + YAML frontmatter is an
*additional* export target for nodes that are prose — useful for publishing to
a static site or for Obsidian interop — but it is not the source of truth.

**Impact on future ideas:** Fossil (the version control system) uses SQLite as
its entire repository format and is inspectable with standard SQL tools. This
is a proven model for a portable, inspectable, self-contained knowledge store.
A Forest instance is just a SQLite file; moving it is a file copy; inspecting
it is a SQL query. The JSON-LD export ensures that the semantic content — the
typed links, the provenance, the schema nodes — is not locked inside the SQL
schema but is representable in an open, standard format.

---

## 11. Go-First Stack Variant

The sections above favour a Rust core. Go is a credible alternative that
trades raw performance for far faster iteration, simpler cross-compilation,
and a richer ecosystem for the HTTP/federation surface. This section
re-examines the stack through a Go-first lens.

| Option | Prior use | Fit | Trade-offs |
|---|---|---|---|
| **Go as the sole backend language** | CockroachDB, Gitea, Mattermost, age (encryption), many CLI/server tools | High | Single binary, `go build` cross-compiles to Linux/macOS/Windows/ARM with no fuss; goroutines map well to per-node async work; CGO is the friction point for embedding models |
| **Go core + Python subprocess for AI** | Pattern used by many Go tools that need ML (e.g. Sourcegraph's Cody backend) | High | Go owns the store, graph, HTTP server; Python subprocess handles embedding and LLM calls over a local socket; clean boundary; no CGO required for the AI path |
| **Go + ONNX Runtime via CGO** | Used in production Go ML pipelines | Medium | Embedding models (nomic-embed-text, MiniLM) run natively in-process via ONNX; eliminates the Python subprocess; CGO complicates cross-compilation |
| **Go + Wasm host (wazero)** | Wazero is a pure-Go WASM runtime; used in several Go plugin systems | High | No CGO; sandboxes executable nodes in Go-native WASM; wazero is mature and actively maintained; smaller runtime than Wasmtime but no JIT (slower for compute-heavy nodes) |
| **Go + templ/htmx for the web UI** | Used in several Go-native web apps as an alternative to a JS SPA | Medium | Eliminates the TypeScript layer; server-side rendering with htmx for interactivity; much simpler to reason about; limited for the canvas mode and live transclusion |

**Likely path (Go variant):** **Go as the sole backend**, with **wazero for
executable node sandboxing**, **`charm.land/fantasy` (or a vendored copy of
crush's routing layer) for multi-provider AI** (see section 13), and either
a **Charmbracelet TUI** (see section 12) or a minimal **templ/htmx web UI**
for early prototyping. The single-binary story is compelling: `forest` ships
as one executable, embeds the SQLite or DuckDB store, and runs a local HTTP
server. No installer, no runtime dependency, no virtual environment.

Go's standard library covers the HTTP server, JSON handling, and concurrent
node operations. The `database/sql` interface works with both SQLite
(`mattn/go-sqlite3` via CGO, or `modernc/sqlite` in pure Go) and DuckDB
(`marcboeker/go-duckdb`). The federation HTTP handlers are idiomatic Go.
Embedding models are the one remaining gap: either a Python subprocess over
a Unix socket, or CGO via ONNX Runtime (`yalue/go-onnxruntime`). The Python
path keeps the main binary CGO-free; the ONNX path keeps it single-process.

**Impact on future ideas:** The Go variant's single-binary nature makes it
easier to ship Forest as a tool that runs alongside other developer tools —
similar to how `age`, `mkcert`, or `gh` are installed and forgotten. A
Go-native Forest is also easier to embed in other systems (a CI pipeline,
a home server, a NAS) than a multi-runtime Rust/Python/TypeScript stack.

---

## 12. TUI Frontend (Charmbracelet)

A terminal UI is not a consolation prize for the absence of a GUI. For a
knowledge tool used primarily by developers and power users, a TUI is often
the right first surface: no install friction, scriptable, composable with
shell workflows, and operable over SSH. The Charmbracelet suite (Bubbletea,
Lipgloss, Glamour, Bubbles) makes idiomatic Go TUIs viable.

| Option | Prior use | Fit | Trade-offs |
|---|---|---|---|
| **Bubbletea (Charmbracelet) + Lipgloss** | Charm CLI tools, `gum`, `soft-serve`, many modern Go TUIs | High | Elm-architecture model (update/view/message); composable components; Lipgloss for styled layout; Glamour for rendered Markdown; active community; pairs naturally with a Go backend |
| **tview (rivo/tview)** | `k9s`, several data-inspection TUIs | Medium | Widget-based; mature; easier to learn than Bubbletea for simple layouts; less composable for complex reactive UIs; not as actively styled |
| **Textual (Python)** | `posting`, `rich`, several Python CLI tools | Medium | Rich layout, CSS-like styling, async-native; but Python runtime dependency; out of place in a Go stack |
| **Ink (React in the terminal, Node.js)** | Many Node.js CLI tools | Low | React component model is familiar; Node.js runtime dependency; not the right fit for a Go-native tool |
| **Raw termbox / tcell** | Low-level foundations for tview, Bubbletea | Low as primary | Maximum control; requires building all abstractions from scratch; only warranted if Bubbletea's model proves limiting |

**TUI layout for Forest's core interactions:**

The stream mode maps naturally to a TUI split pane:

```
┌─────────────────────────────────┬───────────────────────────┐
│ CONTEXT: project:forest + Q2    │ NODE DETAIL               │
│─────────────────────────────────│───────────────────────────│
│ [01HXZJ] The paragraph is the   │ ID: 01HXZJ...             │
│   atom of meaning. #3 links     │ Created: 2026-03-31       │
│                                 │ Links: cites (2)          │
│ [01HWAB] Purple Numbers assign  │ Contexts: project:forest  │
│   persistent IDs... #1 link     │                           │
│                                 │ CONTENT                   │
│ [01HW99] Executable nodes let   │ The paragraph is the atom │
│   code live in the graph...     │ of meaning. Context is a  │
│                                 │ lens, not a folder.       │
│ > [cursor: write new node]      │                           │
│                                 │ LINKED NODES              │
│ Context: [q]uery [l]ocation     │ → cites 01HWAB            │
│          [t]ime [s]ave [f]ork   │ → cites 01HW55            │
└─────────────────────────────────┴───────────────────────────┘
```

Glamour renders Markdown in the detail pane. Lipgloss handles borders and
colour theming. The context switcher is a Bubbles `textinput` component at
the bottom of the left pane.

**Likely path:** **Bubbletea + Lipgloss + Glamour** as the primary early
interface. The TUI exposes the full node lifecycle (write, enrich, link, split,
merge), the context engine (active context, query, save, fork), and the version
history viewer. Canvas mode (the 2D force-directed layout) is deferred — a TUI
cannot render it usefully, but a TUI can print a text-format graph summary
(adjacency list, depth-limited tree) that serves the same orientation purpose
at early prototype scale.

**Impact on future ideas:** A TUI-first approach means the core is driven
entirely by a well-defined interface (the local HTTP API or direct Go function
calls), which makes adding a web UI, a desktop GUI, or a mobile client
later purely additive — the surface is already proven. Charmbracelet's
`wish` library (SSH server for TUI apps) means a Forest instance can be
accessed remotely over SSH without any additional web server infrastructure.

---

## 13. AI Provider Abstraction (Multi-Provider Routing)

Prompt nodes declare a model, not an API key. The system must route a prompt
node's execution to whichever provider the user has configured, without
Forest's core knowing about each provider's SDK. This is the multi-provider
abstraction problem.

### What charmbracelet/crush does (and doesn't) offer

Crush (https://github.com/charmbracelet/crush) is a TUI coding assistant
written in Go, actively developed (v0.53.0, March 2026), and solves exactly
this problem for its own use. It is **not an importable library** — all
routing logic lives under `internal/` — but it is the clearest Go reference
implementation available, and its architecture is worth understanding and
potentially copying.

Crush's provider layer rests on two private Charm libraries:

- **`charm.land/fantasy`** — an abstract `LanguageModel` interface with
  concrete provider implementations for Anthropic, OpenAI, Google Gemini,
  AWS Bedrock, OpenRouter, and Vercel AI Gateway. This is the Go equivalent
  of LangChain's LLM abstraction, built by Charm specifically for their tools.
- **`charm.land/catwalk`** — a provider registry/catalogue that Crush fetches
  at startup (with a local cache fallback) to discover available models and
  their capabilities.
- **`github.com/charmbracelet/anthropic-sdk-go`** and
  **`github.com/charmbracelet/openai-go`** — Charm's own forks of the
  Anthropic and OpenAI Go SDKs, used as the HTTP clients under the hood.

The routing layer itself is small — approximately 870 lines across five files:

| File | Lines | Role |
|---|---|---|
| `internal/app/provider.go` | 95 | Model string parsing (`provider/model` syntax), match/validate |
| `internal/config/provider.go` | 231 | Config loading, provider list caching, concurrent Catwalk + Hyper fetch with 45s timeout + cache fallback |
| `internal/config/catwalk.go` | 82 | Catwalk sync and cache management |
| `internal/config/hyper.go` | 124 | Charm Hyper (managed inference) sync |
| `internal/agent/hyper/provider.go` | 338 | Hyper provider implementation: `Generate()`, `Stream()`, error handling (402/429/401) |

### Can this code be copied into Forest?

**License:** Crush is under **FSL-1.1-MIT** (Functional Source License). This
permits use in non-competing products. FSL-1.1 automatically converts to full
MIT two years after each release — so any code from early 2025 releases is
already MIT; code from the current (2026) releases converts in 2028. Forest
is not a competing product to a TUI coding assistant, so the FSL restriction
does not apply. Attribution is still required.

**Practical assessment:**

The routing layer is modular and self-contained enough to copy, but carries
two dependencies that need a decision:

1. **`charm.land/fantasy`** — this is the load-bearing abstraction. Options:
   - Import it directly (it is a public Go module at `charm.land/fantasy`);
     Forest gets all providers for free and tracks upstream changes.
   - Copy and adapt it; Forest owns the interface and can trim to only the
     providers it needs.
   - Write a thinner equivalent (see below); simpler but loses Charm's
     provider implementations.

2. **`charm.land/catwalk`** — the provider registry is a Charm-operated
   service. Forest almost certainly does not want to depend on an external
   registry for its provider list. Replace with a local config file.

**Recommended approach — three tiers:**

*Tier 1 (minimal, ~100 lines):* Copy `internal/app/provider.go`'s model
string parsing logic only. Implement your own `Provider` interface with a
`Generate(ctx, prompt, opts) (string, error)` and
`Stream(ctx, prompt, opts) (<-chan string, error)` signature. Write one
concrete implementation per provider shape.

*Tier 2 (standard, ~400 lines):* Import `charm.land/fantasy` directly as a
Go module dependency. Copy and adapt `internal/config/provider.go` for
config loading and caching, replacing the Catwalk fetch with a local YAML
config file. This gives Forest Anthropic, OpenAI, Gemini, Bedrock, and
OpenRouter implementations for free.

*Tier 3 (full):* Copy all five files listed above, replace the Catwalk/Hyper
service calls with local config, and keep the Charm Hyper provider only if
Forest plans to offer managed inference as a feature.

### Comparison of provider options

| Option | Prior use | Fit | Trade-offs |
|---|---|---|---|
| **Import `charm.land/fantasy` directly** | Used by crush and other Charm tools | High | Public Go module; covers Anthropic, OpenAI, Google, Bedrock, OpenRouter, Vercel; maintained by Charm; couples Forest to Charm's release cycle |
| **Copy and adapt crush's routing layer (~400 lines)** | As above, but vendored | High | Forest owns the code; no upstream coupling; ~400 lines to maintain; FSL-1.1-MIT allows this; Catwalk dependency replaced with local config |
| **OpenAI-compatible HTTP direct + thin Anthropic adapter** | Ollama, LM Studio, LocalAI, vLLM, GitHub Models, OpenRouter all speak OpenAI wire format | High | Minimal dependency; two HTTP client shapes cover ~95% of the market; pure Go; no Charm coupling; requires writing the Anthropic adapter (~100 lines) |
| **LiteLLM as a local sidecar proxy** | Widely used in Python LLM stacks; supports 100+ providers | Medium | Moves all adapter logic out of Go entirely; adds a Python process; useful if a Python subprocess is already running for embeddings; operational overhead |
| **openrouter.ai as the sole provider gateway** | OpenRouter aggregates Claude, GPT-4o, Gemini, Llama, Mistral via one OpenAI-compatible API | Medium | Single API key, single endpoint, zero adapter code; pay-per-token; loses offline/local model support; user must trust a third-party gateway |

**Likely path:** **Tier 2 — import `charm.land/fantasy` and adapt crush's
config loading layer**, replacing Catwalk with a local YAML config. This gives
Forest a working multi-provider layer in a day rather than a week, with a
clear upgrade path to Tier 1 (write own interface) if the Charm dependency
proves limiting. The config file maps model aliases to providers:

```yaml
models:
  default: claude-sonnet-4-6
  providers:
    claude-sonnet-4-6:
      provider: anthropic
      api_key: $ANTHROPIC_API_KEY
    gpt-4o:
      provider: openai
      api_key: $OPENAI_API_KEY
    local-llama:
      provider: ollama          # OpenAI-compatible
      base_url: http://localhost:11434/v1
    openrouter-mixtral:
      provider: openrouter
      api_key: $OPENROUTER_API_KEY
    github-models:
      provider: openai          # GitHub Models is OpenAI-compatible
      base_url: https://models.inference.ai.azure.com
      api_key: $GITHUB_TOKEN
```

Providers that speak the OpenAI wire format (`ollama`, `github-models`,
`openrouter`) need no adapter — `charm.land/fantasy/providers/openai` or a
direct `go-openai` client handles them. Anthropic's native API shape (different
auth header, `system` field handling) is covered by
`charm.land/fantasy/providers/anthropic`.

**Impact on future ideas:** A prompt node's model declaration is just a key
from this config file. A user can share a prompt node via ActivityPub; the
receiving instance executes it against *their own* configured model, not the
sender's. The model is a hint, not a hard dependency. Per-prompt-node cost
tracking and A/B testing across model quality on the same template both fall
out of this routing layer with minimal additional work.

---

## 14. Markdown + YAML as Canonical Portability Format

Section 10 treats Markdown + YAML frontmatter as an additional export target.
This section argues for treating it as the **primary on-disk format** —
the thing the user can read, edit, and version-control independently of Forest
running — with the operational store (SQLite or DuckDB) as a derived index
over it.

This is the Obsidian/Foam/Dendron model, but applied consistently to *all*
node types, not just prose.

| Option | Prior use | Fit | Trade-offs |
|---|---|---|---|
| **Markdown + YAML frontmatter as source of truth** | Obsidian, Foam, Dendron, Logseq (mostly), Jekyll, Hugo | High for prose nodes | Human-readable, git-friendly, editor-agnostic; YAML frontmatter carries ID, links, contexts, provenance; the file *is* the node; version history is git history; doesn't handle structured/executable nodes cleanly |
| **SQLite as source of truth, Markdown as export** | Notion (proprietary DB → Markdown export), Bear (SQLite → export) | High for operational use | Faster queries; no file system scan on startup; but the graph is opaque without the app; export is a second-class citizen and often lags behind the operational format |
| **YAML-only (no Markdown body)** | Configuration systems, Kubernetes manifests | Low | Structured nodes (schema nodes, context nodes) are YAML-native; prose nodes are not; splitting the format by node type creates two conventions |
| **JSON frontmatter + Markdown body** | Some static site generators; MDX | Medium | More precise than YAML; less human-writable; JSON doesn't support multi-line values elegantly; most tooling expects YAML frontmatter |
| **Plain Markdown with inline syntax for metadata** | Logseq (properties as `key:: value`), Org-mode | Medium | Zero frontmatter overhead; metadata is in-band; but non-standard; hard to parse reliably; conflicts with Markdown renderers that don't know the convention |

**File layout for a Markdown + YAML canonical store:**

```
forest/
  nodes/
    01HXZJ.md        ← prose node
    01HWAB.md
    01HW99.md        ← executable node (runtime declared in frontmatter)
  schemas/
    contact-v1.md   ← schema node (YAML frontmatter + description body)
  contexts/
    project-forest.md  ← context node
  index.db           ← derived: sqlite index for fast query + embeddings
  index.duckdb       ← derived: duckdb for structured/external-ref queries
```

A prose node file:

```markdown
---
id: 01HXZJ
content_type: text/markdown
created_at: 2026-03-31T09:14:00Z
provenance:
  source: user
links:
  - type: cites
    target: 01HWAB
  - type: transcluded_from
    target: https://bob.example/nodes/01HWAB
contexts:
  - project:forest
  - time:2026-Q1
---

The paragraph is the atom of meaning.
```

An executable node file:

```markdown
---
id: 01HW99
content_type: application/x-python
created_at: 2026-04-01T10:00:00Z
runtime: python
trust: user
inputs:
  - 01HWAB
triggers: on-input-change
---

```python
import forest
nodes = forest.query(context="project:forest", limit=10)
print([n.id for n in nodes])
```


**Likely path (Markdown-primary variant):** Markdown + YAML frontmatter as the
**source of truth on disk**, with SQLite and DuckDB as **derived indexes**
rebuilt from the file tree on startup or on file-system watch events.
Version history is git — `git log nodes/01HXZJ.md` shows every enrichment.
The operational store is a cache, not the record; losing it is never
catastrophic because `forest reindex` regenerates it.

This trades some query performance (a cold index rebuild is slower than an
always-live SQLite store) for full transparency and editor-agnosticism. A user
can edit a node file in Vim, and Forest picks up the change. A user can `grep`
their graph without the app. A user can push their graph to a Git remote and
have full history without any Forest-specific sync infrastructure.

**Impact on future ideas:** The Markdown-primary model composes naturally with
the federated wiki publishing surface (section 15 below). Publishing a node
is a git push; the federation layer reads the published files and wraps them
in ActivityPub. Schema nodes and context nodes are YAML-heavy files that read
awkwardly as Markdown but remain human-inspectable. Executable nodes store
their source in a fenced code block — the file is both the node record and a
runnable script that any editor with language support can syntax-highlight.

---

## 15. HTML Publishing and Federated Wiki UI

When Forest nodes are published, they need a web representation. The question
is not just static HTML generation — it is the interaction model for the
published surface. Ward Cunningham's Federated Wiki (FedWiki / Smallest
Federated Wiki) offers a specific and underexplored model that aligns closely
with Forest's own design principles.

| Option | Prior use | Fit | Trade-offs |
|---|---|---|---|
| **Federated Wiki (Ward Cunningham's SFW/Fedwiki)** | Fedwiki.org; used in research and education communities; inspired several personal knowledge tools | High | Page-as-JSON with paragraph-level forks and attributions; side-by-side multi-site navigation; content travels with attribution when forked; the "journal" is an append-only edit log per page; aligns with Forest's node history model |
| **Static site generator (Hugo, Eleventy, Astro)** | GitHub Pages, many personal sites, digital gardens | Medium | Simple, fast, widely understood; one-way publish; no federation; no paragraph-level addressability in the published output |
| **Datasette (SQLite → web)** | Datasette is used by journalists and researchers to publish SQLite databases as browsable, queryable web sites | High for structured nodes | One command publishes the SQLite store as a searchable web interface; excellent for structured data nodes; not designed for prose reading |
| **Single-page app served from the local HTTP server** | Roam Research's published graphs, Obsidian Publish | Medium | The Forest TUI/GUI renders the same data; publish is just making the local server publicly accessible; but no offline-readable static output |
| **ActivityPub + HTML via content negotiation** | Mastodon's web profiles, any ActivityPub server with a web UI | High | The node URL returns HTML for browsers and JSON-LD for ActivityPub clients; this is the minimal viable "published node" — no separate publishing pipeline needed |

**The FedWiki model in detail:**

Federated Wiki represents each page as a JSON document containing an array of
"items" (paragraphs, images, code blocks) and a "journal" of changes. Pages
are forked by copying the JSON; the fork retains the original's attribution.
Navigation is side-by-side: opening a link opens the linked page *alongside*
the current one, building a left-to-right lineage of context.

This maps onto Forest with little distortion:

- A Forest node is a FedWiki page (one item per Forest node, or a context
  view as a multi-item page).
- The node's version history is the FedWiki journal.
- Transcluding a node is forking it — the transclusion carries attribution
  back to the source URL.
- The side-by-side navigation model is Forest's context stream rendered
  spatially: each context is a column, linked nodes open to the right.

A Forest publishing pipeline targeting FedWiki output:

```
forest publish --format fedwiki --output ./public

public/
  pages/
    01hxzj.json    ← FedWiki page JSON for this node
    01hwab.json
  index.html       ← FedWiki client app (the standard SFW client)
  status/
    01hxzj         ← ActivityPub actor/object for this node (content negotiation)
```

The FedWiki client is a JavaScript single-page app that reads the JSON pages
and renders the side-by-side view. It is small (~50KB), self-contained, and
already handles the fork/attribution model. Forest's publish step generates
the JSON; the FedWiki client renders it.

**Likely path:** **ActivityPub + HTML via content negotiation as the primary
federation mechanism**, with **FedWiki-format JSON as the static publishing
target** for read-oriented public sites. The two are not in conflict: the
same node URL can return:

- `text/html` → a human-readable page with the FedWiki client embedded
- `application/activity+json` → the ActivityPub Object for federation
- `application/ld+json` → the JSON-LD representation for semantic consumers
- `application/json` → the FedWiki page JSON for FedWiki clients

Content negotiation routes to the right representation. A Forest instance
that is publicly accessible handles all four. A static export (for hosting on
a CDN or GitHub Pages) handles the first and third via separate files at
predictable paths.

**Impact on future ideas:** The FedWiki side-by-side navigation model is a
natural complement to Forest's stream mode. When a user is reading published
nodes from another Forest instance, the side-by-side view shows the source
instance's context alongside their own — directly visualising the federation.
Cunningham's "neighbourhood" concept (the set of sites a FedWiki installation
watches and can fork from) maps directly to Forest's follow/subscribe model
over ActivityPub. The two systems are, in effect, expressing the same idea
through different implementations; building Forest's publishing layer on
FedWiki's output format is an homage and a practical choice simultaneously.

---

## Decision Dependencies

The choices above are not independent. A few critical chains:

**Rust-first stack:**

```
Language choice (Rust core + Python + TypeScript)
  → SQLite + DuckDB store (Rust SQLite bindings are mature)
  → sqlite-vec for embeddings (same process as the store)
  → Tauri for the desktop shell (Rust → WebView IPC)
  → Wasmtime for sandbox (already in the Rust process)
```

**Go-first stack (§11):**

```
Go single binary
  → modernc/sqlite (pure Go, no CGO) or go-duckdb (CGO, worth it for DuckDB)
  → charm.land/fantasy (or vendored crush routing layer) for multi-provider AI (§13)
  → Python subprocess over Unix socket for embedding models only (no LLM traffic)
  → wazero for executable node sandbox (pure Go WASM, no CGO)
  → Bubbletea TUI (§12) as the primary early interface
  → templ/htmx web UI as a lightweight browser alternative
```

**AI provider abstraction (§13):**

```
OpenAI-compatible HTTP interface as internal contract
  → Config file maps model names → provider base URLs
  → Claude, OpenAI, GitHub Copilot/Models, OpenRouter, Ollama all supported
  → LiteLLM sidecar handles non-OpenAI-compatible providers (Anthropic native API)
  → Prompt nodes store model name, not provider — portable across instances
```

**Markdown-primary portability (§14):**

```
Markdown + YAML frontmatter as source of truth on disk
  → SQLite + DuckDB are derived indexes (rebuilt via `forest reindex`)
  → Version history is git — no custom VCS needed
  → Editor-agnostic: any text editor can read/write nodes
  → Feeds directly into the FedWiki publishing pipeline (§15)
```

**Publishing surface (§15):**

```
ActivityPub + content negotiation as the federation layer
  → text/html     → FedWiki client + page JSON
  → application/activity+json → ActivityPub Object
  → application/ld+json → JSON-LD for semantic consumers
  → Static export targets GitHub Pages / CDN without a running server
  → FedWiki "neighbourhood" maps to ActivityPub follow/subscribe
```

**ULID node IDs (all stacks):**

```
ULID as local ID, promoted to URL on publish
  → Time-ordered scans for temporal context queries
  → Natural merge identity (older ULID survives)
  → Clean path to ActivityPub URL-as-ID
  → Stable path segment in FedWiki JSON filenames
```

The two viable full stacks are:

| Concern | Rust-first | Go-first |
|---|---|---|
| Core language | Rust | Go |
| Store | SQLite + DuckDB | SQLite (modernc) + DuckDB (CGO) |
| Embeddings | ONNX in-process | Python subprocess |
| Sandbox | Wasmtime | wazero |
| Primary UI | Tauri + React | Bubbletea TUI |
| Web UI | TypeScript SPA | templ/htmx |
| AI routing | Direct SDK calls | `charm.land/fantasy` or vendored crush routing (§13) |
| Portability | JSON-LD primary, Markdown additional | Markdown+YAML primary, JSON-LD for federation |
| Publishing | ActivityPub + content negotiation | FedWiki JSON + ActivityPub |

The Go-first stack is the lower-friction path to a working prototype. The Rust-
first stack has higher performance headroom and a more coherent desktop story.
Neither forecloses the other: the Go stack's local HTTP API is the same
interface the Rust stack would expose, so a rewrite of the core is possible
without changing the federation or UI layers.

---

## Deferred Decisions

These are real decisions that do not need to be made before a working
prototype:

- **Graph query language for power users**: Cypher vs. SPARQL vs. a custom
  query language. Defer until traversal depth actually requires it.
- **Mobile client**: The desktop prototype should expose a local HTTP API.
  A mobile client is a separate application that speaks to that API.
- **Multi-user collaboration within a single instance**: ActivityPub handles
  cross-instance federation; real-time multi-user editing of shared nodes is
  a separate problem (CRDTs, OT). Defer past single-user.
- **Schema evolution tooling UX**: The uplift mechanism is designed; the UX
  for resolving ambiguous uplifts is not. Defer to when there are real schema
  nodes to evolve.
- **Billing / hosting model**: Entirely deferred. Relevant only once federation
  is working and users want public instances.
