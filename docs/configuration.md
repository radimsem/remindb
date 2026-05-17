# Configuration — the `.remindb/` directory

> Everything that tunes remindb lives in one folder at the source root. All of it is optional; missing means defaults.

[← back to README](../README.md) · related: [CLI](./cli.md) · [temperature](./temperature.md)

remindb keeps its workspace-level state in a `.remindb/` directory at the source root. Three files live there today:

| File | Purpose |
|------|---------|
| `.remindb/config.json` | Runtime configuration (knobs and feature blocks). |
| `.remindb/ignore` | Gitignore-style exclude patterns. |
| `.remindb/temperatures.json` | Per-path initial-temperature overrides. |

All three are optional; missing → defaults. The whole directory is skipped during source walks, so its contents never end up as memory nodes.

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
      "output_path": "/var/log/remindb.log"
    }
  }
}
```

Every field in every block is optional — only the keys you set override the default; the rest keep the engine baseline. Durations are strings (`"10m"`, `"2h"`). Out-of-range values fail startup with the offending field named, rather than silently clamping.

**`temperature`** overrides the decay/boost policy engine-wide. This is distinct from `.remindb/temperatures.json`, which sets *per-path initial* temperatures — this block changes the *policy* (how fast everything decays, where the cold line sits). See [temperature](./temperature.md) for what each knob does. Read by `serve`; `compile` validates the block but doesn't apply it (it has no running tracker).

**`redaction`** configures the secret-scrubber applied on ingest by both `compile` and `serve`. By default every built-in detector is active; `disable_builtin_kinds` mutes the kinds you list (the rest stay on — see the kind list in `internal/redaction/patterns.go`). `custom` *adds* your own `{ "kind", "pattern" }` regexes on top. An unknown kind or an invalid regex fails startup with the offending name reported.

**`compile`** bounds the ingest pipeline for `compile`, the `serve` rescan loop, and the `MemoryCompile` tool — so a client-triggered compile behaves identically to the CLI. Absent → current behavior (unbounded file size, `GOMAXPROCS` parallelism, no deadline). `max_file_size` takes a size string (`"2GB"`, `"500MB"`, or a bare 1024-based byte count) — a file over the limit is **skipped with a `Warn` naming the path**, never an error, so the rest of the tree still compiles. `max_parallelism` caps the per-file worker pool. `wall_clock_timeout` aborts a runaway compile with a clear error; because emission is transactional, a timeout commits **no partial state**.

**`rescan`** tunes the `serve` background rescan loop and is the one block that is **live-reloaded**: at the top of every tick the loop content-hashes `config.json` and, if it changed, re-sources this block — no restart. Absent → defaults (`enabled: true`, `interval: "30s"`, `settle: "500ms"`). `interval` is how often the workspace is walked; `settle` ignores files modified within that window (debounces mid-save writes). `enabled: false` makes each tick a **no-op** (no walk, no compile) while the loop keeps ticking and keeps re-reading config — flip it back to `true` and scanning resumes on the very next tick, no restart, no re-enable trap. An invalid edit (bad JSON, `interval <= 0`, negative `settle`) is logged `Warn` and the last-good settings are kept; the server never crashes on a bad reload. Because this block is re-sourced at runtime, once `serve` is running it is authoritative over the `--rescan-interval` flag and `REMINDB_RESCAN_INTERVAL`, which only seed the interval until the first config read and when no `rescan` block is present.

**`budgets`** sets the default token budget for the four read tools that take one — `MemorySearch`, `MemoryFetch`, `MemoryFetchBatch`, `MemoryRelated`. Resolution is per-tool and local: an explicit positive `budget` on the call always wins; otherwise the configured default; otherwise the built-in. `MemoryRelated`'s built-in is 1000; the other three treat an unset budget as **unlimited** (no trimming). Write tools are unaffected.

**`server`** configures `serve` itself. `transport` (`stdio`|`http`) and `listen` mirror the flags of the same name; the nested `logging` object sets `level` (`debug`|`info`|`warn`|`error`), `format` (`text`|`json`), and `output_path` (a file; absent → stderr). Absent → today's behavior (stdio, info-level text to stderr). `--verbose` is sugar for `logging.level=debug`.

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
