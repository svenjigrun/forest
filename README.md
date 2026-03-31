# Forest: A Pageless, Context-Driven Information Space

Forest (`#375700`) takes its name from the colour inverse of Purple Numbers'
`#C8A8FF` — the muted violet used by Eugene Kim (eekim) in his Purple Numbers
project at https://eekim.com/software/purple/. The complementary colour is a
dark olive-forest green: R 255−200, G 255−168, B 255−255.

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

## Addressing and Federation

### Node URLs as canonical identifiers

A node's stable, shareable address is a URL. Not a UUID with a URL wrapper
— the URL *is* the ID. This has several consequences:

- A node created on a personal instance resolves at
  `https://alice.example/nodes/01HXZJ...` and that address is permanently
  valid as long as Alice's instance exists (or forwards).
- Nodes can be linked across instances without any central registry. A node
  on Bob's instance can cite a node on Alice's instance using her URL
  directly.
- Merging two nodes means one URL becomes a redirect to the other. All
  inbound links from other instances eventually resolve to the canonical
  address. This is the same mechanism the web uses for moved resources;
  it is well-understood and does not require coordination.
- Local-only nodes (not yet published) carry a provisional local ID that
  is promoted to a URL when the user chooses to make them addressable.

### ActivityPub as the federation layer

ActivityPub (the W3C standard underlying Mastodon, Lemmy, and others) is
a strong candidate for the federation protocol, for specific reasons that
go beyond "it already exists":

**Actors and objects map cleanly.** In ActivityPub, an Actor (a user or
instance) publishes Objects. A node is an Object. An enrichment to a node
is an `Update` activity. A link between two nodes is a `Link` or a custom
activity type. The vocabulary is not a perfect fit, but it is close enough
that the mismatch is in extension, not in contradiction.

**JSON-LD is the wire format.** ActivityPub uses JSON-LD, which means every
node's content, links, and provenance can be expressed as a machine-readable
semantic graph out of the box. This aligns with the Semantic Web lineage in
the prior work table — but with ActivityPub you get the infrastructure
(delivery, federation, actor discovery) for free.

**Inboxes enable push-based context updates.** When a node Alice cites is
updated by Bob, Bob's instance can deliver an `Update` activity to Alice's
inbox. Alice's system can then decide — based on her active contexts and the
ageing state of her local copy — whether to surface the update, enrich her
local node, or ignore it. This is a richer model than polling or webhooks.

**The web as a superset.** If a node's URL resolves to a human-readable
HTML page (with the JSON-LD representation in a `<script type="application/
ld+json">` block or via content negotiation), then the node is also a
first-class web resource. Any browser can read it. Any search engine can
index it. Purple becomes a layer on top of the web rather than a silo
alongside it. A user's published nodes are, in effect, their presence on
the web — without pages.

**Where ActivityPub needs extending.** ActivityPub's object model was
designed for social content (posts, likes, follows), not for a typed
knowledge graph. Extensions needed:

- typed link relationships beyond `inReplyTo` (cites, contradicts,
  transcluded-from, corroborates)
- node versioning with diffable history
- context nodes as a first-class object type
- schema nodes and schema-version links as a first-class object type
- ageing/recency signals that instances can exchange without leaking
  interaction metadata

These are vocabulary extensions, not protocol changes. The Linked Data
ecosystem (schema.org, the Activity Streams vocabulary) provides a
foundation; custom `@context` entries handle the rest.

### Programmatic addressability

Beyond locating a specific node by URL, there is a second kind of
addressability: **querying a set of nodes by expression**. This is where
the graph becomes programmable.

**SQL over the local graph**  
For a single-user local instance backed by SQLite, SQL is already the query
language for the store. Exposing a read-only SQL interface (even a restricted
subset) lets power users and integrations express precise queries:

