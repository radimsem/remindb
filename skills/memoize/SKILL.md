---
description: Write memory to a remindb MCP server with Markdown that indexes well into the node tree. Covers shape rules for good indexing, search-first updates, cold-node summarization, and source recompile. Pair with `remind` for reads.
---

# Memoize — write to remindb so it indexes well

This skill owns the **write path** of remindb's MCP surface: `MemoryWrite`, `MemorySummarize`, `MemoryCompile`. It assumes you already know the read-side mental model (nodes, snapshots, IDs, ranking, notifications, budgets) — that's `remind`'s job. If those terms aren't loaded, read `remind` first.

## Why payload shape matters

A `MemoryWrite` payload is parsed as Markdown before it's stored. The parser is mechanical: heading levels build the tree spine, and each non-heading block (list, code block, table, paragraph) becomes one leaf node attached to the nearest open heading. **The shape of your Markdown determines the granularity of every future search, fetch, and delta.**

Two extremes show the cost:

- **Flat blob.** A 500-word paragraph with no headings collapses to one fat text node. Searching surfaces the whole thing; you can't fetch just the relevant fact. Budgets blow.
- **Structured tree.** The same content under H1/H2/H3 with lists for fact-sets becomes a dozen addressable nodes. Each fact ranks independently in FTS5, fetches independently, and the temperature decay reflects which sub-fact actually got read.

Indexing quality is something you author at write-time. The parser won't fix a flat blob.

## How the parser maps Markdown to nodes

Grounded in `pkg/parser/markdown.go`. Block-level only — inline emphasis, links, and code spans flatten into the parent's content and are not addressable.

| Markdown block | Becomes | Notes |
|---|---|---|
| `# Heading`, `## Heading`, `### Heading`, … | `heading` node — owns a subtree | Each level pops back to the right ancestor (H2 after H3 closes the H3 subtree). Depth in the DB ≈ heading level. |
| Bullet / ordered list | `list` node — single leaf | Items are flattened to `- text` lines (nested items lose nesting). The whole list is one node, but each item ranks independently in FTS5. |
| Fenced code block | `code` node — single leaf | Language tag prepended as the first line. Empty code blocks are dropped. |
| Table | `table` node — single leaf | Rendered as tab-separated rows, header first. |
| Paragraph | `text` node — single leaf | Soft breaks → space, hard breaks → newline. |
| HTML block | `text` node | Trimmed; empty blocks dropped. |
| Frontmatter (`---\n…\n---` at start, YAML/TOML) | `preamble` node — one, before the body | Good for tags or per-doc metadata. |
| Horizontal rule (`---` mid-doc) | **dropped silently** | Don't use `---` to separate sections. Use a heading. |

Two consequences worth pinning:

- **Headings are the *only* tree-building block.** Lists and tables are leaves no matter how deeply nested visually.
- **A payload with no headings has no spine.** Every block attaches to the sentinel root at depth 1 — siblings of everything else at the top level.

## Shape rules

1. **First line of the payload is the label.** Auto-derived, ≤80 chars, displayed in `MemoryTree` and search results. Put the scannable title there. A blank or generic first line ("Notes:", "TODO") gives you a useless label.
2. **Use heading hierarchy to split a long note into addressable subtrees.** H1 = topic, H2 = aspect, H3 = fact. Below H4 rarely earns its keep.
3. **Use lists for fact-sets**, not paragraphs. `- key: value` per line keeps each fact independently rankable in FTS5 even though the list is one node.
4. **Use code blocks for snippets** you want to fetch verbatim. They become clean leaves with their language tag preserved.
5. **Use tables for matrices** — comparison rows, lookup tables. One leaf, but cells are searchable.
6. **Don't use horizontal rules to separate sections.** The parser drops them. Use a heading.
7. **Don't merge unrelated facts into one paragraph.** Either split into list items or promote each to its own H3 under a shared H2.

## Examples — bad vs. good

### Bad: flat blob

```
We use Postgres on AWS RDS, with read replicas in us-west-2 and a primary
in us-east-1. The connection string is in 1Password under "prod-db".
The schema is managed via migrations in db/migrate.
```

Parses to **one text node**. Searching `us-west-2` returns the whole paragraph. There's no `Region` anchor to fetch.

### Good: structured

```
# Postgres production setup

## Region
- Primary: us-east-1
- Replicas: us-west-2

## Credentials
1Password vault entry: `prod-db`.

## Schema
Migrations in `db/migrate/`.
```

Parses to: `heading(Postgres production setup)` → 3× `heading(Region|Credentials|Schema)` → `list` (region) + `text` (credentials) + `text` (schema). Each subtree is independently fetchable; each fact ranks on its own.

### Bad: separator-driven layout

```
Region: us-east-1 primary, us-west-2 replicas

---

Credentials in 1Password under prod-db

---

Schema migrations in db/migrate
```

Three text nodes, all sibling leaves at the root. Horizontal rules are dropped, so the visual grouping you intended doesn't exist in the tree.

### Good: list of fact-pairs

```
# Prod DB facts

- region.primary: us-east-1
- region.replicas: us-west-2
- credentials: 1Password "prod-db"
- schema: db/migrate/
```

One heading node, one list node beneath it. Each line is an FTS5-rankable fact and the list-as-leaf keeps the fetch cheap.

