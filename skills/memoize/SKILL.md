---
name: memoize
description: Write memory to a remindb MCP server with Markdown that indexes well into the node tree. Covers shape rules for good indexing, search-first updates, cold-node summarization, source recompile, wiki-link authoring, manual relation edges, pinning nodes against temperature decay, explicit node removal with three deletion modes, and rolling back the graph to a prior snapshot with optional history pruning. Pair with `remind` for reads.
---

# Memoize — write to remindb so it indexes well

This skill owns the **write path** of remindb's MCP surface: `MemoryWrite`, `MemoryForget`, `MemorySummarize`, `MemoryCompile`, `MemoryRelate`, `MemoryPin`, `MemoryUnpin`, and `MemoryRollback`. It assumes you already know the read-side mental model (nodes, snapshots, IDs, ranking, notifications, budgets, relations) — that's `remind`'s job. If those terms aren't loaded, read `remind` first.

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

## Server-side redaction — secrets scrubbed before storage

`MemoryWrite`, `MemorySummarize`, and `MemoryCompile` all run inbound content through a redactor before it reaches the store. Any substring matching a known secret pattern is replaced in place with a visible marker of the form `«redacted:<kind>»`. The tool still succeeds — redaction never returns an error — and the snapshot still lands; what changes is that the stored content no longer carries the raw secret.

Built-in kinds covered by the default redactor:

| Category | Kind | Matches |
|---|---|---|
| Cloud | `aws_access_key` | `AKIA…` (long-term IAM) and `ASIA…` (temporary STS) access key IDs |
| Cloud | `gcp_oauth_client_secret` | `GOCSPX-…` GCP OAuth 2.0 client secrets |
| Cloud | `google_api_key` | `AIza…` Firebase / Maps / Translate / Places keys |
| Source forge | `github_pat` | Classic `ghp_`, `ghs_`, `gho_`, `ghu_`, `ghr_` personal-access tokens |
| Source forge | `github_fine_grained_pat` | New `github_pat_…` fine-grained tokens (93 chars) |
| Source forge | `gitlab_pat` | `glpat-…` GitLab personal-access tokens |
| AI / ML | `anthropic_api_key` | `sk-ant-api…`, `sk-ant-sid…` Anthropic API keys |
| AI / ML | `huggingface_token` | `hf_…` HuggingFace access tokens |
| AI / ML | `openai_api_key` | `sk-…T3BlbkFJ…` (legacy) and `sk-proj-…`, `sk-svcacct-…`, `sk-admin-…` OpenAI keys |
| Payments | `stripe_secret_key` | `sk_live_…`, `sk_test_…`, `rk_live_…`, `rk_test_…` |
| Payments | `stripe_webhook_secret` | `whsec_…` webhook signing secrets |
| Communication | `discord_bot_token` | `[MNO]…\.…\.…` bot tokens (3 dot-separated segments) |
| Communication | `discord_webhook` | `https://discord.com/api/webhooks/…` webhook URLs |
| Communication | `slack_token` | `xoxa-`, `xoxb-`, `xoxc-`, `xoxd-`, `xoxe-`, `xoxo-`, `xoxp-`, `xoxr-`, `xoxs-` |
| Communication | `slack_webhook` | `https://hooks.slack.com/services/T…/B…/…` |
| Email | `mailgun_api_key` | `key-…` Mailgun API keys |
| Email | `sendgrid_api_key` | `SG.…` SendGrid API keys |
| Packages | `npm_token` | `npm_…` registry tokens |
| Packages | `pypi_token` | `pypi-AgEIcHlwaS5vcmc…` upload tokens |
| Project mgmt | `linear_api_key` | `lin_api_…` Linear API keys |
| Structural | `jwt` | `eyJ…\.eyJ…\.…` header.payload.signature triples |
| Structural | `db_connection_string` | `postgres://`, `mysql://`, `mongodb://`, `redis://`, `amqp://`, `mssql://` URLs with embedded credentials |
| Structural | `bearer_auth_header` | `Authorization: Bearer …` headers (case-insensitive) |
| Structural | `basic_auth_url` | `http(s)://user:pass@host` URLs with HTTP basic auth credentials |
| Structural | `private_key_block` | Full PEM-encoded `-----BEGIN … PRIVATE KEY-----` … `-----END …-----` blocks |
| Structural | `env_secret_assignment` | `*_TOKEN=`, `*_KEY=`, `*_SECRET=`, `*_PASSWORD=`, `*_API_KEY=` style assignments |

