# remindb skills

Two paired skills that teach an agent how to actually *use* remindb's MCP tool suite. Install them next to the [per-agent plugin](../plugins/) so the agent ships with both the MCP server and the know-how to drive it.

## What's here

| Skill | Purpose |
|---|---|
| [`remind/`](./remind/) | **Read path.** Orient with the tree; search, fetch (single or batched), resync via delta, diff two snapshots, walk a node's history, traverse the relations graph, and check DB health. Covers the node/snapshot/temperature/relations mental model and the FTS5 query syntax. |
| [`memoize/`](./memoize/) | **Write path.** Author Markdown that indexes well into the node tree: search-first updates, cold-node summarization, source recompile, wiki-link relations, manual edges, pinning against decay, three-mode node removal, and snapshot rollback. Notes how shape also drives automatic TOON/MathML compaction. |

The two are designed to load together — `memoize` references the mental model `remind` defines, so install both.

## Install

The skills are published from this repo and managed by [`vercel-labs/skills`](https://github.com/vercel-labs/skills). Every supported agent has a native skill loader, so one command installs both into the right place for your agent:

```bash
npx skills@latest add radimsem/remindb/skills -a claude-code
# -a codex | gemini-cli | opencode | openclaw | ...
```

Run it again with a different `-a <agent>` to add the skills for another agent. This is the *skills* half of setup; the [`plugins/<agent>/`](../plugins/) folders install the MCP server itself — you want both.

## Updating

Skills evolve with the MCP tool surface. Refresh every installed skill to the latest published version:

```bash
npx skills@latest update
```

No re-clone, no manual copy — `update` re-pulls in place across whichever agents you installed for.

## See also

- [Top-level README](../README.md) — what remindb is, the MCP server, benchmarks
- [`plugins/<agent>/README.md`](../plugins/) — per-agent MCP plugin install (the other half of setup)
- [`vercel-labs/skills`](https://github.com/vercel-labs/skills) — the installer these commands invoke
