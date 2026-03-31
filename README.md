# Purple Extended: A Pageless, Context-Driven Information Space

## Origin: Purple Numbers

Eugene Kim's (eekim) Purple Numbers project tackled one of hypertext's oldest
frustrations: links point to pages, but meaning lives in paragraphs. Purple
Numbers assigned a persistent, addressable identifier to every paragraph in a
document — rendering as small superscript anchors — so that any unit of
thought could be linked to, cited, or transcluded with precision. The URL
fragment was the pointer; the paragraph was the atom.

This document explores what happens when you take that insight and follow it
to its logical conclusion: **if the paragraph is the atom, then the page is
just an accidental container**. Remove the container. What remains is a living
graph of addressable thoughts, visible and traversable through context rather
than navigation.

---

## The Core Idea

There are no pages. There is only information:

- written directly by the user
- linked in from external sources (URLs, feeds, APIs)
- pulled on demand from outside systems (queries, integrations)

What the user sees at any moment is determined not by a URL or a folder, but
by a **context** — a lens that filters, surfaces, and arranges the
information space. Contexts are malleable and composable:

- a new paragraph the user is writing right now
- a location (home, office, transit, a specific city)
- a time or recurrence (morning review, project deadline week)
- a freeform query ("what did I think about X?", "show me everything from last Tuesday")
- a mood, a project, a person, a tag

The same piece of information can appear in many contexts. No piece of
information belongs exclusively to any context. There is no filing. There is
only relevance.

---

## Architecture

### The Information Unit

The base unit is the **node**: a paragraph-or-smaller chunk of text (or
media) with:

- a globally unique, persistent ID (a UUID or content-hash)
- a creation timestamp and edit history
- zero or more typed outbound links (cites, responds-to, contradicts,
  transcluded-from, imported-from)
- zero or more context tags (auto- and user-assigned)
- provenance metadata (written-by-user, imported-from-url, generated-by-query)

Nodes are immutable once written; edits create new versions that point back
to the prior version. The graph is append-only at the node level.

### The Graph Store

The canonical store is a **directed labeled graph** — not a document store,
not a relational database, not a key-value cache (though those may be used as
indexes). Nodes are vertices; links and context-membership are edges.

Suitable backing stores:

| Option | Fit | Notes |
|---|---|---|
| RDF triple store (e.g. Apache Jena, Oxigraph) | High | Native graph semantics; SPARQL for complex context queries |
| Property graph (e.g. Neo4j, Kuzu) | High | Richer edge properties; Cypher is more legible than SPARQL |
| SQLite + adjacency tables | Medium | Portable; sufficient for single-user; limited at graph traversal scale |
| CRDTs over a local-first store (e.g. Automerge + IndexedDB) | Medium | Enables offline-first and sync; graph queries require a layer on top |

For a single-user local-first prototype, **SQLite with a nodes table and an
edges table** is the right starting point. Graph query complexity only
justifies a dedicated store once traversal depth and cross-context ranking
matter at scale.

### Sync and Portability

Nodes carry enough metadata to be exported as JSON-LD or as plain Markdown
with YAML frontmatter, preserving IDs and link structure. The user owns their
graph; sync is an optional layer (e.g. a CRDT-backed sync server or a
git-like log of append operations).

---

## Data Storage

### Node Schema (simplified)

```json
{
  "id": "01HXZJ...",
  "content": "The paragraph is the atom of meaning.",
  "content_type": "text/markdown",
  "created_at": "2026-03-31T09:14:00Z",
  "versions": ["01HXZJ...-v0"],
  "provenance": {
    "source": "user",
    "imported_url": null
  },
  "links": [
    { "type": "cites", "target_id": "01HWAB..." },
    { "type": "transcluded_from", "target_id": "01HWAB...", "target_url": "https://example.com/post#pNNN" }
  ],
  "contexts": ["project:purple", "location:home", "time:2026-Q1"]
}
```