Two consequences worth pinning:

- **Markers are visible.** An agent that accidentally writes `AKIAIOSFODNN7EXAMPLE` and later fetches the node will see `«redacted:aws_access_key»` in the content. Treat that as the signal: "I tried to memoize something the server categorically refuses to store verbatim."
- **Redaction is idempotent.** `Scrub(Scrub(s)) == Scrub(s)`. The marker glyphs (`«»`) and lowercase kind names don't match any pattern, so re-scrubbing pre-redacted content is a no-op. Safe to call across compile + write paths without double-marking.

There is **no bypass.** If a secret legitimately belongs in the store (a redacted-by-design placeholder, a public example), write the placeholder instead of the secret. Don't try to defeat the marker — the rule is "the store never sees the raw bytes," not "the store sees them if you ask nicely."

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

## MemoryForget — explicit node removal

Use `MemoryForget` when a node is wrong, stale, or never belonged — when you want it gone without rebuilding the surrounding subtree via `MemoryCompile` and without polluting the diff history by overwriting with empty content. Each call creates exactly one snapshot, so the deletion is recoverable through `MemoryHistory` and visible to `MemoryDelta`.

Three mutually exclusive modes; pick the one that matches the structural shape you want left behind.

### Mode: strict (default) — refuse to delete a parent

```
remindb__MemoryForget(node_id="<leaf_id>")
remindb__MemoryForget(node_id="<leaf_id>", mode="strict")
```

Deletes the node iff it has no children. If the node has children, the call fails with `node <id> has N children; pass mode=cascade or mode=reparent` and nothing changes. Use this when you mean "remove this leaf" and want a loud failure if the tree shape has drifted since you decided. Strict is the right default — it forces you to think about descendants explicitly.

### Mode: cascade — also remove descendants

```
remindb__MemoryForget(node_id="<subtree_root>", mode="cascade")
```

Deletes the target and every descendant. The snapshot records one `rem` entry per removed node, in subtree order — the server walks the subtree explicitly so each descendant is visible to `MemoryDelta`, then issues the deletes through the same `parent_id ON DELETE CASCADE` FK that any single-row delete would. The FTS5 and relations trigger sync runs per row. Use when an entire branch is obsolete — a whole compiled file that no longer exists, a deprecated feature's documentation, an experiment whose nodes were never useful.

### Mode: reparent — promote children to the target's parent

```
remindb__MemoryForget(node_id="<intermediate_id>", mode="reparent")
```

Deletes the target and re-parents its direct children to the target's parent. Use when the target node is wrong (bad heading, accidental level, misnamed section) but its children are still valid in the surrounding context. The snapshot records one `rem` entry for the target plus one `mod` entry per direct child whose `parent_id` moved. The mod entries carry `OldHash == NewHash` (content unchanged — only the structural link moved); `remind` describes how to interpret this on the read side.

**Root case.** If the target has no parent (`parent_id IS NULL`), the children become roots themselves. Forbidding this would just force a two-step workaround — promotion to root is part of reparent's contract.

### Order of operations matters (internal)

For reparent, the server updates children's `parent_id` *before* deleting the target. The `ON DELETE CASCADE` FK would otherwise eat the children the moment the target row dies. You don't author this ordering — it's a server invariant — but knowing it explains why reparent's snapshot lists mods *before* the rem in the diff trail.

### Pinning does not block deletion

`MemoryPin` protects a node from temperature decay and cold-set selection, **not** from explicit deletion. `MemoryForget` ignores `pinned`. If you don't want a node deleted by an automated workflow, the safeguard is not pinning — it's not calling `MemoryForget` on it.

### Relations layer self-heals

Deleting a node:

- **Drops the node's outgoing edges** via the relations FK cascade (the row pointing *from* the deleted node disappears).
- **Demotes incoming edges to pending**, keyed by the deleted node's label and source file. If a node with the same label later reappears (e.g., via `MemoryCompile` or `MemoryWrite`), the next compile re-resolves them automatically.

No action needed on the relations layer; this is server-side trigger behavior.

## MemoryRollback — revert to a snapshot

Use `MemoryRollback` when one or more recent writes left the graph in a bad state and you'd rather restore a known-good point than patch each affected node by hand. The verb takes a target `snapshot_id` (use `MemoryHistory` or `MemoryStats` to identify it) and emits **one new snapshot** capturing the rolled-back state — the rollback itself is an event in the diff trail, visible to `MemoryDelta` like any other write.

