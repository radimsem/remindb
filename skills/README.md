# remindb skills

Two paired skills that teach an agent how to actually *use* remindb's MCP tool suite. Install them next to the [per-agent plugin](../plugins/) so the agent ships with both the MCP server and the know-how to drive it.

## What's here

| Skill | Purpose |
|---|---|
| [`remind/`](./remind/) | **Read path.** Orient with the tree, search and fetch under a token budget, resync via delta. Covers the node/snapshot/temperature mental model and the FTS5 query syntax. |
| [`memoize/`](./memoize/) | **Write path.** Author Markdown that indexes well into the node tree, search-first updates, cold-node summarization, source recompile. |

The two are designed to load together. `memoize` references the mental model that `remind` defines, so install both.

## Per-agent installation

The [`plugins/<agent>/`](../plugins/) folders install the MCP server. The skills are a separate manual install — every supported agent has a native skill loader, so installation is the same shape across the board: copy the two folders into the agent's skills directory and (where required) reload.

| Agent | Native loader | Where the skills go |
|---|---|---|
| **[Claude Code](../plugins/claude-code/)** | Yes | Copy both folders into `~/.claude/skills/`. Invoke per session via `/remind` or `/memoize`. |
| **[OpenClaw](../plugins/openclaw/)** | Yes | Copy both folders into `~/.openclaw/skills/`. Restart the gateway: `openclaw gateway restart`. |
| **[Codex](../plugins/codex/)** | Yes | Copy both folders into `~/.codex/skills/`. Restart the Codex TUI to pick them up. |
| **[Gemini CLI](../plugins/gemini-cli/)** | Yes | Copy both folders into `~/.gemini/skills/`. Run `/skills reload` (or `/skills refresh`) to pick them up. |
| **[OpenCode](../plugins/opencode/)** | Yes | Copy both folders into `~/.config/opencode/skills/`. Auto-discovered on next prompt. |

### Fast path — clone-and-copy

Every supported agent has a native skill loader. Clone once, copy into the agent's skills directory:

```bash
git clone https://github.com/radimsem/remindb /tmp/remindb
cp -r /tmp/remindb/skills/{remind,memoize} ~/.claude/skills/             # Claude Code
# or
cp -r /tmp/remindb/skills/{remind,memoize} ~/.codex/skills/              # Codex
# or
cp -r /tmp/remindb/skills/{remind,memoize} ~/.openclaw/skills/           # OpenClaw
# or
cp -r /tmp/remindb/skills/{remind,memoize} ~/.gemini/skills/             # Gemini CLI (then /skills reload)
# or
cp -r /tmp/remindb/skills/{remind,memoize} ~/.config/opencode/skills/    # OpenCode
```

OpenCode and Gemini CLI also walk up from the current directory looking for project-local skills — drop the folders into `.opencode/skills/` or `.gemini/skills/` (or the cross-agent `.agents/skills/` alias both honour) and commit them with the repo if you want the skills to travel with the project.

## Updating

Skills evolve with the MCP tool surface. Re-pull and recopy after a remindb upgrade:

```bash
cd /tmp/remindb && git pull
cp -r skills/{remind,memoize} ~/.claude/skills/   # or your agent's path
```

`cp -r` overwrites the existing skill folders in place. For Gemini CLI, run `/skills reload` afterwards.

## See also

- [Top-level README](../README.md) — what remindb is, the MCP server, benchmarks
- [`plugins/<agent>/README.md`](../plugins/) — per-agent MCP plugin install (the other half of setup)