## MemoryWrite

### Create a new node

```
remindb__MemoryWrite(payload="<full content, properly structured>")
```

- `anchor` omitted or empty → creates a new node. Content-addressed ID is derived from the payload (xxhash64).
- Default `node_type` = `text` unless the payload starts with a heading.
- Default `source` = `mcp:write`. Default `depth` = 1 (top-level child of root).

### Update an existing node

```
remindb__MemoryWrite(anchor="<node_id>", payload="<full replacement>")
```

- Replaces content **in place**. `node_type`, `parent_id`, and `source` are preserved.
- The whole payload is the replacement — there's no append, no patch. If you want to extend, `MemoryFetch` first, edit the content, write it back.

### Search-first rule

Before creating a new node, search for an existing anchor on the same topic via `remind`'s `MemorySearch`. Updating beats creating because:

- The existing parent / type / source / children stay attached.
- The temperature history is preserved (a fresh node starts at the default warmth).
- The diff trail tells future sessions how this fact evolved.

A new sibling node next to an existing one on the same topic is almost always a mistake — the tree fragments and search returns near-duplicate hits.

### One logical note per call

Every write creates a snapshot. Don't write per-keystroke or per-token. If you have three independent facts to record, that's three calls — but each call should be one coherent note. Bundling unrelated facts into a single call gives you one node with mixed content; bundling related facts into a single well-structured payload (headings + lists) gives you a clean subtree.

## Summarize a cold node — the notification handoff

`remind` describes the notification: `level: "warning"`, `logger: "remindb.temperature"`, `data.message: "Cold nodes detected; consider summarizing via MemorySummarize"`, `data.nodes: [{id, label, file, temperature}, …]`.

When you see one, walk the `nodes` array and compact each one here:

```
remindb__MemoryFetch(anchor="<id>", budget=1500)                       # read what's there
remindb__MemorySummarize(node_id="<id>", summary="…")                  # replace in place; rebounds to 0.5
remindb__MemorySummarize(node_id="<id>", summary="…", temperature=0.7) # override when summary is high-value
```

`MemorySummarize`:

- Replaces content, recomputes `token_count`, rewrites the label to `"Summary: <first line>"` (truncated to 70 chars including prefix).
- **Preserves `node_type`, `parent_id`, and source file.**
- **Bumps temperature to `SummarizeRebound` (default 0.5)** so the summarized node falls out of the cold set immediately. Pass an optional `temperature` (in `[0, 1]`) to override per call when the summary deserves a stronger or weaker signal than the default.
- Creates a snapshot — prior wording recoverable via `MemoryHistory`.

The same shape rules apply to the `summary` payload. If the summary is more than a few sentences, give it headings or a list — a dense paragraph is what you're compacting *away from*. The summary should index *better* than the original, not just be shorter.

Notifications are deduplicated server-side per `ColdNotifyTTL` (default 1 hour); the same node won't be re-pushed within that window once it has been delivered. Summarizing immediately rebounds the node out of the cold set, so the next reminder only arrives if it decays back below `ColdThreshold`.

## Recompile when the source drifts

`MemoryCompile` lives here because it creates a snapshot — same write-side semantics as `MemoryWrite`.

```
remindb__MemoryCompile(path="<file or subdir>", message="<optional snapshot note>")
```

Use when files on disk changed outside remindb's rescan loop: external edit, disabled watcher, fresh `git pull`. Prefer narrow paths — compiling one file is milliseconds; compiling the entire source tree is slow and creates a large snapshot.

`path` may be absolute or relative; the server re-anchors it to its source root (`REMINDB_SOURCE`) before compiling, so the form you pass doesn't fork into duplicate nodes. Paths outside the source root, or when `REMINDB_SOURCE` is unset, pass through unchanged.

If a `.remindb.ignore` file lives at the source root, `MemoryCompile` (and the background rescan) honors it — gitignore-style minimal subset (literal names, `*` wildcards, trailing `/` for dir-only, `**` for any-segment-count, `#` comments). Patterns subtract from the supported-extension allow-list; they cannot re-include `node_modules` or dotfiles. Operators set this once; the agent doesn't author the file.

## Anti-patterns — do not

- Don't write a flat 500-word paragraph as one payload. Split with headings.
- Don't use `---` (horizontal rule) to separate sections. The parser drops it. Use a heading.
- Don't merge unrelated facts into one paragraph. List items or sub-headings.
- Don't write a payload with a blank or generic first line — that's your label.
- Don't append by writing the same payload + extra text to a fresh node — different content hash, new ID, fragmented tree. Update the existing anchor instead.
- Don't skip the search-first step. A near-duplicate sibling is worse than an in-place update.
- Don't write per-keystroke. Batch into one coherent note per call.
- Don't try to reparent a node by writing a payload with a different heading hierarchy — `MemoryWrite` with an anchor preserves `parent_id`, the heading shape inside the payload only affects content, not the tree position.
- Don't ignore a `level: "warning"` / `logger: "remindb.temperature"` notification. Walk the `nodes` array and `MemorySummarize` each `id`. The server won't re-push the same node until it warms and re-cools.
- Don't summarize *toward* a flat paragraph. Apply the same shape rules — a summary that loses structure is a summary that won't index.
- Don't `MemoryCompile` the entire source tree when one file changed. Narrow the path.
