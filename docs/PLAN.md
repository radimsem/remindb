# remindb — Implementation Plan

> Token-efficient agentic memory database with MCP interface
> Language: Go · Storage: SQLite (libSQL) · Protocol: MCP · Format: TOON

---

## 1. Architecture Overview

The system has two distinct phases mirroring the diagram:

**Compiler Phase** (offline pipeline):
`Source Files → Parser/AST → Transformer → Diff Engine → DB Emitter → SQLite`

**MCP Runtime Phase** (online agent-facing):
`MCP Server → MCP Tools → Query Engine → SQLite → Injected Context → Agent/LLM`

A background rescan goroutine inside the MCP server bridges both phases — it receives file-change signals via channel and triggers incremental recompilation inline.

---

## 2. Project Structure

```
remindb/
├── cmd/
│   ├── remindb/              # Main CLI entrypoint
│   │   └── main.go           # compile / serve subcommands
│   └── inspector/            # Debug tool: dump DB state, query nodes
│       └── main.go
│
├── pkg/
│   ├── parser/               # COMPILER PHASE — Stage 1
│   │   ├── parser.go         # Unified parser dispatcher (by file extension)
│   │   ├── markdown.go       # Markdown → AST (gomarkdown/markdown)
│   │   ├── yaml.go           # YAML → AST (goccy/go-yaml)
│   │   ├── json.go           # JSON → AST (encoding/json + TOON encoding)
│   │   ├── preamble.go       # Preamble extraction (YAML/TOML metadata at file start)
│   │   └── ast.go            # Unified AST node definitions
│   │
│   ├── transformer/          # COMPILER PHASE — Stage 2 (enrichment, no restructuring)
│   │   ├── transformer.go    # Pipeline orchestrator
│   │   ├── anchor.go         # Anchor generation + minimization (content-hash based)
│   │   ├── compress.go       # Whitespace trimming, redundant char removal
│   │   ├── label.go          # Context label generation (short summary per node)
│   │   ├── tokenest.go       # Token count estimation (cl100k_base approximation)
│   │   └── prefix.go         # Prefix tree compression for repeated paths
│   │
│   ├── diff/                 # COMPILER PHASE — Stage 3
│   │   ├── engine.go         # Diff engine: compares current AST vs last snapshot
│   │   ├── delta.go          # Delta representation (add/remove/modify per node)
│   │   ├── hash.go           # Content hashing for change detection (xxhash)
│   │   └── cursor.go         # Cursor: position marker into version history
│   │
│   ├── emitter/              # COMPILER PHASE — Stage 4
│   │   ├── emitter.go        # Orchestrates DB writes from diff output
│   │   ├── schema.go         # CREATE TABLE statements, migrations
│   │   ├── insert.go         # INSERT nodes, diffs
│   │   └── fts.go            # FTS5 index population
│   │
│   ├── store/                # DATABASE LAYER (shared by compiler + runtime)
│   │   ├── db.go             # DB connection manager (libSQL / turso-go)
│   │   ├── schema.sql        # Raw SQL schema file
│   │   ├── nodes.go          # Node CRUD operations
│   │   ├── diffs.go          # Diff/snapshot storage operations
│   │   ├── snapshots.go      # Snapshot/version management
│   │   ├── cursors.go        # Cursor read/write
│   │   ├── search.go         # FTS5 search helpers
│   │   └── temperature.go    # Temperature read/write/decay logic
│   │
│   ├── query/                # MCP RUNTIME — Query Engine
│   │   ├── engine.go         # Budget-ranked context assembly
│   │   ├── budget.go         # Token budget allocation + enforcement
│   │   ├── traversal.go      # Tree traversal (ancestors via parent_id, descendants)
│   │   ├── rank.go           # Node ranking (temperature × relevance × recency)
│   │   └── format.go         # Output formatting (TOON for structured, plain for text)
│   │
│   ├── temperature/          # MCP RUNTIME — Temperature System
│   │   ├── tracker.go        # Access tracking + temperature updates
│   │   ├── decay.go          # Exponential decay function
│   │   ├── cold.go           # Cold node detection + summarization triggers
│   │   └── config.go         # Tunables: decay rate, cold threshold, check interval
│   │
│   ├── mcp/                  # MCP RUNTIME — Server + Tools
│   │   ├── server.go         # MCP server setup (modelcontextprotocol/go-sdk)
│   │   ├── tools.go          # Tool registration (MemoryFetch, MemoryWrite, etc.)
│   │   ├── tool_fetch.go     # MemoryFetch(anchor, budget) implementation
│   │   ├── tool_delta.go     # MemoryDelta(agent_id, node) implementation
│   │   ├── tool_write.go     # MemoryWrite(anchor, payload) implementation
│   │   ├── tool_summarize.go # MemorySummarize(node_id) implementation
│   │   ├── tool_search.go    # MemorySearch(query, budget) — FTS5 tool
│   │   ├── tool_history.go   # MemoryHistory(anchor, depth) — version browsing
│   │   ├── notifications.go  # Cold-node summarization notifications to client
│   │   ├── rescan.go         # Background goroutine: receives file-change signals via chan, triggers incremental recompile
│   │   └── middleware.go      # Logging, temperature tracking on every tool call
│   │
├── internal/
│   ├── tokens/               # Token estimation utilities
│   │   └── estimate.go       # BPE-approximate token counter
│   └── testutil/             # Shared test helpers
│       ├── fixtures.go       # Sample .md/.yaml/.json files
│       └── dbhelper.go       # In-memory DB for tests
│
├── schema/
│   ├── 001_initial.sql       # Initial migration
│   └── 002_temperature.sql   # Temperature column + indices
│
├── testdata/                 # Test fixture files
│   ├── sample.md
│   ├── sample.yaml
│   ├── sample.json
│   └── expected/             # Expected AST / DB outputs
│
├── go.mod
├── go.sum
├── Makefile
├── README.md
├── LICENSE                   # MIT
└── .goreleaser.yaml          # Release automation
```