### Context Schema

A context is itself a node — it is just a node whose primary function is to
act as a filter and a view. It holds:

- a query expression (structured or natural language)
- optional spatial constraints (geofence, named place)
- optional temporal constraints (recurrence rule, date range)
- a display configuration (sort order, density, visible node types)

Contexts compose: a context can include other contexts by reference, and can
be saved, shared, or forked.

### External Ingestion

External information is pulled and stored as nodes with provenance. An
ingestion pipeline:

1. resolves the external URL / API response
2. segments content at the paragraph/block level
3. assigns each segment a node ID
4. preserves the source URL and the original paragraph anchor (Purple Number
   or equivalent) as part of provenance
5. stores the node in the graph, marked `provenance.source = "external"`

Ingested nodes are not shown by default in all contexts — they are available
for search and transclusion, but surface contextually.

---

## AI Layer

The AI layer is not a chatbot bolted on. It is a first-class part of the
context system, doing three things:

### 1. Contextual Relevance Ranking

When a context is active, an embedding model scores every candidate node for
relevance to the current context signal (the paragraph being written, the
query, the location-time conjunction). This is standard semantic search over
node embeddings, but the query is dynamic and multi-signal.

- Embeddings: stored per-node, updated on edit
- Model: a small, fast local embedding model (e.g. `nomic-embed-text`,
  `all-MiniLM-L6-v2`) is preferred for latency; a remote model for higher
  quality when online
- Index: approximate nearest-neighbor (FAISS, SQLite-vec, or hnswlib) over
  the embedding store

### 2. On-Demand External Retrieval

When the user asks a question or pastes a URL, the AI layer:

- decides whether the answer likely exists in the local graph (retrieval) or
  needs to be fetched externally (search + ingest)
- issues structured queries against the graph and/or calls external
  APIs/Claude API
- returns the answer as a new node (with provenance marking it AI-generated),
  linked to the source nodes it synthesized from

The Claude API (`claude-sonnet-4-6` or `claude-haiku-4-5`) is the right
model here: capable enough to synthesize across multiple retrieved nodes,
fast enough for interactive use.

```
User writes paragraph → embeddings computed →
  nearest neighbors retrieved from graph →
    passed as context to Claude API →
      response returned as a new candidate node →
        user accepts / edits / discards
```

### 3. Context Inference

When the user has not explicitly set a context, the system infers one from:

- the content of what the user is currently writing (embedding similarity)
- device signals (location, time of day, calendar)
- recent interaction history (what nodes the user has touched in the last
  N minutes)

