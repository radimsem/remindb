# Config model — bootstrap, the `.remindb/` files, and how each change lands

Reference for `remindb-setup`. Load when installing the binary, authoring `.remindb/`, or diagnosing why a config change didn't take effect.

## Bootstrap — get the binary on PATH

Check first: `remindb --version`. If absent, install (confirm before running a remote installer):

```bash
# Linux / macOS — lands at ~/.local/bin/remindb (override: bash -s -- --prefix /usr/local)
curl -fsSL https://raw.githubusercontent.com/radimsem/remindb/main/install.sh | bash
```

```powershell
# Windows (PowerShell 5.1+) — lands at %LOCALAPPDATA%\Programs\remindb\bin
iwr -useb https://raw.githubusercontent.com/radimsem/remindb/main/install.ps1 | iex
```

From source (Go 1.26+): `go build -o ~/.local/bin/remindb ./cmd/remindb` from a clone. Ensure the install dir is on `PATH`. Update later with `remindb update` (re-runs the installer only when a newer release exists).

The binary is half of setup; the agent's MCP plugin (the `mcpServers` entry that spawns `remindb serve`) is the other half — that's installed per host, outside this skill. A server only attaches at agent launch, so a freshly-installed binary needs a restart before its tools appear.

## The `.remindb/` files (at the source root)

All optional; missing → defaults. The dir is skipped during source walks, so it never becomes memory nodes.

| Entry | Controls |
|---|---|
| `.remindb/config.json` | Runtime knobs as feature blocks (`budgets`, `temperature`, `rescan`, `server`, `redaction`). Unknown keys are **rejected at startup** — a typo fails fast, not silently. |
| `.remindb/ignore` | Gitignore-style excludes; subtract from the supported-extension allow-list. Honored by `compile`, the background rescan, and `MemoryCompile`. Can't re-include hardcoded skip dirs (`node_modules`, `vendor`, …) or dotfiles. |
| `.remindb/pinned` | Same grammar as `ignore`; pre-seeds the `pinned` column for matched files **at insert time**, keeping them permanently warm. |
| `.remindb/temperatures.json` | Per-path initial-temperature overrides, applied **at insert time**. Composes with `pinned`: `pinned` = *whether*, `temperatures.json` = *at what temperature*. |

`config.json` blocks worth knowing:

- **`budgets`** — per-tool default token budgets (`search`, `fetch`, `fetch_batch`, `related`).
- **`temperature`** — `enabled` (`false` freezes decay + notifications), `cold_threshold` / `hot_threshold` / `notify_threshold`, `tick_interval`. Shifts search ranking *and* the cold-node stream (see `remind`).
- **`rescan`** — `enabled`, `interval`, `settle` for the background source-rescan loop.
- **`server`** — resource debounce windows (`server.resources`), log buffer size, opt-in per-session logfiles (`server.logging.session_files.enabled`).
- **`redaction`** — payload redaction rules.

Full schema with every key + default: <https://github.com/radimsem/remindb/blob/main/docs/configuration.md>.

## The 3-tier apply model — how each change takes effect

This is the crux: a `.remindb/` change does **not** land uniformly.

| Change | How it applies | Action you must take |
|---|---|---|
| `config.json` → `temperature`, `rescan` | **Live-reload** — re-sourced each tick by the tracker + rescan loop (hashed on change) | none |
| `temperatures.json`, `pinned` | **Insert-time only** — existing nodes untouched | **reseed via CLI** (below) |
| `config.json` → `budgets`, `server`, `redaction` | **Frozen at `serve` startup** — copied into the server once | **restart / new session** |
| `.remindb/ignore` | Affects the next compile / rescan walk | recompile or wait for the next rescan tick |

### Reseed temperatures + pins onto existing nodes

```bash
remindb compile "<source>" --db "<db>" --reseed-temperatures --reseed-pinned
```

- Directory compiles only. `--reseed-temperatures` overwrites stored temperatures on unchanged nodes with `temperatures.json` values; `--reseed-pinned` re-applies `.remindb/pinned` to every matching node — **overwriting manual `MemoryUnpin` choices**. Always warn before running.
- The DB is WAL + `busy_timeout(5000)`, so it's safe to run while `serve` holds the file (≤5s lock-wait buffer); `serve` reads the reseeded rows immediately. Prefer an idle moment to minimize write contention.
- `MemoryCompile` (the tool) deliberately does **not** reseed pins — the agent can't reseed its own pin signal. Hence the CLI.

## Cold-node notifications — the SetLoggingLevel handshake

Cold-node nudges (`level: "warning"`, `logger: "remindb.temperature"`) push only to client sessions that set a logging level. A session that never sends `logging/setLevel` is silent — decay still runs server-side, but you're never told which nodes went cold, so the `MemorySummarize` handoff never fires. Most MCP clients send it automatically; if yours doesn't, it's a client capability gap, not a remindb misconfig — surface it: *"your MCP client must call `logging/setLevel` (e.g. `debug`) to receive remindb cold-node notifications."* Confirmation: notifications never arrive *before* the handshake.