---

## 3. SQLite Schema Design

```sql
PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;

-- Core content nodes (the "brain cells")
CREATE TABLE nodes (
    id           CHAR(11) PRIMARY KEY,      -- base62-encoded xxhash64, fixed 11 chars
    parent_id    CHAR(11) REFERENCES nodes(id) ON DELETE CASCADE,
    source_file  VARCHAR(512) NOT NULL,     -- origin file path (max realistic path)
    node_type    VARCHAR(16) NOT NULL,      -- heading|list|table|code|text|preamble|kv
    depth        TINYINT NOT NULL,          -- nesting depth in source (0-15 is plenty)
    label        VARCHAR(120) NOT NULL,     -- short context summary for agent
    content      TEXT NOT NULL,             -- token-compressed content (only field that needs TEXT)
    format       VARCHAR(8) NOT NULL DEFAULT 'plain', -- plain|toon (how to decode content)
    token_count  INTEGER NOT NULL,          -- estimated token count
    content_hash CHAR(16) NOT NULL,         -- xxhash64 hex-encoded, always 16 chars
    temperature  REAL NOT NULL DEFAULT 0.5, -- 0.0 (cold) to 1.0 (hot)
    access_count INTEGER NOT NULL DEFAULT 0,
    last_accessed_at INTEGER,               -- unix timestamp (smaller than DATETIME text)
    created_at   INTEGER DEFAULT (unixepoch()),
    updated_at   INTEGER DEFAULT (unixepoch())
);

-- Version snapshots (git-like linear history)
CREATE TABLE snapshots (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    cursor_hash CHAR(16) NOT NULL UNIQUE,   -- xxhash64 of full DB state
    parent_id   INTEGER REFERENCES snapshots(id),
    message     VARCHAR(256),               -- optional commit message
    created_at  INTEGER DEFAULT (unixepoch())
);

-- Per-node diffs between snapshots
CREATE TABLE diffs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot_id INTEGER NOT NULL REFERENCES snapshots(id),
    node_id     CHAR(11) NOT NULL,          -- which node changed
    op          CHAR(3) NOT NULL,           -- add|mod|rem (fixed 3-char codes)
    old_hash    CHAR(16),                   -- previous content_hash (NULL for add)
    new_hash    CHAR(16),                   -- new content_hash (NULL for rem)
    old_content TEXT,                        -- previous content (for rollback)
    new_content TEXT                         -- new content
);

-- Cursor: current position in version history (single-row table)
CREATE TABLE cursors (
    id          VARCHAR(8) PRIMARY KEY DEFAULT 'HEAD',
    snapshot_id INTEGER NOT NULL REFERENCES snapshots(id),
    updated_at  INTEGER DEFAULT (unixepoch())
);

-- FTS5 full-text search index
CREATE VIRTUAL TABLE nodes_fts USING fts5(
    label, content, node_type,
    content=nodes,
    content_rowid=rowid,
    tokenize='porter unicode61'
);

-- Indices for hot paths
CREATE INDEX idx_nodes_source      ON nodes(source_file);
CREATE INDEX idx_nodes_type        ON nodes(node_type);
CREATE INDEX idx_nodes_temperature ON nodes(temperature);
CREATE INDEX idx_nodes_parent      ON nodes(parent_id);
CREATE INDEX idx_diffs_snapshot    ON diffs(snapshot_id);
CREATE INDEX idx_diffs_node        ON diffs(node_id);
```

