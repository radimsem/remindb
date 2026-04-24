<p align="center">
  <img src="assets/logo.png" alt="remindb logo" width="400" />
</p>

<h1 align="center">remindb</h1>

<p align="center">
  Token-efficient agentic memory database with an MCP interface.
  <br />
  One portable <code>.db</code> file your agents read, write, search, and version — so they stop wasting context re-reading the same files.
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue" alt="License" /></a>
  <img src="https://img.shields.io/badge/go-1.23%2B-00ADD8" alt="Go 1.23+" />
  <img src="https://img.shields.io/badge/storage-SQLite-003B57" alt="SQLite" />
  <img src="https://img.shields.io/badge/protocol-MCP-6f42c1" alt="MCP" />
</p>

---

## The Problem

An AI agent opens your project, sees a folder of notes, and does what an LLM does best: reads every file, again, from scratch. Each session starts at zero. Each `Read` and `Grep` burns thousands of tokens reproducing context the agent already had yesterday. There is no memory — only the context window the host rebuilt this morning.

Long-context models don't fix this. A 1M-token window is still paid per call, still re-tokenized, and still can't tell yesterday's stale note from today's relevant one. Raw files are the wrong substrate for memory.

## The Idea

`remindb` is a single portable SQLite file your agents treat as long-term memory. It parses Markdown, YAML, JSON, JSONL, and TOON into a structured AST, stores each node with a content hash, encodes repetitive structures as TOON when it saves tokens, and exposes the whole thing through eight purpose-built MCP tools.

A few pieces hold it together:

- **Memory Tree as the agent's index.** Instead of listing a directory and reading every file to orient, the agent calls `MemoryTree` once. Each entry is a typed node — `[heading]`, `[list]`, `[kv]`, `[table]`, `[preamble]`, `[text]`, `[code]` — with an ID, a short label, a temperature, and a token count. Think `ls -la` for memory: one call, a scannable index, hot stuff floats up.

  A slice of a real tree (as printed by `remindb inspect --tree`):

  ```
  [preamble] Preamble: framework, language, project (id=3kGXxidmWBp file=CLAUDE.md temp=0.50 tok=14)
  [heading] Project Instructions (id=6EuIVj5zt5j file=CLAUDE.md temp=0.75 tok=5)
    [heading] Architecture (id=603qfsg4qd2 file=CLAUDE.md temp=0.88 tok=3)
      [text] Next.js 15 conventions with a clear separation of data… (id=3GGuLAq3yNP file=CLAUDE.md temp=0.82 tok=111)
      [list] 7-item list: app/, components/, lib/, db/, hooks/, types… (id=ITAKw5NVNPt file=CLAUDE.md temp=0.71 tok=228)
    [heading] Data Model (id=FQwpXL4bm6Y file=CLAUDE.md temp=0.62 tok=3)
      [list] 7-item list: products, variants, orders, carts, users, s… (id=Il8jcgTJOGt file=CLAUDE.md temp=0.55 tok=155)
    [heading] Payment Integration (id=LTQZLSkPsDW file=CLAUDE.md temp=0.30 tok=5)
      [text] Stripe Payment Intents; not legacy Checkout Sessions… (id=GLbXrUYs32G file=CLAUDE.md temp=0.24 tok=35)
    [heading] Observability (id=2wkOdf47OjR file=CLAUDE.md temp=0.08 tok=4)
      [list] 4-item list: Sentry · Vercel logs · OTel tracing · Prom… (id=C1HCYSAOkpu file=CLAUDE.md temp=0.08 tok=90)
  ```

  A fresh compile starts every node at `temp=0.50`; the spread above is what an agent sees after it's been reading for a while. "Architecture" is hot because the agent keeps coming back to it; "Observability" is nearly cold and will show up on the next round of summarization nudges.

- **Temperature — what's hot vs. cold.** Each node has a temperature between 0 and 1. Reads bump it up (`T += 0.15`), a background tick cools everything down (`T = T · e^(-λ·Δt)`). Ideas the agent keeps returning to stay warm and rank higher; notes nobody touches drift toward the cold threshold and get flagged for summarization. Ranking uses `score = relevance × (0.3 + 0.7 × temperature)` — cold nodes don't disappear, they just stop crowding the top of the results.

