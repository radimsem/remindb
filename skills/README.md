# remindb skills

Two paired skills that teach an agent how to actually *use* remindb's MCP tool suite. Install them next to the [per-agent plugin](../plugins/) so the agent ships with both the MCP server and the know-how to drive it.

## What's here

| Skill | Purpose |
|---|---|
| [`remind/`](./remind/) | **Read path.** Orient with the tree, search and fetch under a token budget, resync via delta. Covers the node/snapshot/temperature mental model and the FTS5 query syntax. |
| [`memoize/`](./memoize/) | **Write path.** Author Markdown that indexes well into the node tree, search-first updates, cold-node summarization, source recompile. |

The two are designed to load together. `memoize` references the mental model that `remind` defines, so install both.

## Per-agent installation

The [`plugins/<agent>/`](../plugins/) folders install the MCP server. The skills are a separate manual install — agents with a native skill loader get a folder drop; the rest paste each `SKILL.md` body into the agent's system-prompt context file.

| Agent | Native loader | Where the skills go |
|---|---|---|
| **[Claude Code](../plugins/claude-code/)** | Yes | Copy both folders into `~/.claude/skills/`. Invoke per session via `/remind` or `/memoize`. |
| **[OpenClaw](../plugins/openclaw/)** | Yes | Copy both folders into `~/.openclaw/skills/`. Restart the gateway: `openclaw gateway restart`. |
| **[Codex](../plugins/codex/)** | No (use context) | Append both `SKILL.md` files to a markdown file under `~/.codex/memories/` (or `~/.codex/memories_extensions/`). |
| **[Gemini CLI](../plugins/gemini-cli/)** | No (use context) | Append both `SKILL.md` files to `~/.gemini/GEMINI.md`. |
| **[OpenCode](../plugins/opencode/)** | No (use context) | Append both `SKILL.md` files to `~/.config/opencode/AGENTS.md` (global) or `.opencode/AGENTS.md` (per-project). |

### Fast path — clone-and-copy

For any agent with a native skill loader:

```bash
git clone https://github.com/radimsem/remindb /tmp/remindb
cp -r /tmp/remindb/skills/{remind,memoize} ~/.claude/skills/   # Claude Code
# or
cp -r /tmp/remindb/skills/{remind,memoize} ~/.openclaw/skills/ # OpenClaw
```

For agents without a native loader, append both skill bodies to the agent's context file:

```bash
git clone https://github.com/radimsem/remindb /tmp/remindb
cat /tmp/remindb/skills/remind/SKILL.md /tmp/remindb/skills/memoize/SKILL.md \
    >> ~/.gemini/GEMINI.md   # Gemini CLI — adjust path per agent
```

The frontmatter at the top of each `SKILL.md` is harmless when pasted into a context file; agents read the body either way.

## Updating

Skills evolve with the MCP tool surface. Re-pull and recopy after a remindb upgrade:

```bash
cd /tmp/remindb && git pull
cp -r skills/{remind,memoize} ~/.claude/skills/   # or your agent's path
```

For paste-installs, replace the previous block in the context file rather than appending again.

## See also

- [Top-level README](../README.md) — what remindb is, the MCP server, benchmarks
- [`plugins/<agent>/README.md`](../plugins/) — per-agent MCP plugin install (the other half of setup)
