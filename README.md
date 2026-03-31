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
- provenance metadata (written-by-user, imported-from-url)

**Nodes are not cheap.** The system does not create nodes freely. A new node
is only created when a concept genuinely has no existing home in the graph.
The default behaviour for every interaction — a query, an AI response, an
edit, an import — is to find the best existing node and *enrich* it, not
spawn a new one. Node count is a signal of the graph's conceptual breadth,
not its activity level.

Node lifecycle:

1. **Write** — user authors a new concept that has no existing node. A node
   is created.
2. **Enrich** — information arrives (user edit, AI synthesis, external
   import) that belongs to an existing concept. The existing node is updated
   in-place; the prior version is kept in history but does not appear as a
   separate node in any view.
3. **Split** — a node has grown to contain two genuinely distinct concepts.
   The user (or AI, with user approval) splits it into two nodes with a typed
   link between them. The original version is retained in history.
4. **Merge** — two nodes turn out to be about the same concept. They collapse
   into one; all edges from both are preserved on the merged node.

Edits record a version history internally, but that history is not a
proliferation of nodes — it is a log attached to a single node. The graph's
topology reflects *what the user knows*, not *how many times they interacted
with the system*.

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

Because contexts are nodes, they obey the same lifecycle rules as content
nodes. When the user opens a new context — by entering a query, changing
location, or starting a session — the system first searches for an existing
context node that closely matches. If one is found, it is reused and
optionally enriched (e.g. its query expression is sharpened, its display
config updated). A new context node is only created when no existing one is
a good enough match. This keeps the context graph from fragmenting in the
same way that content nodes might: "work at the blue desk" and "working from
home, Tuesday mornings" are probably the same context, and the system should
recognise that rather than accumulate both.

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

### 2. Graph-First Synthesis

When the user asks a question or enters a context, the AI layer's first
obligation is to the existing graph, not to generation. The flow is:

```
User query / active context
  → retrieve candidate nodes by embedding similarity
  → rank and cluster by conceptual overlap
  → present existing nodes as the primary answer

If existing nodes are incomplete:
  → AI drafts enrichments to specific nodes (not new nodes)
  → user reviews diffs against the existing content
  → user accepts, edits, or discards each enrichment
  → accepted enrichments are written as a new version of the existing node

Only if no existing node is close enough:
  → AI drafts a new node
  → user accepts / edits / discards
  → accepted node enters the graph
```

The key inversion from a standard RAG pattern: the output of AI synthesis is
a **proposed edit to an existing node**, not a new document. The graph
accumulates depth, not breadth. A node about "contextual relevance" that has
been enriched ten times over six months is far more valuable than ten
separate AI-generated summaries of the same idea.