**Type sizing rationale:**
- `CHAR(11)` for node IDs: base62-encoded xxhash64 of `source_file + parent_id + content` — 11 chars fits the full 64-bit hash (~1.8×10¹⁹ values). Structural hash input means identical content at distinct tree positions gets distinct IDs, eliminating the dominant collision source
- `CHAR(16)` for content hashes: xxhash64 produces 8 bytes → 16 hex chars, always fixed length
- `CHAR(3)` for diff ops: `add`, `mod`, `rem` — fixed 3-char enum avoids string overhead
- `VARCHAR(16)` for node_type: longest value is `preamble` (8 chars), 16 gives headroom
- `VARCHAR(120)` for label: enforces the "1-line summary" constraint at the schema level
- `INTEGER` timestamps: unix epoch integers are 4–8 bytes vs DATETIME strings at 19 bytes, and sort/compare as native integers
- `TEXT` only where unbounded: `content`, `old_content`, `new_content` — these genuinely vary from 10 bytes to 100KB

---

## 4. Core Go Dependencies

| Purpose | Package | Why |
|---|---|---|
| Markdown parsing | `github.com/gomarkdown/markdown` | Rich AST with heading/list/table/code nodes, tree walking via `ast.WalkFunc` |
| YAML parsing | `github.com/goccy/go-yaml` | Fast, well-maintained, struct tag support |
| TOON encoding | `github.com/toon-format/toon-go` | Official Go implementation, encode + decode, struct tags |
| SQLite / libSQL | `github.com/tursodatabase/turso-go` | Embedded libSQL with zero-setup native lib; `database/sql` compatible. Fallback: `mattn/go-sqlite3` for wider platform support |
| MCP server | `github.com/modelcontextprotocol/go-sdk/mcp` | Official SDK (Google-maintained), typed tool handlers, notifications, stdio+SSE |
| Hashing | `github.com/cespare/xxhash/v2` | Fast non-cryptographic hash for content diffing |
| CLI framework | `github.com/spf13/cobra` | Subcommand CLI (`compile`, `serve`, `inspect`) |
| Logging | `log/slog` (stdlib) | Structured logging, zero dependencies |
| Testing | `github.com/stretchr/testify` | Assertions + test suites |

---

## 5. Implementation Phases

### Phase 1 — Parser Pipeline (Weeks 1–2)

**Goal:** Parse `.md`, `.yaml`, `.json`, `.toon` files into a unified internal AST.

**Tasks:**
1. Define the unified `ContextNode` struct that all parsers produce:
   ```go
   type ContextNode struct {
       ID          string         // generated anchor
       ParentID    string
       SourceFile  string
       NodeType    NodeType       // heading, list, table, code, text, kv, preamble
       Depth       int
       Label       string         // auto-generated context hint
       Content     string         // rendered content in `Format`
       Format      string         // plain | toon — how Content is encoded
       ContentHash string
       TokenCount  int
       Children    []*ContextNode
   }
   ```
2. Implement Markdown parser using `gomarkdown/markdown`:
   - Walk AST via `ast.WalkFunc`, map `ast.Heading`, `ast.List`, `ast.Table`, `ast.CodeBlock`, `ast.Paragraph` to `ContextNode`
   - Extract preamble (YAML/TOML metadata block) before parsing body
   - Generate heading-based hierarchy (H1 > H2 > H3 nesting)
3. Implement YAML parser using `goccy/go-yaml`:
   - Unmarshal into `any`, normalize `map[any]any` → `map[string]any` recursively
   - Apply the split rule: scalars stay inlined in the parent's `Content`; maps/arrays with ≥ `MaxInlineFields` (=5) entries promote to their own `ContextNode`
   - Sub-threshold subtrees render as block YAML-style text (sorted keys, deterministic)
4. Implement JSON parser:
   - Parse with `encoding/json` + `UseNumber()` for integer precision
   - Share the split rule and renderer with the YAML parser (see `split.go`)
   - **TOON encoding at parse time**: when a promoted array/map is uniform and TOON beats plain by a savings threshold (≥15%), store its subtree as TOON in `Content` and set `Format: "toon"`. Otherwise `Format: "plain"`. The transformer never restructures `Content`.