```sql
SELECT n.id, n.content
FROM nodes n
JOIN edges e ON e.source_id = n.id
WHERE e.type = 'cites'
  AND e.target_id = 'https://alice.example/nodes/01HXZJ...'
  AND n.recency_score > 0.3
ORDER BY n.recency_score DESC;
```

This is not a user-facing feature. It is a developer/integration surface —
the foundation for building context plugins, export tools, and bridges to
other systems.

**JSONata for node content**  
Where SQL addresses the graph structure, JSONata addresses the content
*within* nodes. A node may contain structured data (a list, a table, a
set of key-value pairs embedded in Markdown or as a JSON block). JSONata
expressions can extract, transform, and project that content:

```
nodes[provenance.source = "user"]
  .{ "id": id, "first_sentence": $substringBefore(content, ".") }
```

This is useful for transclusion — instead of transcluding an entire node,
a `{{node-id | $.items[status="open"]}}` expression transcludes only the
matching fragment. The transclusion is live: as the source node is enriched,
the expression re-evaluates.

**URL as a query carrier**  
These two approaches compose into a URL scheme for programmatic
addressability:

```
https://alice.example/nodes?cites=https://bob.example/nodes/XYZ&since=2024
https://alice.example/nodes/01HXZJ.../fragment?q=$.items[status%3D"open"]
```

The first form returns a set of nodes (a context, in effect). The second
returns a fragment of a single node's content. Both are cacheable, linkable,
and composable with the federation layer — another instance can subscribe
to a query URL and receive push updates when the result set changes.

This is the path by which Forest could become a superset of general web
use rather than a parallel system: nodes replace pages, queries replace
navigation, and federation replaces centralised hosting — while remaining
fully compatible with how links and URLs already work.

---

## Structured Data

### Nodes are content-type agnostic

A node's content is not required to be prose. It can be:

- a Markdown paragraph (the default)
- a JSON or JSON-LD object
- a CSV or Arrow/Parquet table
- a full dataset with schema and metadata
- a reference to an external dataset (a pointer, not a copy)

The content type is declared on the node. Rendering, querying, and
diffing are handled by the view layer, which selects an appropriate
representation based on content type and active context. A user querying
a set of nodes does not need to know — or care — whether the data lives
in one node, many nodes, or an external source. The view is the
abstraction; the storage topology is an implementation detail.

### Storage topologies

Three topologies, all first-class:

**1. Whole-dataset node**  
The entire dataset — schema, metadata, and records — lives in a single
node. Suitable for small or self-contained datasets. The node is
addressable as a unit; individual records are addressable via fragment
queries (JSONata or SQL-over-node). A spreadsheet, a contacts list, a
configuration table.

**2. Record-per-node with a schema node**  
Each record is its own node, linked to a shared schema node. The schema
node defines field names, types, and constraints. Each record node links
to a specific version of the schema. Suitable when individual records are
themselves concepts worth addressing, linking, and enriching independently
— a collection of books, a set of meeting notes with a common structure,
a project tracker where each task is a node.

**3. External dataset reference**  
The node contains a pointer to an external data source — a URL, a
DuckDB/DuckLake catalogue entry, a database connection — plus the schema
and query needed to materialise a view. The data is not stored in the
graph; the node is the address and the intent. The view layer resolves
the pointer at query time, caches aggressively, and presents the result
identically to topologies 1 and 2. This is how Forest connects to live
data: a node that references a Postgres table, a Parquet file on S3, or
a public data API is indistinguishable to the user from a node that
contains the data locally.

These topologies compose: a context can mix prose nodes, record nodes,
and external-reference nodes in the same view. The query layer
normalises them.

### Schema nodes

A schema node is a first-class node. It holds:

- field definitions (name, type, constraints, description)
- a version identifier
- a link to its predecessor schema version (if any)
- uplift instructions: how to transform a record from the previous version
  to this one

Record nodes link to a specific schema version, not to the schema node
itself. This means the schema can evolve without invalidating existing
records. When a record node is opened for editing, the system checks
whether its linked schema version is current:

