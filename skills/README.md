# remindb skills

Two paired skills that teach agents how to use remindb's MCP tool suite efficiently. Install them alongside the [per-agent plugin](../plugins/) so the agent ships with both the MCP server *and* the usage know-how.

## What's here

| Skill | Purpose |
|---|---|
| [`efficient-memo/`](./efficient-memo/) | **Read path.** Orient with the tree, search/fetch under a token budget, resync via delta. Covers the node/snapshot/temperature mental model and FTS5 query syntax. |
| [`memoize/`](./memoize/) | **Write path.** Author Markdown that indexes well into the node tree, search-first updates, cold-node summarization, source recompile. |

The two are designed to be loaded together. `memoize` references the mental model that `efficient-memo` defines; install both.

## Per-agent installation

The [`plugins/<agent>/`](../plugins/) folders install the MCP server. The skills are a separate, manual install — agents with a native skill loader get a folder drop; the rest paste each `SKILL.md` body into the agent's system-prompt context file.

| Agent | Native loader | Where to put each skill |
|---|---|---|
| **[Claude Code](../plugins/claude-code/)** | Yes | Copy both folders into `~/.claude/skills/`. Invoke per session via `/efficient-memo` or `/memoize`. |
| **[OpenClaw](../plugins/openclaw/)** | Yes | Copy both folders into `~/.openclaw/skills/`. Restart the gateway: `openclaw gateway restart`. |
| **[Codex](../plugins/codex/)** | No (use context) | Append both `SKILL.md` files to a markdown file under `~/.codex/memories/` (or `~/.codex/memories_extensions/`). |
| **[Gemini CLI](../plugins/gemini-cli/)** | No (use context) | Append both `SKILL.md` files to `~/.gemini/GEMINI.md`. |
| **[OpenCode](../plugins/opencode/)** | No (use context) | Append both `SKILL.md` files to `~/.config/opencode/AGENTS.md` (global) or `.opencode/AGENTS.md` (per-project). |

### Fast path — clone-and-copy

For any agent with a native skill loader:

```bash
git clone https://github.com/radimsem/remindb /tmp/remindb
cp -r /tmp/remindb/skills/{efficient-memo,memoize} ~/.claude/skills/   # Claude Code
# or
cp -r /tmp/remindb/skills/{efficient-memo,memoize} ~/.openclaw/skills/ # OpenClaw
```

For agents without a native loader, append both skill bodies to the agent's context file:

```bash
git clone https://github.com/radimsem/remindb /tmp/remindb
cat /tmp/remindb/skills/efficient-memo/SKILL.md /tmp/remindb/skills/memoize/SKILL.md \
    >> ~/.gemini/GEMINI.md   # Gemini CLI — adjust path per agent
```

The frontmatter at the top of each `SKILL.md` is harmless when pasted into a context file; agents read the body either way.

## Updating

Skills evolve with the MCP tool surface. Re-pull and recopy after a remindb upgrade:

```bash
cd /tmp/remindb && git pull
cp -r skills/{efficient-memo,memoize} ~/.claude/skills/   # or your agent's path
```

For paste-installs, replace the previous block in the context file rather than appending again.

## See also

- [Top-level README](../README.md) — what remindb is, the MCP server, benchmarks
- [`plugins/<agent>/README.md`](../plugins/) — per-agent MCP plugin install (the other half of setup)