5. Preamble extraction (`preamble.go`): reuse the YAML parser on the front-matter block; relabel the produced root as `NodePreamble`.
6. Implement TOON parser: `toon.Decode` → shared `buildNode` path. Same split rule, TOON re-encoding, and promotion logic as JSON/YAML — source-format agnostic from `buildNode` down.
7. Write comprehensive parser tests against `testdata/` fixtures.

### Phase 2 — Transformer (Weeks 2–3)

**Goal:** Enrich parsed nodes (anchors, labels, token counts, compression) without restructuring them. Content form (plain vs TOON) and the split rule are decided by the parser in Phase 1 — the transformer only annotates and compresses.

**Tasks:**
1. **Whitespace trimming:** Strip redundant whitespace, normalize line endings, collapse blank lines
2. **Anchor minimization:** Generate short, stable anchors from `source_file + parent_id + content` (base62-encoded xxhash64, 11 chars). Structural hash input eliminates same-content collisions across tree positions
3. **Context label generation:** Auto-generate 1-line summaries per node:
   - Headings: use the heading text itself
   - Lists: "N-item list about {first item topic}"
   - Tables: "Table with columns: {col1, col2, ...} ({N} rows)"
   - Code blocks: "Code ({language}): {first line or function name}"
   - Text: first sentence, truncated to 80 chars
4. **Token estimation:** Implement a fast BPE-approximate counter (character-based heuristic: `len(text) * 0.75` for English, refined with a lookup for common token boundaries)
5. **Prefix tree compression:** For deeply nested YAML/JSON paths, compress common prefixes into shared anchors

### Phase 3 — Diff Engine (Weeks 3–4)

**Goal:** Compute minimal deltas between the current parse and the last snapshot.

**Tasks:**
1. Load the previous snapshot's node hashes from the DB
2. Walk the new AST and compare `content_hash` per anchor:
   - **Added:** node exists in new AST but not in DB
   - **Modified:** node exists in both but hash differs
   - **Removed:** node exists in DB but not in new AST
3. Generate `Delta` structs with old/new content for storage
4. Compute a `cursor_hash` (hash of all node hashes, sorted) to identify the full state
5. If `cursor_hash` matches HEAD, skip the emit phase entirely (no changes)

### Phase 4 — DB Emitter + Store (Weeks 4–5)

**Goal:** Persist nodes, diffs, and FTS index into SQLite.

**Tasks:**
1. Implement schema migrations (embed SQL files via `embed`)
2. Implement transactional batch inserts:
   - Upsert nodes (INSERT ON CONFLICT UPDATE)
   - Insert diffs referencing the new snapshot
   - Update FTS5 index (DELETE old, INSERT new for modified nodes)
3. Create new snapshot record, advance HEAD cursor
4. Implement the `store` package as a clean repository interface:
   ```go
   type Store interface {
       // Nodes
       GetNode(ctx context.Context, id string) (*Node, error)
       GetNodesByFile(ctx context.Context, path string) ([]*Node, error)
       GetChildren(ctx context.Context, parentID string) ([]*Node, error)
       GetAncestors(ctx context.Context, id string) ([]*Node, error)
       UpsertNode(ctx context.Context, node *Node) error
       DeleteNode(ctx context.Context, id string) error

       // Search
       Search(ctx context.Context, query string, limit int) ([]*Node, error)

       // Snapshots
       CreateSnapshot(ctx context.Context, msg string) (*Snapshot, error)
       GetSnapshot(ctx context.Context, id int) (*Snapshot, error)
       ListSnapshots(ctx context.Context, limit int) ([]*Snapshot, error)

       // Temperature
       UpdateTemperature(ctx context.Context, id string, temp float64) error
       IncrementAccess(ctx context.Context, id string) error
       GetColdNodes(ctx context.Context, threshold float64) ([]*Node, error)
   }
   ```

### Phase 5 — Temperature System (Week 5)

**Goal:** Track node usage heat and trigger cold-node summarization.

**Tasks:**
1. Implement exponential decay: `T(t) = T₀ × e^(-λ × Δt)` where `λ` is the configurable decay rate and `Δt` is time since last access in hours
2. On every `MemoryFetch` or `MemorySearch` that returns a node, increment its `access_count` and boost its temperature: `T_new = min(1.0, T_old + α)` where `α` is the access boost (e.g., 0.15)
3. Run a background goroutine (configurable interval, default 10 minutes) that:
   - Applies decay to all node temperatures
   - Identifies nodes below the cold threshold (default: 0.1)
   - Emits MCP notifications for cold nodes suggesting summarization