Two idempotent fast paths skip the snapshot emit entirely: rolling back to the current HEAD returns `"already at snapshot <id>; nothing to do"`, and rolling back to a snapshot whose computed state already matches HEAD (e.g., after a no-op `MemoryWrite` of the same payload) returns `"no rollback applied; computed state matches HEAD for snapshot <id>"`. Neither emits a diff row, so an accidental rollback-to-self doesn't pollute history.

### The two modes

```
remindb__MemoryRollback(snapshot_id=42)                       # default: drop_after=false
remindb__MemoryRollback(snapshot_id=42, drop_after=true)
```

- **`drop_after=false` (default)** — preserves the discarded snapshots as branched history. The main spine becomes `target → ... → previous_HEAD → rollback_snap`; the intermediate snapshots stay reachable via `MemoryHistory` for each affected node so you can still audit what was rolled back. Use this whenever the audit trail matters or you might want to cherry-pick a single change back.
- **`drop_after=true`** — hard-deletes every snapshot row (and its `diffs` rows) between target and the rollback snapshot. The main spine collapses to `target → rollback_snap` and `MemoryHistory` no longer surfaces the discarded events. Use this when the intervening writes are noise — agent thrash, half-written notes, leaked secrets — and you don't want them recoverable. **Irreversible:** once `drop_after=true` commits, the deleted snapshots can't be brought back; another `MemoryRollback` won't find them.

### What gets restored and what doesn't

Restored to the target snapshot's values:

- **Node content** — full text and content hash.
- **Node metadata** — `parent_id`, `source_file`, `node_type`, `depth`, `label`, `format`, `token_count`. Reparented nodes (via `MemoryForget mode=reparent`) and renamed labels return to their pre-mutation shape.
- **Tree shape** — nodes deleted between target and HEAD reappear; nodes created since target are removed.
- **FTS5 index** — automatically by triggers on the row updates the rollback emits.

**Not** restored:

- **Temperature, access count, last-accessed timestamp.** These reflect access history, not content history, and rolling them back would lose information about how the agent has been navigating the graph. They keep their current values.
- **Pinned state.** Same reason: pin reflects a workflow decision, not content state. Nodes that get recreated by the rollback (deleted between target and HEAD) start unpinned regardless of whether they were pinned at the target snapshot.
- **Relations and pending relations.** The relations layer is a sideband, distinct from the snapshot-tracked node graph. A `MemoryRelate` edge created between target and HEAD stays after rollback (the rollback didn't touch it); a `MemoryRelate` edge that existed at target but was deleted along with its endpoint node will re-resolve through the normal pending-relations flow on the next compile if the endpoint is restored.

### Pre-migration limitation

Rollback target older than the `0005_diff_metadata` migration cut-over carries NULL old-metadata for any `OpRem` events in the range. Nodes that were deleted in that range can't be fully reconstructed — the rollback **skips** them and surfaces a per-node warning in the result text:

```
rolled back to snapshot 42 (new snapshot 71, 8 nodes affected)
warning: 2 node(s) could not be fully restored:
  - abc12345678: pre-migration OpRem; node metadata unavailable
  - def98765432: pre-migration OpRem; node metadata unavailable
```

Treat these warnings as actionable — the rolled-back graph is missing those nodes. If they were important, recreate them manually via `MemoryWrite` and reconstruct any relations you remember. The limitation only affects targets *older* than the migration; new diffs always capture the full metadata.

### Atomicity

The handler runs the entire flow — node mutations, snapshot insert, diff inserts, cursor advance, optional prune — in a single `Store.Tx`. A crash or `ctx` cancellation mid-call rolls everything back; you never see a half-rolled-back graph with a stale HEAD cursor. This is the reason `MemoryRollback` bypasses `emitter.Emit` (the standard write tools use Emit, but two transactions for emit + prune would leave a recoverable-but-real failure window).

### When to choose rollback over MemoryForget

- **Multiple bad writes** to clean up at once → rollback (one verb, one snapshot, atomic).
- **Single bad node**, the rest of the recent diff trail is fine → `MemoryForget` (smaller blast radius, leaves history intact).
- **Polluted compile** that re-ingested the wrong source root → rollback to before the compile, then `MemoryCompile` the correct path.
- **Privacy-sensitive content** accidentally written → rollback with `drop_after=true` to hard-delete the snapshots that hold the bad content. The `diffs.old_content` / `new_content` rows are the only place the content survives; pruning removes them.

## Authoring wiki-links — graph relations in your payload

A `[[Label]]` marker in a Markdown or HTML payload becomes a **parsed edge** in the relations graph (see `remind`'s "Relations — the graph layer" section for the mental model). The parser strips resolver parameters from the stored content and captures them as edge metadata; what readers see is the clean normalized marker.

### Markdown syntax

```
[[Architecture]]                                        # bare label
[[Architecture; w=2.76]]                                # with weight
[[Architecture; w=2.76; source=docs/ARCH.md]]           # source-qualified
[[Architecture; w=2.76; id=3kGXxidmWBp]]                # explicit target ID
[[3kGXxidmWBp]]                                         # bare ID (11 base62 chars)
```

After parsing, `[[Architecture; w=2.76; source=docs/ARCH.md]]` is stored in `nodes.content` as `[[Architecture]]`. The resolver params (`w`, `source`, `id`) live in the source file on disk — re-extracted on every `MemoryCompile`.

### HTML alternative

```
<knowledge>Architecture</knowledge>
<knowledge weight="2.76">Architecture</knowledge>
<knowledge weight="2.76" source="docs/ARCH.md">Architecture</knowledge>
<knowledge weight="2.76" id="3kGXxidmWBp">Architecture</knowledge>
```

Both syntaxes produce the same normalized `[[Label]]` in the stored content. Agents should learn one pattern across formats.

### Resolution priority

The resolver tries each marker in this exact order:

1. `id=<hint>` present → look up by ID. **No fallback if the ID doesn't exist** — the edge goes pending.
2. `source=<file>` + label → label match restricted to that file. **No fallback if the (source, label) pair misses** — the edge goes pending. `source=docs/x.md` matches both the exact stored path and absolute paths ending in `/docs/x.md`.
3. Label only → label match across all heading nodes, case-insensitive, whitespace-trimmed. First match wins by `(source_file ASC, depth ASC, id ASC)`.

The **hard-constraint** rule for IDs and source qualifiers is intentional: a typo in `source=docs/x.md` produces a discoverable pending row instead of silently linking to whatever heading shares the label elsewhere. Fix the qualifier and the next compile self-heals the edge.

### Weight semantics

`weight` is `REAL`, default `1.0`. **Higher weight = more important connection.**

- `1.0` — regular link (default).
- `2.0`–`5.0` — emphasized; a connection you want to surface first.
- `< 1.0` — weak / tentative link.

In `MemoryRelated`, results rank by **sum-along-path weight** — `weight_min` filters out edges below the threshold. Authoring `[[X; w=3]]` is a deliberate signal that the link matters; don't waste it on incidental references.

### Code blocks and code spans bypass extraction

`[[X]]` inside fenced code blocks or `<code>` elements is example text, not a link. The parser leaves the marker in the content verbatim and emits no edge. This is intentional — example syntax in documentation must not generate relations.

## MemoryRelate — manual edges between existing nodes

Use `MemoryRelate` when the user wants to connect two nodes that don't have a `[[Label]]` marker in their source — typically after a conversation where two previously-disconnected memories turn out to be related.

```
remindb__MemoryRelate(source_id="<node_id>", target_label="Architecture")
remindb__MemoryRelate(source_id="<node_id>", target_label="Architecture", target_source="docs/ARCH.md", weight=2.5)
remindb__MemoryRelate(source_id="<node_id>", target_id="3kGXxidmWBp", weight=2.0)
```

Resolution uses the same priority as parsed wiki-links (id → source+label → label only, both narrowing modifiers are hard constraints). Hit → edge in resolved relations. Miss → pending edge, retried on every subsequent compile.

### Snapshot-free by design

**`MemoryRelate` does NOT create a snapshot.** Edges are a sideband — they don't appear in `MemoryDelta` or `MemoryHistory`, and the cursor doesn't move. The only way to inspect the current graph is to call `MemoryRelated`.

This is different from `MemoryWrite` / `MemorySummarize` / `MemoryCompile`, which all snapshot. The rationale: edges churn faster than content does, and binding them to the cursor chain would either pollute the diff trail or force batching that defeats the agent's flexibility.

### Prefer `target_label` + `target_source` over `target_id`

Structural node IDs are content-addressed on `(source_file, parent_id, sibling_index)`. Reordering siblings in a file rotates the IDs of every later sibling. A `target_id=<id>` manual edge doesn't self-heal — if the target's ID changes, the edge silently points at the wrong node (or, if no node has the old ID, becomes orphaned).

**Recommended pattern:** `target_label` + `target_source` for manual edges. The next compile re-resolves the label against the current state of the headings, so sibling reorders don't break the edge. Use `target_id` only for short-lived references.

### Origin coexistence

`parsed` and `manual` are independent origins. If both a `[[Architecture]]` marker in source *and* a `MemoryRelate(..., target_label="Architecture")` resolve to the same target, both rows exist — `UNIQUE(source_node_id, target_node_id, origin)` permits the pair. `MemoryRelated` deduplicates by target node in its output, so the agent sees one row per related node regardless.

## MemoryPin / MemoryUnpin — protect a node from decay

Some content shouldn't cool down: a project invariant, a stable fact, a root label the user has flagged as important. The temperature ticker would eventually drive any node below `ColdThreshold` and push it into the cold-node notification stream, prompting a summarization that compacts wording you wanted to keep verbatim. Pinning gates that policy.

```
remindb__MemoryPin(node_id="<node_id>")
remindb__MemoryPin(node_id="<node_id>", temperature=0.9)   # pin AND set to 0.9 in one call
remindb__MemoryUnpin(node_id="<node_id>")
```

What pinning does:

- The node is skipped by the temperature decay sweep — its `temperature` value freezes at whatever it was when pinned, or at the optional `temperature` override if you pass one.
- The node is excluded from the cold-set query, so it never appears in a `level: "warning"` / `logger: "remindb.temperature"` notification.
- Boosts still apply normally. Reading the node via `MemoryFetch` / `MemorySearch` warms it as usual; pin gates *cooling*, not access.
- Updates via `MemoryWrite` still work and create snapshots as usual. The `pinned` flag is independent of content.

The optional `temperature` argument (in `[0, 1]`) is for the common case where the node has drifted to some transient value you don't want frozen as-is — pin at `0.9` to mark a high-priority invariant, or pin at `0.5` to anchor a node at neutral warmth. Pass it whenever the *current* temperature doesn't match the importance you want to lock in. `MemoryUnpin` deliberately has no temperature option — releasing a node should return it to the lifecycle, not double as a temperature reset.

### Snapshot-free by design

**`MemoryPin` and `MemoryUnpin` do NOT create a snapshot.** Pin state is metadata, not content — it doesn't appear in `MemoryDelta`, `MemoryHistory`, or the cursor chain. Binding it to the snapshot ledger would conflate "I changed what this node says" with "I changed how the temperature engine treats this node," and the two have different lifecycles. The same carve-out applies to `MemoryRelate` for the same reason.

### When to pin

- A heading that names a stable architectural concept (`# Authentication model`) you want the agent to find on every search regardless of recency.
- A frontmatter or preamble node carrying invariants (allowed tech stack, ownership map, security boundaries).
- A summary node the user has explicitly marked as canonical — pin it so the next decay cycle doesn't re-summarize it into oblivion.

### When NOT to pin

- A working note that will naturally decay once the task is done. Let it cool; summarize when notified.
- An entire compiled file. Pinning every node in `docs/` fights the whole temperature model and the cold-node notification stream loses its signal.
- A node you can't decide about. The default is unpinned; pinning is an explicit "this matters" signal, not a "just in case."

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

If a `.remindb.ignore` file lives at the source root, `MemoryCompile` (and the background rescan) honors it — gitignore-style subset (literal names, `*` / `?` / `[abc]` wildcards, trailing `/` for dir-only, leading `/` for root-anchor, `**` for any-segment-count, `!` negation with last-match-wins, `\` to escape a leading `!` or `#`, `#` comments). Patterns subtract from the supported-extension allow-list; they cannot re-include hardcoded skip directories (dependency caches and build outputs like `node_modules`, `vendor`, `target`, `dist`, `venv`) or dotfiles. Operators set this once; the agent doesn't author the file.

## Anti-patterns — do not

- Don't write a flat 500-word paragraph as one payload. Split with headings.
- Don't use `---` (horizontal rule) to separate sections. The parser drops it. Use a heading.
- Don't merge unrelated facts into one paragraph. List items or sub-headings.
- Don't write a payload with a blank or generic first line — that's your label.
- Don't append by writing the same payload + extra text to a fresh node — different content hash, new ID, fragmented tree. Update the existing anchor instead.
- Don't overwrite a node with empty content as a deletion workaround. That fights the temperature model (an empty node sits at default warmth and pollutes search), it leaves a phantom in `MemoryTree`, and the diff trail loses the "this was deleted" signal. Use `MemoryForget` instead — it produces a clean `rem` entry the next `MemoryDelta` walker can act on.
- Don't reach for `mode=cascade` when `mode=reparent` is what you mean. Cascade discards the whole subtree; reparent keeps the children in the tree under the target's parent. If the target is the *only* thing wrong (bad heading, accidental level), reparent — cascade is the nuclear option.
- Don't pin a node hoping it will survive `MemoryForget`. Pin only gates temperature decay and the cold-set query; explicit deletion ignores it. If you want a safeguard against deletion, the safeguard is workflow discipline, not the `pinned` flag.
- Don't skip the search-first step. A near-duplicate sibling is worse than an in-place update.
- Don't write per-keystroke. Batch into one coherent note per call.
- Don't try to reparent a node by writing a payload with a different heading hierarchy — `MemoryWrite` with an anchor preserves `parent_id`, the heading shape inside the payload only affects content, not the tree position.
- Don't ignore a `level: "warning"` / `logger: "remindb.temperature"` notification. Walk the `nodes` array and `MemorySummarize` each `id`. The server won't re-push the same node until it warms and re-cools.
- Don't summarize *toward* a flat paragraph. Apply the same shape rules — a summary that loses structure is a summary that won't index.
- Don't `MemoryCompile` the entire source tree when one file changed. Narrow the path.
- Don't author `[[Label]]` inside a fenced code block or `<code>` element expecting it to become a relation — the parser bypasses extraction in code contexts by design.
- Don't use `target_id` for long-lived `MemoryRelate` edges. Sibling reorders rotate IDs; `target_label` + `target_source` self-heals on the next compile.
- Don't expect `MemoryRelate` to show up in `MemoryDelta` or move the cursor. Relations are a sideband — call `MemoryRelated` to inspect the current graph.
- Don't rely on label-only matching when the target heading shares a name with another file's heading. The first-match rule is deterministic but the "first" is whichever file sorts earliest by `(source_file, depth, id)` — pin `source=` if you mean a specific file.
- Don't pin everything. A universally pinned tree has no cold set; the temperature notifications stop being a signal. Pin sparingly — only nodes the user has flagged as invariants.
- Don't expect `MemoryPin` / `MemoryUnpin` to surface in `MemoryDelta`, `MemoryHistory`, or move the cursor. Pin state is metadata, not content — same sideband treatment as `MemoryRelate`.
- Don't use pinning as a workaround for premature summarization. If a node keeps cooling because it isn't being accessed, that's the cold-node notifier doing its job — summarize it. Pin only when the *content itself* should remain verbatim, not when you'd rather not deal with the summary yet.
- Don't `MemoryRollback` to clean up a single bad node — use `MemoryForget`. Rollback's blast radius is every snapshot since the target; reaching for it to undo one write throws away unrelated diff history.
- Don't `MemoryRollback` with `drop_after=true` to "tidy up" the snapshot list. The pruning is irreversible; if the goal is cosmetic, leave `drop_after=false` and accept the orphan branches — `MemoryHistory` and `MemoryDelta` already filter by node, so the visible noise per query is small.
- Don't expect `MemoryRollback` to restore temperature, pinned state, or relations. It restores the snapshot-tracked node graph; sideband state stays current. If you need the rolled-back nodes pinned again, call `MemoryPin` after the rollback.
- Don't ignore the "could not be fully restored" warning. Each skipped node means the rolled-back graph is missing content that existed at the target. Either accept the gap deliberately or recreate the nodes via `MemoryWrite`.
- Don't rely on server-side redaction as the *primary* defense against pasting secrets. The redactor covers a fixed pattern set (AWS / GCP / Google keys, classic + fine-grained GitHub PATs, GitLab PATs, OpenAI / Anthropic / HuggingFace keys, Stripe keys + webhook secrets, Slack / Discord tokens + webhooks, SendGrid / Mailgun keys, npm / PyPI tokens, Linear keys, JWTs, DB connection strings with embedded creds, `Authorization: Bearer` headers, PEM blocks, `*_KEY=` / `*_SECRET=` assignments) — it will catch the obvious shapes, but a private API token with a custom format will pass through. If a payload might contain something you wouldn't paste into a public gist, treat the redactor as a backstop, not as authorization to paste it.
- Don't try to encode a secret to slip past the redactor (base64, URL-encoded, split across whitespace, etc.). Even if it works, the resulting node carries the secret in a less searchable form — strictly worse than not memoizing it at all. If you need to record that a secret exists, write its *location* (the 1Password entry, the vault path) instead of the value.
