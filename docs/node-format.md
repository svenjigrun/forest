# Node File Format

Every Forest node is stored as a Markdown file with a YAML frontmatter block.
The file is the source of truth. The DuckDB index is derived from it.

## File location

| Content type | Directory |
|---|---|
| `text/markdown` | `nodes/` |
| `application/json` | `nodes/` |
| `application/x-python`, `application/sql`, `application/x-prompt+llm` | `nodes/` |
| `forest/schema` | `schemas/` |
| `forest/context` | `contexts/` |
| `forest/external-ref` | `nodes/` |

File name: `<UUIDv7>.md`

## Frontmatter fields

| Field | Required | Description |
|---|---|---|
| `id` | Yes | UUIDv7, stable forever |
| `content_type` | Yes | One of the content type strings below |
| `created_at` | Yes | RFC3339 UTC timestamp |
| `provenance.source` | Yes | `user`, `external`, or `ai` |
| `provenance.imported_url` | No | Source URL for external nodes |
| `links` | No | List of typed outbound links |
| `contexts` | No | List of context tag strings |
| `runtime` | Executable only | `python`, `sql`, `js`, `shell`, `prompt+llm` |
| `trust` | Executable only | `user`, `trusted`, `untrusted` |
| `inputs` | Executable only | List of input node IDs |
| `triggers` | Executable only | `on-input-change`, `on-demand`, `never` |

## Content types

| String | Meaning |
|---|---|
| `text/markdown` | Prose node (default) |
| `application/json` | Structured data node |
| `application/x-python` | Python executable node |
| `application/sql` | SQL executable node |
| `application/x-prompt+llm` | LLM prompt node |
| `forest/schema` | Schema definition node |
| `forest/context` | Context definition node |
| `forest/external-ref` | Pointer to an external dataset |

## Link types

| String | Meaning |
|---|---|
| `cites` | This node references another as a source |
| `responds-to` | This node is a response or reply |
| `contradicts` | This node disagrees with the target |
| `corroborates` | This node supports the target |
| `transcluded-from` | Content is transcluded from the target |
| `imported-from` | Content was ingested from the target URL |
| `schema` | This node conforms to the target schema node |

## Example: prose node

```markdown
---
id: 01234567-89ab-7def-8123-456789abcdef
content_type: text/markdown
created_at: 2026-03-31T09:00:00Z
provenance:
  source: user
links:
  - type: cites
    target: aaaaaaaa-bbbb-7ccc-8ddd-eeeeeeeeeeee
contexts:
  - project:forest
  - time:2026-Q1
---

The paragraph is the atom of meaning.
```

## Example: Python executable node

```markdown
---
id: 01234567-89ab-7def-8123-456789abcde0
content_type: application/x-python
created_at: 2026-04-01T10:00:00Z
provenance:
  source: user
runtime: python
trust: user
inputs:
  - aaaaaaaa-bbbb-7ccc-8ddd-eeeeeeeeeeee
triggers: on-input-change
---

print('hello from a Forest node')
```