When enriching, the AI annotates what it is changing and why — provenance is
recorded at the version level ("enriched from query: X, drawing on nodes
A, B, C") so the history of how a node evolved is always recoverable.

External sources follow the same rule: ingested content is matched against
existing nodes first. If a paragraph from an external URL says the same
thing as an existing node, it is linked as a `corroborates` edge, not
duplicated as a new node. Only genuinely new concepts from external sources
become new nodes.

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

**5. The AI deepens existing nodes; it does not multiply them.**  
AI output defaults to proposing enrichments to existing nodes, presented as
diffs. New nodes are only proposed when no existing node is a reasonable
home for the concept. The user reviews and approves all AI-proposed changes
to their own nodes. The graph's node count reflects the actual breadth of
the user's knowledge, not the volume of their queries.

**6. External information is a guest.**  
Nodes imported from external sources are clearly distinguished from
user-authored content. The user's graph is not polluted by ingestion; external
nodes participate in context and search but are visually and semantically
separable.

---

## Ageing

Not all nodes and contexts are equally relevant at all times. A node
recording what a user thought about a job they held twenty years ago is not
worthless — but it should not compete on equal footing with what they are
working on this week. Ageing is the mechanism by which the graph gracefully
recedes without losing anything.

### What ageing is not

Ageing is not deletion and not archiving in the traditional sense. The node
remains fully intact, searchable, and linkable. Ageing only affects two
things: **default relevance ranking** (aged nodes rank lower unless
explicitly surfaced) and **contextual visibility** (they do not appear in
ambient views unless the active context or query pulls them in).

### How nodes age

Each node carries a **recency signal** — a composite of:

- time since last user edit
- time since last user view (not just passive appearance in a stream, but
  deliberate interaction: expanding, linking, querying)
- the ageing of the contexts it belongs to (a node in only aged contexts
  ages faster)

Recency decays on a long, non-linear curve. The curve is slow at first —
something from last year is not noticeably aged. But it accelerates for
nodes that have seen no interaction in years. There is no cliff: a node from
twenty years ago is simply ranked very low by default, not invisible.

The decay rate is per-node, not global. A node the user touches once a year
resets. A node never revisited after creation ages steadily. This means
the graph self-organises: the user's active conceptual surface stays dense
and high-signal; the distant past recedes to the periphery.

### How contexts age

Context nodes age the same way, with one addition: a context's ageing also
reflects the ageing of the nodes it most commonly surfaces. A context built
around a job role, a project, or a place will naturally age as the nodes it
draws on age. The system does not need to know that the user left a job — it
infers it from the pattern of non-interaction.

### Recall

Aged nodes and contexts are always reachable. Three signals bring them
forward:

1. **Explicit query** — the user asks about something old. Aged nodes
   matching the query surface normally, with a subtle indicator that the
   content is long-untouched.
2. **Contextual resonance** — the user is writing something that strongly
   resembles an aged node. The system surfaces it as a suggested link or
   enrichment source, flagged as dormant: "you wrote about this in 2008."
3. **Context revival** — the user activates (or is auto-matched to) an aged
   context. All the nodes it draws on are temporarily de-aged for that
   session, allowing the user to re-enter a past conceptual state without
   those nodes polluting their current ambient view afterwards.

### The value of the aged periphery

The aged portion of the graph is not waste. It is the record of who the user
was, what they knew, and how they thought across time. The fact that it
recedes by default is not a loss — it is what makes the active surface
legible. And because ageing is a ranking signal rather than a deletion,
the user retains the ability to re-engage with any part of their history
at any time, on their own terms.

---



- **Identity and addressability across devices**: Purple Numbers assumed a
  single canonical document. In a distributed, sync'd graph, what does a
  stable, shareable node address look like? Content-hashing, UUIDs, or
  something cryptographic?

- **Context explosion** (partially addressed): Because contexts are nodes
  and follow the same enrich-before-create rule, the system resists
  proliferation structurally. But the matching problem is harder for
  contexts than for content nodes — two contexts may have identical query
  expressions but different emotional or temporal intent, and collapsing
  them would be wrong. The similarity threshold for context reuse needs to
  be more conservative than for content enrichment.

- **Latency of relevance ranking**: Embedding-based ranking over a large
  local graph must be fast enough to feel live. At what node count does this
  break? What is the degradation strategy?

- **The blank canvas problem**: A pageless system with no hierarchy gives new
  users no affordance for where to start. Onboarding must solve for this
  without recreating pages by another name.

- **Enrichment conflict**: When AI proposes to enrich a node, the diff must
  be legible — the user needs to see clearly what changes and why. For short
  nodes this is simple. For a node that has been enriched many times and
  grown dense, how do you present a proposed change without burying the user
  in context?

- **When to split vs. enrich**: The system needs a principled heuristic for
  when a node has become too broad and should be split. Too aggressive and
  you recreate the proliferation problem. Too conservative and nodes become
  monolithic blobs that lose the addressability that makes the graph useful.

- **Merge identity**: When two nodes merge, which ID survives? All inbound
  links to the absorbed node need to be updated or redirected. In a
  distributed system with other users holding links, this is a hard
  consistency problem.

- **The cold-start enrichment problem**: Enrichment only works well once the
  graph has enough nodes to match against. For a new user with a sparse
  graph, almost every interaction would propose new nodes — indistinguishable
  from the proliferation problem. The transition from sparse to dense needs a
  designed path.

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