Inferred context is always surfaced transparently ("Showing: similar to what
you're writing + project:purple") and can be overridden at any point.

---

## Rendering

### No Pages, No Navigation Hierarchy

The UI presents a **stream or canvas of nodes** shaped by the active context.
There is no sidebar tree, no breadcrumb, no back button in the traditional
sense. Orientation comes from the context label, not from a path.

Two primary layout modes:

**Stream mode** (default, prose-oriented)  
Nodes render in chronological or relevance order as a scrollable, readable
column. Each node shows its ID anchor (the Purple Number equivalent) and its
outbound links as inline affordances. Writing a new node inserts it at the
cursor position; it immediately participates in the graph.

**Canvas mode** (spatial, exploration-oriented)  
Nodes are positioned on a 2D canvas. Linked nodes are drawn near each other
by a force layout. The user can pin, cluster, and annotate. Canvas state is
itself a context — a saved spatial arrangement is a view that can be
returned to.

### Transclusion

Any node can be transcluded into any other node with a `{{node-id}}`
reference. The transcluded content renders inline, live (always reflects the
current version of the source node), and is visually distinguished (a subtle
border or tint). Edits happen at the source; the transclusion updates
everywhere.

This is Ted Nelson's transclusion, finally cheap enough to be practical.

### Context Switcher

A persistent, minimal control (keyboard shortcut + small HUD) lets the user:

- see the active context(s)
- add a signal to the context (type a query, drop a location pin, pick a
  time window)
- save the current context as a named view
- fork the context to explore a branch of the graph without losing the
  current view

### Progressive Disclosure

Nodes render at low information density by default (first ~60 words + link
count). Expanding a node shows the full content, version history, provenance,
and linked nodes. This keeps the stream scannable without hiding depth.

---

## UX Principles

**1. Writing is the primary interaction.**  
The user writes and the system responds. Every action — querying, linking,
ingesting, context-switching — can be initiated from the writing surface.
There are no separate "search" or "import" screens.

**2. Context is always visible and always mutable.**  
The user is never confused about why they are seeing what they see. The active
context is shown. It can be changed at any time. Changing it is cheap and
reversible.

**3. Nothing is lost and nothing is hidden permanently.**  
Deleted nodes are versioned, not destroyed. Archived nodes are accessible via
query. The graph grows; it does not shrink. The user's past thinking is always
reachable if they know how to ask for it.

**4. Links are first-class citizens.**  
Creating a link between two nodes is as lightweight as typing. The system
suggests links as the user writes (based on embedding similarity to existing
nodes). The user can accept, reject, or retype the relationship label.

**5. The AI is an assistant, not an author.**  
AI-generated nodes are always marked. The user decides whether to accept them
into the graph. The system never silently modifies the user's own nodes.

**6. External information is a guest.**  
Nodes imported from external sources are clearly distinguished from
user-authored content. The user's graph is not polluted by ingestion; external
nodes participate in context and search but are visually and semantically
separable.

---

## Open Questions / Design Tensions

- **Identity and addressability across devices**: Purple Numbers assumed a
  single canonical document. In a distributed, sync'd graph, what does a
  stable, shareable node address look like? Content-hashing, UUIDs, or
  something cryptographic?

- **Context explosion**: If contexts are composable and saved, the user may
  accumulate hundreds of them. How do you manage contexts without recreating
  the folder problem that pages represented?

- **Latency of relevance ranking**: Embedding-based ranking over a large
  local graph must be fast enough to feel live. At what node count does this
  break? What is the degradation strategy?

- **The blank canvas problem**: A pageless system with no hierarchy gives new
  users no affordance for where to start. Onboarding must solve for this
  without recreating pages by another name.

- **Provenance and trust**: If external content and AI-generated content live
  in the same graph as user-authored content, the visual and semantic
  distinction must be robust enough to survive context-switching and
  transclusion.

---

## Relationship to Prior Work

| Work | What it contributed | What's left to extend |
|---|---|---|
| Purple Numbers (Kim) | Paragraph-level addressability | Apply to a living graph, not static documents |
| Xanadu (Nelson) | Transclusion, bidirectional links | Make it local-first, cheap, and fast |
| Roam Research | Bi-directional block references, daily notes as context | Remove the page; make context dynamic not structural |
| Notion | Rich blocks, databases, views | Remove the hierarchy; unify document and database |
| Memex (Bush) | Associative trails as navigation | Make the trails automatic and multi-signal |
| Semantic Web | Machine-readable typed links | Keep the semantics, remove the bureaucracy |
| Obsidian | Local-first, markdown, graph view | Make the context dynamic; make the AI integral, not a plugin |

---

## Next Steps

1. **Prototype the node store**: SQLite schema for nodes, edges, and contexts.
   Implement append-only versioning.
2. **Build the ingestion pipeline**: URL → paragraph segmentation → node
   creation with provenance.
3. **Wire embedding index**: Per-node embeddings + FAISS or SQLite-vec for
   nearest-neighbor retrieval.
4. **Stream renderer**: A minimal single-column node stream that renders
   Markdown and surfaces link affordances.
5. **Context engine**: Active context = a query + optional geo/time signal +
   embedding-ranked results.
6. **Claude API integration**: Retrieval-augmented generation over the local
   graph; results returned as draft nodes.