- **Git-style versioning.** Every `compile` or `MemoryWrite` lands a snapshot — a linear parent chain with a `cursor_hash` that fingerprints the whole DB state. Per-node diffs (`add` / `mod` / `rem`, with old and new content) sit alongside. `MemoryDelta` then hands the agent *only* what changed since its last cursor — a tiny resync instead of a whole-file re-read.

- **TOON encoding at rest.** Arrays of uniform objects (configs, tables, list-of-dicts) store ~40% smaller in [TOON](https://github.com/johannschopplich/toon) than in plain YAML or JSON. The parser tries both representations for each structured node, keeps whichever wins by ≥15%, and records the choice in a `format` column. The query layer decodes on read. Irregular prose stays as plain text — TOON has nothing to offer there, so we don't pretend.

- **FTS5 search, not grep.** Search runs on SQLite's FTS5 virtual table, built at write time with a porter tokenizer over labels, content, and types. `MemorySearch` returns ranked anchors in milliseconds — no file rescans, no regex timeouts — and trims to whatever token budget the agent passes. Ask for 500 tokens of matches and that's exactly what you get back.

- **Portable by design.** The entire memory is one `.db` file. Copy it to another machine, hand it to another agent, commit it into a repo, sync it across devices. No server, no daemon, no external state. Any MCP-capable agent — Claude Code, Codex, Gemini CLI, OpenCode, OpenClaw — can point `serve` at the same file and share the same knowledge.

## Installation

### Quick install

**Linux / macOS:**

```bash
curl -fsSL https://raw.githubusercontent.com/radimsem/remindb/main/install.sh | bash
```

By default the binary is installed to `~/.local/bin/remindb`. Pick a different prefix with `--prefix`:

```bash
curl -fsSL https://raw.githubusercontent.com/radimsem/remindb/main/install.sh | bash -s -- --prefix ~/.cargo
```

**Windows (PowerShell 5.1+):**

```powershell
iwr -useb https://raw.githubusercontent.com/radimsem/remindb/main/install.ps1 | iex
```

The binary lands at `%LOCALAPPDATA%\Programs\remindb\bin\remindb.exe`. Override with `-Prefix`:

```powershell
./install.ps1 -Prefix C:\tools\remindb
```

### From source (Go 1.23+)

```bash
git clone https://github.com/radimsem/remindb.git
cd remindb
go build -o ~/.local/bin/remindb ./cmd/remindb
```

Verify:

```bash
remindb --help
```

## Architecture

Two phases, one SQLite file between them. The compiler runs offline and turns source files into versioned nodes; the MCP runtime runs online and answers the agent in milliseconds. The `.db` is the whole handoff — copy it, commit it, sync it.

<p align="center">
  <img src="assets/arch.svg" alt="remindb architecture" width="100%" />
</p>

| Layer | Responsibility |
|-------|----------------|
| **Parser** | One dispatcher, format-specific stages for Markdown, YAML, JSON/JSONL, TOON. Emits a unified `[]*ContextNode` tree with `id`, `parent_id`, `label`, `content`, `node_type`, `depth`, `token_count`, `content_hash`. |
| **Transformer** | Generates 11-char base62 IDs (xxhash64), estimates cl100k-base tokens, compresses whitespace, decides plain vs. TOON per node. |
| **Diff Engine** | Compares the fresh AST against the last snapshot, produces `add`/`mod`/`rem` deltas, hashes the full state into a new `cursor_hash`. |
| **Emitter** | Writes nodes, diffs, and the new snapshot in one transaction; maintains the FTS5 index via triggers. |
| **Store** | SQLite with WAL mode. Tables: `nodes`, `snapshots`, `diffs`, `cursors`, plus the `nodes_fts` virtual table. |
| **Query Engine** | Token-budgeted context assembly. Traverses ancestors and descendants via `parent_id`, ranks with `relevance × (0.3 + 0.7 × temperature)`, formats output. |
| **Temperature** | Access boost on read, exponential decay on a tick. Tunables: `DecayRate=0.05`, `AccessBoost=0.15`, `ColdThreshold=0.1`, `TickInterval=10m`. |
| **MCP Server** | `modelcontextprotocol/go-sdk` over stdio. Registers eight tools, dispatches to the query engine, and notifies clients when nodes cross the cold threshold. |
| **Rescan Loop** | Optional background goroutine that polls the source directory and triggers incremental recompilation without bringing the server down. |

## CLI

Four subcommands, one flag (`--db`) shared by all. When `--db` is omitted and the command takes a directory, `remindb` derives `./<dirname>.db` automatically.

```
remindb compile <path>   Ingest files or a directory into the database
remindb serve            Start the MCP server (stdio)
remindb inspect          Dump DB stats; optionally render the node tree
remindb bench            Measure token savings vs. raw-file baselines
```

### `compile`

One-shot ingestion of a file or directory. Creates a new snapshot; records diffs against the previous one.

```bash
remindb compile ./notes # → ./notes.db
remindb compile ./notes --db memory.db -m "add Q2 notes"
remindb compile ./docs/architecture.md --db project.db
```

| Flag | Purpose |
|------|---------|
| `--db PATH` | Target database. Default: derived from the source directory name, else `memory.db`. |
| `-m, --message` | Snapshot message (defaults to `compile:<path>`). |

### `serve`

Starts the MCP server on stdio. When `--source` is set, remindb runs an initial compile (if the DB is empty) and keeps a background rescan loop running.

```bash
remindb serve --db ./notes.db --source ./notes
remindb serve --db ./notes.db --source ./notes --rescan-interval 30s -v
```

| Flag | Env | Purpose |
|------|-----|---------|
| `--db` | `REMINDB_DB` | Database file. |
| `--source` | `REMINDB_SOURCE` | Source directory to watch and incrementally recompile. |
| `--rescan-interval` | `REMINDB_RESCAN_INTERVAL` | e.g. `30s`, `5m`. `0` keeps the tracker's default. |
| `-v, --verbose` | — | Debug-level logs. Default is info. |

### `inspect`

Read-only snapshot of what's in a database. Without `--tree` it prints stats; with `--tree` it renders the node hierarchy with temperatures colour-coded blue (cold) → red (hot).

```bash
remindb inspect --db ./notes.db
remindb inspect --db ./notes.db --tree --depth 6
```

| Flag | Purpose |
|------|---------|
| `--tree` | Render the node tree. |
| `--depth N` | Maximum depth when rendering. Default: `10`. Requires `--tree`. |

`NO_COLOR=1` disables the ANSI palette.

### `bench`

Runs the scenario suite — tree · search · fetch · delta — against one database and prints token savings compared to a naive "list + read + grep" baseline.

```bash
remindb bench \
  --db ./notes.db --dir ./notes --budget 1000 \
  --query "WebSocket idempotency" --query "Snowflake COPY INTO"
```

| Flag | Purpose |
|------|---------|
| `--dir` | Source directory (inferred from the DB path if omitted). |
| `--budget` | Token budget for search and fetch scenarios. Default: `1000`. |
| `--query` | Repeatable. Skips the search scenario when empty. |

## MCP tools

Eight tools, registered once, surfaced to any MCP-capable agent (Claude Code, Codex, Gemini CLI, OpenCode, OpenClaw, …).

| Tool | Purpose |
|------|---------|
| **`MemoryTree`** | Renders the full node hierarchy with labels, types, IDs, temperatures, and token counts. The agent's cheap orientation call. |
| **`MemorySearch`** | FTS5 full-text search over labels and content. Returns ranked anchors within a token budget. |
| **`MemoryFetch`** | Returns one anchor plus its ancestors and children, trimmed to a token budget. The agent's "read just this region" call. |
| **`MemoryWrite`** | Writes or updates content at an anchor. Creates a new snapshot and a per-node diff. |
| **`MemoryDelta`** | Returns only the nodes that changed since a given snapshot cursor. Lets agents resync with a tiny payload instead of re-reading files. |
| **`MemoryHistory`** | Browses the version history of a specific node — who/when/how it changed, rollback-capable via stored old content. |
| **`MemorySummarize`** | Replaces a node's content with a shorter summary provided by the agent. Used when the temperature tracker flags a cold node for compaction. |
| **`MemoryCompile`** | Compiles source files or a directory into the database from within a session. Same engine as the `compile` CLI. |

### Agent integrations

Five ready-to-install plugin folders ship with the repo, one per supported coding agent. Each has a manifest matching that agent's spec, an MCP stanza, and a README with install commands, env-var conventions, and a concrete example that compiles the agent's own memory folder into remindb.

| Agent | Folder | Install docs |
|-------|--------|--------------|
| Claude Code | [`plugins/claude-code/`](./plugins/claude-code/) | [plugins/claude-code/README.md](./plugins/claude-code/README.md) |
| Gemini CLI | [`plugins/gemini-cli/`](./plugins/gemini-cli/) | [plugins/gemini-cli/README.md](./plugins/gemini-cli/README.md) |
| Codex | [`plugins/codex/`](./plugins/codex/) | [plugins/codex/README.md](./plugins/codex/README.md) |
| OpenCode | [`plugins/opencode/`](./plugins/opencode/) | [plugins/opencode/README.md](./plugins/opencode/README.md) |
| OpenClaw | [`plugins/openclaw/`](./plugins/openclaw/) | [plugins/openclaw/README.md](./plugins/openclaw/README.md) |

Pair the plugin with the companion [`efficient-memo`](./skills/efficient-memo/) skill — it teaches the agent the FTS5 query format, token-budget conventions, and the `MemoryTree → MemorySearch → MemoryFetch` chain so you don't re-explain them each session. Drop the folder into your agent's user-scope skills directory (Claude Code: `~/.claude/skills/efficient-memo/`; OpenClaw: the equivalent user skills path). Agents without a native skill loader can paste `SKILL.md` into their `AGENTS.md` / system-prompt equivalent.

For any other MCP-capable agent, add this stanza to its MCP config by hand:

```json
{
  "mcpServers": {
    "remindb": {
      "type": "stdio",
      "command": "remindb",
      "args": ["serve", "--db", "/absolute/path/to/memory.db", "--source", "/absolute/path/to/notes"],
      "env": {}
    }
  }
}
```

On startup, the agent sees eight `Memory*` tools alongside its usual toolbox. A reasonable first prompt:

```
Call MemoryTree to orient. Then call MemorySearch for "<topic>" with budget 1000
and MemoryFetch on the top hit. Explain what you learned and which files it came from.
```

## Benchmarks

Token counts are measured against the naive baseline an agent falls back to without a memory layer: list the directory, read every matching file, grep through it. Numbers come from `./scripts/bench-agents.sh` over the five plugin fixtures in `testdata/`, plus a one-off compile of a real Obsidian vault (117 markdown files across AI concepts, market briefs, security notes, and MOCs — ~619k naive tokens end-to-end).

The scenario suite (tree · 3 searches · fetch · delta) rolls up into three workflow categories:

- **context window** — a single `MemoryTree` orientation call.
- **context gathering** — 3 × `MemorySearch` + `MemoryFetch` + `MemoryDelta`, token-weighted.
- **total session** — sum of both.

<p align="center">
  <img src="assets/bench.svg" alt="remindb token savings by scenario category" width="100%" />
</p>

<sub>The `obsidian vault` row is a real vault at `~/Documents/Brain`: 117 markdown files, ~619k naive tokens, 3 190 compiled nodes.</sub>

> [!NOTE]
> **Corpus size moves the numbers in remindb's favour.** The plugin fixtures are ~3k–20k tokens each; the Brain vault is ~619k. As the corpus grows, the naive baseline scales linearly (more files to list, more bytes to grep, more prose to re-read), while remindb's answers stay bounded by the token budget you pass. That's why the vault's context-gathering row hits **99.3 %** — every search still returns ~800 tokens, but the baseline is now 15–20× larger.
>
> The scenario list is also intentionally short. A real 30-minute agent session does dozens of orient/search/fetch/write/re-orient cycles, and the same search often fires three or four times as the agent loops on a problem. Each of those calls compounds toward **90 %+ full-session savings** on realistic corpora.

Reproduce the table yourself:

```bash
./scripts/bench-agents.sh
```

Or run a Go-level micro-benchmark against your own vault:

```bash
go test -run='^$' -bench='BenchmarkTokens' -benchtime=1x .
```

## License

MIT — see [`LICENSE`](LICENSE).

## Support

I'm a college student building agentic AI tooling in the evenings and weekends between classes. `remindb` is free, MIT-licensed, and will stay that way — no telemetry.

If this saved you tokens (or saved you from reading the same 100 files for the hundredth time), tossing even a small support helps.

Thanks for reading this far. If you end up using `remindb` in anger, I'd love to hear what you built — open an issue with a short story, or drop a star. Both matter more than you'd think.
