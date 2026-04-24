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

```
     COMPILER PHASE (offline)                          MCP RUNTIME PHASE (online)

  ┌──────────────────────────────┐               ┌──────────────────────────────┐
  │ Source files                 │               │ MCP server                   │
  │ .md · .yaml · .json · .toon  │               │ Tool dispatcher · budgets    │
  └──────────────┬───────────────┘               └──────────────┬───────────────┘
                 │                                              │
  ┌──────────────▼───────────────┐               ┌──────────────▼───────────────┐
  │ Parser / AST builder         │               │ MCP tools                    │
  │ Headings · KV · preamble     │               │ MemoryTree · MemorySearch    │
  └──────────────┬───────────────┘               │ MemoryFetch · MemoryWrite    │
                 │                               │ MemoryDelta · MemoryHistory  │
  ┌──────────────▼───────────────┐               │ MemorySummarize · *Compile   │
  │ Transformer                  │               └──────────────┬───────────────┘
  │ Node typing · anchor IDs     │                              │
  │ TOON encoding · token est.   │               ┌──────────────▼───────────────┐
  └──────────────┬───────────────┘               │ Query engine                 │
                 │                               │ Budget-ranked · temp rank    │
  ┌──────────────▼───────────────┐               │ Tree traversal · FTS5 search │
  │ Diff engine                  │               └──────────────┬───────────────┘
  │ Delta · xxhash64 cursor_hash │                              │
  └──────────────┬───────────────┘               ┌──────────────▼───────────────┐
                 │                               │ Injected context             │
  ┌──────────────▼───────────────┐               │ Minimal tokens · task-scoped │
  │ DB emitter                   │               └──────────────┬───────────────┘
  │ INSERT nodes · diffs · FTS5  │                              │
  └──────────────┬───────────────┘               ┌──────────────▼───────────────┐
                 │                               │ Agent / LLM                  │
  ┌──────────────▼───────────────┐               └──────────────────────────────┘
  │ Rescan loop                  │
  │ Polls source · incremental   │
  └──────────────────────────────┘

                ┌──────────────────────────────────────────────┐
                │ SQLite .db  ·  one portable file             │
                │ nodes · snapshots · diffs                    │
                │ cursors · nodes_fts (FTS5)                   │
                └──────────────────────────────────────────────┘

  DB emitter    →  writes nodes · diffs · the new snapshot in one transaction
  Query engine  →  reads ranked nodes + FTS5 hits, trimmed to the token budget
  Temperature   →  bumps access counts on read · decays on a 10-minute tick
  Rescan loop   →  triggers Parser → … → DB emitter on source-file change
```

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
| Claude Code | [`claude-code/`](./claude-code/) | [claude-code/README.md](./claude-code/README.md) |
| Gemini CLI | [`gemini-cli/`](./gemini-cli/) | [gemini-cli/README.md](./gemini-cli/README.md) |
| Codex | [`codex/`](./codex/) | [codex/README.md](./codex/README.md) |
| OpenCode | [`opencode/`](./opencode/) | [opencode/README.md](./opencode/README.md) |
| OpenClaw | [`openclaw/`](./openclaw/) | [openclaw/README.md](./openclaw/README.md) |

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

Token counts are measured against the naive baseline an agent falls back to without a memory layer: list the directory, read every matching file, grep through it. Numbers come from `./scripts/bench-agents.sh` running the current `testdata/` vaults.

```
=== openclaw ==================================================================
scenario                               naive (tok)  remindb (tok)  saved
tree                                   ~12128       ~8380          +30.9%
search:WebSocket persistent connecti…  ~13835       ~620           +95.5%
search:Sentry alert threshold deploy…  ~9323        ~802           +91.4%
search:stale memory flagged review     ~14195       ~953           +93.3%
fetch                                  ~3206        ~675           +78.9%
delta                                  ~583         ~28            +95.2%
total                                  ~53270       ~11458         +78.5%

=== claude-code ===============================================================
scenario                               naive (tok)  remindb (tok)  saved
tree                                   ~6199        ~3353          +45.9%
search:Stripe webhook idempotency ke…  ~8359        ~639           +92.4%
search:PostgreSQL connection pool Pg…  ~3919        ~296           +92.4%
search:drizzle migration NOT NULL ba…  ~7981        ~236           +97.0%
fetch                                  ~2733        ~333           +87.8%
delta                                  ~2983        ~28            +99.1%
total                                  ~32174       ~4885          +84.8%

=== codex =====================================================================
scenario                               naive (tok)  remindb (tok)  saved
tree                                   ~6950        ~2989          +57.0%
search:WebSocket operator Vendor C b…  ~14510       ~605           +95.8%
search:dead letter queue rejected re…  ~4892        ~456           +90.7%
search:Snowflake COPY INTO parquet     ~6665        ~542           +91.9%
fetch                                  ~2867        ~393           +86.3%
delta                                  ~824         ~28            +96.6%
total                                  ~36708       ~5013          +86.3%

=== gemini-cli ================================================================
scenario                               naive (tok)  remindb (tok)  saved
tree                                   ~6155        ~2952          +52.0%
search:exponential backoff jitter De…  ~4585        ~231           +95.0%
search:PLAT 1903 retry storm ConfigM…  ~7264        ~537           +92.6%
search:Vault token renewer silent al…  ~5849        ~520           +91.1%
fetch                                  ~2807        ~448           +84.0%
delta                                  ~3010        ~28            +99.1%
total                                  ~29670       ~4716          +84.1%
```

> [!NOTE]
> **Small corpora, small sessions — and the delta matters.** These vaults are ~3k–20k tokens each, kept small so the repository doesn't balloon with fixture data. The scenario list (tree · 3 searches · fetch · delta) is also intentionally short — a realistic agent session is *much* longer. A typical 30-minute coding session does dozens of orient / search / fetch / write / re-orient cycles, and the same search often fires three or four times as the agent loops on a problem. Each of those calls is where `remindb` turns a ~15k-token re-read into a 500-token budgeted answer.
>
> A real knowledge base — say, an Obsidian vault with 100 articles (~300k tokens) — pushes the numbers in remindb's favour, not against it: the naive baseline scales linearly with corpus size (more files to list, more bytes to grep, more prose to re-read), while remindb's answers stay bounded by the token budget you pass. Expected **session-level savings on realistic corpora run 85–99%**, and **full coding sessions trend toward 90%+ total token reduction** as orient/search/fetch calls compound.

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