4. Implement temperature-aware query ranking: `score = relevance × (0.3 + 0.7 × temperature)` — cold nodes are still findable but deprioritized

### Phase 6 — Query Engine (Weeks 5–6)

**Goal:** Budget-aware context retrieval with tree traversal.

**Tasks:**
1. Implement budget-ranked retrieval:
   - Accept a token budget from the agent
   - Search/traverse to find candidate nodes
   - Rank by `score = fts_rank × temperature × recency_boost`
   - Greedily fill the budget: add highest-scored nodes until budget exhausted
   - Return results in TOON format for structured data, plain text for prose
2. Implement tree traversal via `parent_id`:
   - `ancestors(node_id, depth)`: recursive CTE walking parent_id up to root
   - `descendants(node_id, depth)`: recursive CTE walking children down
   - `siblings(node_id)`: nodes sharing the same parent_id
3. Implement delta-cursor queries: "what changed since snapshot X?"
4. Implement FTS5 search with rank-boosted temperature

### Phase 7 — MCP Server + Tools (Weeks 6–8)

**Goal:** Expose the full system as an MCP server.

**MCP Tools to implement:**

| Tool | Input | Behavior |
|---|---|---|
| `MemoryFetch` | `anchor: string, budget: int` | Retrieve context around an anchor within token budget |
| `MemorySearch` | `query: string, budget: int` | FTS5 search, return ranked nodes within budget |
| `MemoryWrite` | `anchor: string, payload: string` | Write/update content at anchor, trigger diff + snapshot |
| `MemoryDelta` | `since_snapshot?: int` | Return changes since a snapshot |
| `MemorySummarize` | `node_id: string` | Trigger summarization of a node (replace content with summary) |
| `MemoryHistory` | `anchor: string, depth: int` | Browse version history for a specific node |
| `MemoryTree` | `root?: string, depth: int` | Return the node tree structure (labels only, minimal tokens) |

**MCP Notifications:**

| Notification | Trigger | Payload |
|---|---|---|
| `remindb/cold_nodes` | Background timer | List of cold node IDs + labels suggesting summarization |
| `remindb/compile_complete` | Rescan goroutine recompile | Stats: nodes added/modified/removed |

**Implementation with official Go SDK:**
```go
server := mcp.NewServer(&mcp.Implementation{
    Name:    "remindb",
    Version: "0.1.0",
}, &mcp.ServerOptions{})

mcp.AddTool(server, &mcp.Tool{
    Name:        "MemoryFetch",
    Description: "Retrieve context around an anchor within a token budget",
}, handleMemoryFetch)

// Run on stdio for CLI integration, SSE for networked use
transport := &mcp.StdioTransport{}
server.Run(ctx, transport)
```

### Phase 8 — CLI + Rescan Goroutine (Week 8)

**Goal:** CLI entrypoints and background file rescan inside the MCP server.

**Tasks:**
1. Implement CLI with cobra:
   - `remindb compile <dir> -o brain.db` — one-shot compilation
   - `remindb serve brain.db --source <dir>` — start MCP server with background rescan
   - `remindb inspect brain.db` — dump DB stats, node tree, temperature map
2. Rescan goroutine inside `pkg/mcp/rescan.go`:
   ```go
   // RescanLoop runs inside the MCP server goroutine pool.
   // It receives signals on the channel (from CLI, from MemoryWrite,
   // or from a periodic ticker) and triggers incremental recompile
   // on only the changed files.
   func RescanLoop(ctx context.Context, signals <-chan RescanSignal, store Store, compiler Compiler, session *mcp.ServerSession) {
       ticker := time.NewTicker(rescanInterval) // configurable, default 30s
       defer ticker.Stop()
       for {
           select {
           case sig := <-signals:
               recompileFile(ctx, sig.Path, store, compiler, session)
           case <-ticker.C:
               recompileChanged(ctx, store, compiler, session) // stat all source files, diff mtime
           case <-ctx.Done():
               return
           }
       }
   }
   ```
3. On recompile completion, send `remindb/compile_complete` notification to MCP client with stats
4. Signal handling for clean shutdown (SIGINT/SIGTERM cancel the root context)

---

## 6. Design Recommendations & Improvements

### Use `mattn/go-sqlite3` instead of (or alongside) Turso