- if current: edit normally
- if behind: present the uplift as a diff for the user to review and
  confirm before editing; the node is then re-linked to the current
  schema version
- if the uplift is ambiguous (a field was split, a type changed
  non-trivially): surface the specific conflict with UX to resolve it,
  rather than silently applying a best-guess transformation

Schema nodes age and link the same as any other node. A schema from a
project abandoned five years ago is not deleted — it is aged, but still
linked from the record nodes that use it, and still resolvable. Reviving
those records means either uplifting them to a current schema or working
with the old one explicitly.

### Querying across topologies

The query surface is uniform regardless of topology. Three layers:

**Fragment queries within a node** — JSONata for JSON/document content,
SQL for tabular content within a single node. These operate on the node's
content directly and are used in live transclusion:

```
{{https://alice.example/nodes/contacts | $.people[active=true].name}}
```

**Graph queries across nodes** — SQL or a graph query language (Cypher,
SPARQL) over the node store. These traverse the link structure and can
join prose nodes, record nodes, and schema nodes:

```sql
SELECT r.id, r.content->>'name' AS name
FROM nodes r
JOIN edges e ON e.source_id = r.id AND e.type = 'schema'
JOIN nodes s ON s.id = e.target_id
WHERE s.content->>'entity' = 'contact'
  AND r.recency_score > 0.2;
```

**External queries** — for external-reference nodes, the query is
delegated to the external engine (DuckDB, a REST API, a SQL database).
The result is returned as a virtual node set, rendered identically to
local record nodes, and can be saved as a context or transcluded.

The three layers are composable: a context can be defined as a join
across a local graph query and an external query, with the results
merged and ranked by the relevance engine.

### Structured data and the AI layer

The enrichment-first principle applies to structured data, but the
mechanics differ from prose:

- For record-per-node datasets, AI enrichment proposes changes to
  individual record nodes — new field values, corrected entries, added
  links to related nodes — presented as cell-level diffs rather than
  text diffs.
- For whole-dataset nodes, AI can propose new records, schema amendments,
  or derived columns — each as a reviewable change.
- For external-reference nodes, AI cannot enrich the source (it is
  external), but can propose a local annotation node linked to the
  external reference, capturing observations about the dataset without
  modifying it.

Schema changes proposed by AI are treated with extra caution: they
always require explicit user confirmation, and the uplift instructions
are shown in full before being applied to any record node.

---



### Node Schema (simplified)

```json
{
  "id": "https://alice.example/nodes/01HXZJ...",
  "content": "The paragraph is the atom of meaning.",
  "content_type": "text/markdown",
  "created_at": "2026-03-31T09:14:00Z",
  "versions": ["https://alice.example/nodes/01HXZJ.../v/0"],
  "provenance": {
    "source": "user",
    "imported_url": null
  },
  "links": [
    { "type": "cites", "target_id": "https://alice.example/nodes/01HWAB..." },
    { "type": "transcluded_from", "target_id": "https://bob.example/nodes/01HWAB...", "target_url": "https://example.com/post#pNNN" }
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

## Open Questions / Design Tensions

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
| Semantic Web / JSON-LD | Machine-readable typed links, shared vocabularies | Keep the semantics, remove the bureaucracy |
| ActivityPub (W3C) | Federated actor/object model, inbox delivery, JSON-LD wire format | Extend vocabulary for typed knowledge links, versioning, context nodes |
| Obsidian | Local-first, markdown, graph view | Make the context dynamic; make the AI integral, not a plugin |
| JSONata | Declarative expression language for JSON traversal and projection | Use as the fragment query language for live transclusion |
| SQL | Relational query over structured data | Expose as a read-only developer surface over the local node store; delegate to external engines for external-reference nodes |
| DuckDB / DuckLake | In-process OLAP over local and remote Parquet/Arrow data | Model for external-reference nodes: the node is a pointer + query; DuckDB materialises the view at query time |

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
