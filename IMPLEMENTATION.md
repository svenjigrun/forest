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

## Decision Dependencies

The choices above are not independent. A few critical chains:

```
Language choice (Rust core + Python + TypeScript)
  → SQLite + DuckDB store (Rust SQLite bindings are mature)
  → sqlite-vec for embeddings (same process as the store)
  → Tauri for the desktop shell (Rust → WebView IPC)
  → Wasmtime for sandbox (already in the Rust process)

ULID node IDs
  → Time-ordered scans for temporal context queries
  → Natural merge identity (older ULID survives)
  → Clean path to ActivityPub URL-as-ID

ActivityPub federation
  → JSON-LD wire format is already the portability format
  → Content negotiation: HTML for browsers, JSON-LD for AP clients
  → Vocabulary extensions cover typed links, schema nodes, context nodes

Dual-track AI (local + remote)
  → Ollama for ambient (privacy, offline, low latency)
  → Claude API for deliberate prompt node execution
  → Model declaration in prompt nodes = model is a parameter
```

The Rust core is the load-bearing choice. Everything else has multiple valid
options; Rust (or its absence) determines the embedding story, the sandbox
story, and the desktop story simultaneously. If Rust is off the table, the
fallback is Go for the core + Python for AI + Electron for the shell —
a slightly heavier but equally coherent stack.

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