**Rationale:** `turso-go` gives you embedded libSQL with no setup, which is great. But `mattn/go-sqlite3` has a much larger ecosystem (migration tools, testing helpers, wider platform support including Windows and 32-bit). For a portable single-file `.db`, you don't need Turso's cloud sync features initially. **Recommendation:** Use `mattn/go-sqlite3` as the primary driver, offer Turso sync as an optional layer via build tags.

### Consider a hybrid approach to label generation

Rather than purely heuristic labels, consider using the LLM itself for label generation during `MemoryWrite`. When the agent writes new content, the MCP server can request back a 1-line summary. This makes labels significantly higher quality. The heuristic approach remains as the fallback for bulk compilation.

### Add a "context window" abstraction

Instead of just returning raw nodes, add a `ContextWindow` type that formats the result with clear section boundaries:
```
[heading:a3f8b2] Project Architecture
  [list:c7d1e9] 5-item list: core components...
  [table:b2a4f1] Table: dependencies (12 rows)
    ... (truncated, 847 tokens remaining in budget)
```
This gives the agent structural awareness without reading every node.

### TOON is not always optimal

TOON shines for uniform arrays of objects (~40% savings). For deeply nested config or prose-heavy markdown, the savings are minimal or negative. **Recommendation:** Add a threshold: only TOON-encode nodes where the estimated savings exceed 15%. Keep a `format` column on nodes (`toon|plain|compressed`) so the query engine knows how to decode.

### WAL mode is now in the schema

The `PRAGMA journal_mode=WAL` and `PRAGMA busy_timeout=5000` are included directly in the init schema above. This ensures safe concurrent reads (MCP query engine) during writes (rescan goroutine) without needing a separate write-serialization goroutine — WAL handles it natively.

### Alternative: consider Rust for the core engine

Go is a solid choice for the MCP server and CLI, but the parser/transformer/diff pipeline is CPU-intensive work where Rust would offer meaningfully better performance. A hybrid architecture — Rust core compiled to a shared library via CGO, Go for the MCP server — is worth considering if performance on large codebases matters. However, this adds significant build complexity. **Recommendation:** Start pure Go, profile, and extract hot paths to Rust only if needed.

### Add token estimation validation

Your BPE approximation (`len * 0.75`) will drift for code, CJK text, and structured formats. Consider bundling a pre-computed BPE vocabulary (tiktoken's `cl100k_base` is ~100KB) and doing exact counting for nodes above a size threshold (e.g., >500 chars). The `tiktoken-go` package exists for this.

---

## 7. Milestone Timeline

| Week | Milestone | Deliverable |
|---|---|---|
| 1–2 | Parser pipeline | Parse .md/.yaml/.json → unified AST, full test coverage |
| 2–3 | Transformer | Token compression, TOON encoding, label generation |
| 3–4 | Diff engine | Delta computation, content hashing, snapshot model |
| 4–5 | DB emitter + store | SQLite schema, CRUD, FTS5, migrations |
| 5 | Temperature system | Decay, access tracking, cold detection |
| 5–6 | Query engine | Budget-ranked retrieval, tree traversal via parent_id |
| 6–8 | MCP server | All 7 tools, notifications, rescan goroutine, middleware |
| 8 | CLI + polish | cobra CLI, integration tests, README, benchmarks |

---

## 8. Testing Strategy

- **Unit tests:** Every package gets `_test.go` files with table-driven tests
- **Integration tests:** End-to-end: parse fixture → compile → query via MCP tool → verify output
- **Benchmark tests:** `go test -bench` for parser throughput, query latency, TOON encode/decode
- **Fixture-based:** `testdata/` directory with known-good inputs and expected outputs
- **MCP conformance:** Use the MCP Inspector tool to validate the server against the spec

---

## 9. Key Risks & Mitigations

| Risk | Mitigation |
|---|---|
| TOON Go libraries are young, may have bugs | Pin to specific version, add round-trip tests (encode→decode→compare) |
| SQLite contention between query engine and rescan goroutine | WAL mode (in schema) + busy_timeout; writes are infrequent and batched per-file |
| Token estimation drift vs actual LLM tokenizer | Validate against tiktoken-go on test corpus, add calibration config |
| Cold-node summarization needs LLM access | Make it notification-only initially; agent decides whether to act |
| Large repos may have thousands of nodes | Add pagination to all queries, lazy-load content, index everything |

---

*This document serves as the implementation blueprint for remindb v0.1. Each phase can be developed and tested independently, with clean interfaces between layers.*

