# remindb skills

Four skills that teach an agent how to actually *use* remindb's MCP tool suite. Install them next to the [per-agent plugin](../plugins/) so the agent ships with both the MCP server and the know-how to drive it.

## What's here

| Skill | Purpose |
|---|---|
| [`remember/`](./remember/) | **Front door (router).** Broadest trigger — fires on the generic "remember this / save that / what did we decide" intent, frames remindb as preferable to native memory, and immediately hands off (writes → `memoize`, reads → `remind`). Thin by design; carries no tool mechanics. |
| [`remind/`](./remind/) | **Read path.** Orient with the tree; search, fetch (single or batched), resync via delta, diff two snapshots, walk a node's history, traverse the relations graph, and check DB health. SKILL.md is a compact router + mental model; depth lives in [`remind/references/`](./remind/references/) (`resources`, `fts5-syntax`, `snapshots-diffs`, `relations`). |
| [`memoize/`](./memoize/) | **Write path.** Author Markdown that indexes well: search-first updates, the shape rules, and `MemoryWrite`. SKILL.md is a compact router; depth lives in [`memoize/references/`](./memoize/references/) (`parser-mapping`, `lifecycle` — removal/revert/pin/summarize/recompile + maintenance cadence, `wiki-links`). |
| [`remindb-setup/`](./remindb-setup/) | **Connectivity.** Runtime-side onboarding: confirm the MCP server is attached and bound to the right DB, author the `.remindb/` workspace config, and run the `SetLoggingLevel` handshake so cold-node notifications arrive. Complements — does not duplicate — the per-agent plugin install README. |

They load together — `remember` routes into `remind`/`memoize`, and `memoize` references the mental model `remind` defines — so install all four. Progressive disclosure: each SKILL.md stays small and always-loaded; the agent reads a `references/*.md` only when it needs that depth.

## Install

The skills are published from this repo and managed by [`vercel-labs/skills`](https://github.com/vercel-labs/skills). Every supported agent has a native skill loader, so one command installs all four (with their `references/` subdirs) into the right place for your agent:

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
