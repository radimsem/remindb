# Configuration — the `.remindb/` directory

> Everything that tunes remindb lives in one folder at the source root. All of it is optional; missing means defaults.

[← back to README](../README.md) · related: [CLI](./cli.md) · [temperature](./temperature.md)

remindb keeps its workspace-level state in a `.remindb/` directory at the source root:

| Entry | Purpose |
|------|---------|
| `.remindb/config.json` | Runtime configuration (knobs and feature blocks). |
| `.remindb/ignore` | Gitignore-style exclude patterns. |
| `.remindb/temperatures.json` | Per-path initial-temperature overrides. |
| `.remindb/sessions/` | Machine-managed per-client session ledger ([below](#session-ledger-remindbsessions)). |
| `.remindb/logs/` | Opt-in per-session tool-call/error logfiles ([below](#session-logfiles-remindblogs)). |

The three files are optional; missing → defaults. The whole directory is skipped during source walks, so its contents never end up as memory nodes.

## Runtime config: `.remindb/config.json`

A single JSON object of feature blocks. Unknown top-level or nested keys are rejected at startup — that catches typos like `"redact"` vs `"redaction"` before they silently no-op. A missing file or an empty `{}` means all defaults.

```json
{
  "budgets": {
    "search": 1000,
    "fetch": 1500,
    "fetch_batch": 4000,
    "related": 1000
  },
  "compile": {
    "max_file_size": "2GB",
    "max_parallelism": 4,
    "wall_clock_timeout": "10m"
  },
  "temperature": {
    "enabled": true,
    "decay_rate": 0.03,
    "access_boost": 0.2,
    "cold_threshold": 0.08,
    "notify_threshold": 0.07,
    "summarize_rebound": 0.6,
    "tick_interval": "10m",
    "cold_notify_ttl": "2h",
    "cold_notify_limit": 100
  },
  "redaction": {
    "disable_builtin_kinds": ["env_secret_assignment"],
    "custom": [
      { "kind": "internal_token", "pattern": "INT-[0-9a-f]{32}" }
    ]
  },
  "rescan": {
    "enabled": true,
    "interval": "30s",
    "settle": "500ms"
  },
  "server": {
    "transport": "http",
    "listen": "127.0.0.1:7474",
    "logging": {
      "level": "debug",
      "format": "json",
      "output_path": "/var/log/remindb.log",
      "buffer_size": 1000,
      "session_files": {
        "enabled": true,
        "max_file_size": "10MB"
      }
    },
    "resources": {
      "debounce": "500ms",
      "overrides": { "logs": "1s", "temperature": "2s" }
    },
    "sessions": {
      "flush_interval": "30s"
    }
  }
}
```

Every field in every block is optional — only the keys you set override the default; the rest keep the engine baseline. Durations are strings (`"10m"`, `"2h"`). Out-of-range values fail startup with the offending field named, rather than silently clamping.

**`temperature`** overrides the decay/boost policy engine-wide. This is distinct from `.remindb/temperatures.json`, which sets *per-path initial* temperatures — this block changes the *policy* (how fast everything decays, where the cold line sits). See [temperature](./temperature.md) for what each knob does. Like `rescan`, the `serve` temperature ticker is **live-reloaded**: at the top of every tick it content-hashes `config.json` and, if it changed, re-sources this block — no restart. Absent → engine defaults (`enabled: true`). `enabled: false` **freezes the brain** — each tick performs no decay and no cold-node notification while the loop keeps ticking and keeps re-reading config; flip it back to `true` and decay/notify resume on the very next tick, no restart, no re-enable trap. An invalid edit (bad JSON, out-of-range knob) is logged `Warn` and the last-good policy is kept; the server never crashes on a bad reload. When disabled, no positive `tick_interval` is required (the loop falls back to the bootstrap default to keep re-reading). Read by `serve`; `compile` validates the block but doesn't apply it (it has no running tracker).

**`redaction`** configures the secret-scrubber applied on ingest by both `compile` and `serve`. By default every built-in detector is active; `disable_builtin_kinds` mutes the kinds you list (the rest stay on — see the kind list in `internal/redaction/patterns.go`). `custom` *adds* your own `{ "kind", "pattern" }` regexes on top. An unknown kind or an invalid regex fails startup with the offending name reported.

**`compile`** bounds the ingest pipeline for `compile`, the `serve` rescan loop, and the `MemoryCompile` tool — so a client-triggered compile behaves identically to the CLI. Absent → current behavior (unbounded file size, `GOMAXPROCS` parallelism, no deadline). `max_file_size` takes a size string (`"2GB"`, `"500MB"`, or a bare 1024-based byte count) — a file over the limit is **skipped with a `Warn` naming the path**, never an error, so the rest of the tree still compiles. `max_parallelism` caps the per-file worker pool. `wall_clock_timeout` aborts a runaway compile with a clear error; because emission is transactional, a timeout commits **no partial state**.

**`rescan`** tunes the `serve` background rescan loop and, like `temperature`, is **live-reloaded**: at the top of every tick the loop content-hashes `config.json` and, if it changed, re-sources this block — no restart. Absent → defaults (`enabled: true`, `interval: "30s"`, `settle: "500ms"`). `interval` is how often the workspace is walked; `settle` ignores files modified within that window (debounces mid-save writes). `enabled: false` makes each tick a **no-op** (no walk, no compile) while the loop keeps ticking and keeps re-reading config — flip it back to `true` and scanning resumes on the very next tick, no restart, no re-enable trap. An invalid edit (bad JSON, `interval <= 0`, negative `settle`) is logged `Warn` and the last-good settings are kept; the server never crashes on a bad reload. Because this block is re-sourced at runtime, once `serve` is running it is authoritative over the `--rescan-interval` flag and `REMINDB_RESCAN_INTERVAL`, which only seed the interval until the first config read and when no `rescan` block is present.

**`budgets`** sets the default token budget for the four read tools that take one — `MemorySearch`, `MemoryFetch`, `MemoryFetchBatch`, `MemoryRelated`. Resolution is per-tool and local: an explicit positive `budget` on the call always wins; otherwise the configured default; otherwise the built-in. `MemoryRelated`'s built-in is 1000; the other three treat an unset budget as **unlimited** (no trimming). Write tools are unaffected.

**`server`** configures `serve` itself. `transport` (`stdio`|`http`) and `listen` mirror the flags of the same name; the nested `logging` object sets `level` (`debug`|`info`|`warn`|`error`), `format` (`text`|`json`), `output_path` (a file; absent → stderr), and `buffer_size` (the capacity of the in-memory ring buffer behind the `remindb://logs` resource; must be > 0, absent → 1000). Absent → today's behavior (stdio, info-level text to stderr, 1000-record buffer). `--verbose` is sugar for `logging.level=debug`. `buffer_size` only sizes the `remindb://logs` mirror — it never affects what reaches stderr/the file. The nested `session_files` object opts into per-session tool-call/error logfiles ([below](#session-logfiles-remindblogs)) — a third sink of the same captured records, alongside the shared stream and the `buffer_size` ring buffer: `enabled` (absent/`false` → off, zero behavior change) and `max_file_size` (a size string, absent → `"10MB"`, must be positive). It only takes effect when `serve` has a source workspace. The nested `resources` object tunes resource-update notification coalescing (see [resources](./resources.md#live-updates--subscriptions)): `debounce` is the global trailing-edge window applied to every subscribable resource (absent → `"500ms"`), and `overrides` maps a short resource name (`graph`, `snapshots`, `tree`, `files`, `temperature`, `logs`) to its own window. Absent overrides fall back to built-in floors of `"1s"` for `logs` and `"2s"` for `temperature` (so the two high-frequency streams never flood); every other resource uses the global default. A negative duration, or an `overrides` key naming a resource that isn't subscribable, fails startup with the offending field named. The nested `sessions` object has one knob, `flush_interval` (absent → `"30s"`), the cadence at which `serve` checkpoints the session ledger ([below](#session-ledger-remindbsessions)); it doubles as the crash-recovery granularity. Must be positive.

**Precedence**, highest first: **explicit CLI flag → `.remindb/config.json` → environment variable → built-in default**. The committed workspace config is authoritative — an env var only fills a key the config leaves *unset*, it never overrides one the config sets. In CI/automation, override a committed value with the explicit flag, not `REMINDB_*`. (`logging` has no flag/env tier beyond `--verbose`, which forces `debug` and wins.)

Reserved for a future release, with its own issue when it lands: `snapshots`.

## Filtering with `.remindb/ignore`

Drop a `.remindb/ignore` at the source root to exclude paths from `compile`, the `serve` rescan loop, the `MemoryCompile` tool, and `bench`. It's a gitignore-style subset — patterns, comments, blank lines.

```
# .remindb/ignore
*.jsonl              # session logs are large and unhelpful
sessions/            # any directory called sessions, at any depth
**/cache/**          # nested cache trees
cache/scratch.md     # exact relative path
!cache/keep.md       # re-include one file (last-match-wins)
/anchored.md         # leading / anchors to the source root
fo?.md               # ? matches exactly one char
file[abc].md         # [abc] matches one char from the set
\!literal.md         # backslash escapes a leading ! or #
```

## Pre-seeding temperatures with `.remindb/temperatures.json`

Drop a `.remindb/temperatures.json` at the source root to set initial temperatures for files at compile time. It's a JSON object; values are floats in `[0, 1]`. Read on `compile`, the `serve` rescan loop, and the `MemoryCompile` tool.

```json
{
  "*": 0.3,
  "README.md": 0.9,
  "src/api/routes.yaml": 0.95,
  "src": {
    "*": 0.6,
    "api": {
      "deprecated.json": 0.1
    },
    "internal": 0.4
  },
  "docs/": 0.4
}
```

Slash-keys and nested objects mix freely — `"src/api/routes.yaml"` and `{"src": {"api": {"routes.yaml": …}}}` mean the same thing. Values can sit on files (`README.md`), directories (`internal`, `docs/`), or a `*` glob that fills in the rest at that level. Resolution walks the path segment by segment and takes the most specific match: a file key beats a sibling `*`, which beats an ancestor's default.

Two keys that resolve to the same leaf with disagreeing values fail at load time with the offending path named. A missing file is silently skipped; everything starts at the engine default of `0.50`. Supported: numbers in `[0, 1]`, nested objects, slash-keys, `*` glob at any level, leading `./` and trailing `/` (both normalized). Anything else — out-of-range numbers, string values, leading `/`, `..` segments, empty segments from `//` — fails the command at startup with the offending key named.

By default, edits here reach only the nodes whose source files *also* changed in the same compile. That's deliberate: agent activity (`MemoryFetch` boosts, the decay tick) shouldn't be wiped silently every time the workspace is recompiled. Pass `remindb compile <dir> --reseed-temperatures` when you mean it — the flag overrides stored temperatures for every node whose source file is keyed here, regardless of whether its content changed. The reseed pass is a temperature update, not a content change, so it does **not** create a new snapshot. It applies to directory compiles only; single-file compiles ignore it, and the `MemoryCompile` MCP tool doesn't expose it (an agent can't use it to overwrite its own temperature signal).

## Session ledger: `.remindb/sessions/`

`serve` durably records, per MCP client that has ever attached to this database, one append-only JSONL file: `<client-name>-<hash>.jsonl`. The prefix is the self-reported client name (sanitized for the filesystem, cosmetic only); `<hash>` is the stable identity key — a content hash of the client's name/title/version/protocol/transport, *not* the spoofable name. Each line is one session checkpoint: connect time, last activity, disconnect time (once closed), and total tool calls. Readers collapse a file by session id, keeping the last line per session, so a process that crashes mid-session loses at most one `flush_interval` of that session's tail and a reconnect (a fresh session id) can never double-count. At `serve` start each file is compacted to one line per session, bounding growth to connections-over-time, not flush ticks.

The ledger never stores payloads, summaries, or node bodies — only connection metadata and counters. It's machine-managed: don't hand-edit it. It surfaces over MCP as `remindb://sessions/history` (all clients) and `remindb://sessions/history/{hash}` (one client) — see [resources](./resources.md). Like every `.remindb/` entry it's excluded from source walks. Flush cadence is `server.sessions.flush_interval` (default `"30s"`).

## Session logfiles: `.remindb/logs/`

Off by default. Set `server.logging.session_files.enabled: true` (and run `serve` with a source workspace) and each connected MCP client session gets its own append-only `.remindb/logs/<session-id>.log`, keyed by the same id `remindb://sessions` reports — the SDK session id, or the synthesized `contentid` fallback for the lone stdio session. Each file is **JSONL**: one record per line (`{time, level, msg, fields}`), serialized from a single shared `sessionlog.Record` definition so the read-back resource deserializes the exact shape `serve` writes (no second hand-rolled parser that could drift). Each file captures that session's `Memory*` tool-call trace (tool name, elapsed, error) and its `Warn`/`Error` records, **distinct from** the shared stderr stream and the `remindb://logs` ring buffer, so an operator can audit one client's activity in isolation. The session trace is captured even when the shared stream sits at `info` (it has its own threshold).

Like the ledger, these files **never** contain payloads, summaries, or node bodies — only the same payload-free fields the shared log carries (see [logging-conventions](../.claude/rules/logging-conventions.md)). Each file is bounded by `server.logging.session_files.max_file_size` (default `"10MB"`, must be positive): when an append would cross the cap the file rotates once to `<session-id>.log.1` (replacing any prior rotation) and a fresh file starts. These logs are read back over MCP as the passive resources `remindb://sessions/logs` (index) and `remindb://sessions/logs/{id}` (one session's structured records, active file only) — see [resources](./resources.md#the-sessionslogs-envelope). Like every `.remindb/` entry the directory is excluded from `compile`, the `serve` rescan loop, `MemoryCompile`, and `bench`, so session logs never become memory nodes. Disabled or unconfigured ⇒ no files written and the logger chain is byte-identical to today.
