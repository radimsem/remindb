# CLI reference

> Six subcommands, one shared flag (`--db`). Skip `--db` on a directory and remindb derives `./<dirname>.db` for you.

[← back to README](../README.md) · related: [configuration](./configuration.md) · [architecture](./architecture.md)

```
remindb compile <path>   Ingest files or a directory into the database
remindb serve            Start the MCP server (stdio or HTTP)
remindb inspect          Dump DB stats; optionally render the node tree or file list
remindb bench            Measure token savings vs. raw-file baselines
remindb doctor           Run integrity checks; --fix self-heals
remindb update           Reinstall remindb by re-running the install script
```

Everything that lives under `.remindb/` — runtime config, ignore patterns, temperature pre-seeding — has its own page: [configuration](./configuration.md). This page is the subcommands.

## `compile`

One-shot ingestion of a file or directory. Creates a new snapshot and records diffs against the previous one.

```bash
remindb compile ./notes # → ./notes.db
remindb compile ./notes --db memory.db -m "add Q2 notes"
remindb compile ./docs/architecture.md --db project.db
remindb compile ./notes --reseed-temperatures # force .remindb/temperatures.json values onto unchanged nodes
```

| Flag | Purpose |
|------|---------|
| `--db PATH` | Target database. Default: derived from the source directory name, else `memory.db`. |
| `-m, --message` | Snapshot message (defaults to `compile:<path>`). |
| `--reseed-temperatures` | Push `.remindb/temperatures.json` values through to nodes whose source files didn't change on disk. Directory compiles only; no new snapshot. See [configuration → pre-seeding temperatures](./configuration.md#pre-seeding-temperatures-with-remindbtemperaturesjson). |

## `serve`

Starts the MCP server. Default transport is stdio (one server per client process); pass `--transport http` to expose the same `Memory*` suite over streamable HTTP so a CI worker or a hosted agent session can connect to the same memory database. With `--source` set, remindb runs an initial compile (if the DB is empty) and keeps a background rescan loop running. Omit `--source` (and `REMINDB_SOURCE`) to run in DB-only mode — the server opens an existing DB and exposes the MCP surface without filesystem watching.

```bash
remindb serve --db ./notes.db --source ./notes
remindb serve --db ./notes.db --source ./notes --rescan-interval 30s -v
remindb serve --db ./notes.db --source ./notes --transport http
remindb serve --db ./notes.db --source ./notes --transport http --listen 127.0.0.1:7474
remindb serve --db ./notes.db                                                          # DB-only (no source, no rescan)
```

HTTP defaults to `127.0.0.1:7474`. Binding to a non-loopback address (e.g. `--listen 0.0.0.0:7474`) emits a one-time Warn at startup — there is no built-in authentication yet, so put a reverse proxy in front before exposing the server beyond localhost.

| Flag | Env | Purpose |
|------|-----|---------|
| `--db` | `REMINDB_DB` | Database file. |
| `--source` | `REMINDB_SOURCE` | Source directory to watch and incrementally recompile. Omit for DB-only mode. |
| `--rescan-interval` | `REMINDB_RESCAN_INTERVAL` | e.g. `30s`, `5m`. `0` keeps the tracker's default. Requires `--source`. |
| `--transport` | `REMINDB_TRANSPORT` | `stdio` (default) or `http`. Also `server.transport` — see [configuration → precedence](./configuration.md#runtime-config-remindbconfigjson). |
| `--listen` | `REMINDB_LISTEN` | Listen address for HTTP transport. Default `127.0.0.1:7474`; requires `--transport=http`. Also `server.listen`. |
| `-v, --verbose` | — | Force debug-level logs (default info). Sugar for `server.logging.level=debug`; wins over config. |

`serve` background-checks GitHub releases on startup and emits an `info` log when a newer tag is available, with `hint=remindb update` — the prompt to upgrade comes from the server, the upgrade itself is one command.

## `inspect`

Read-only snapshot of what's in a database. Without `--tree` or `--files` it prints stats; `--tree` renders the node hierarchy (temperatures colour-coded blue cold → red hot); `--files` renders the compiled source files grouped by compile root.

```bash
remindb inspect --db ./notes.db
remindb inspect --db ./notes.db --tree --depth 6
remindb inspect --db ./notes.db --files
```

| Flag | Purpose |
|------|---------|
| `--tree` | Render the node tree. |
| `--files` | Render compiled source files grouped by compile root. |
| `--depth N` | Maximum depth when rendering. Default: `10`. Requires `--tree`. |

`NO_COLOR=1` disables the ANSI palette. (`MemoryStats` returns the same data over MCP — same numbers, no terminal.)

## `bench`

Runs the scenario suite — tree · search · fetch · delta — against one database and prints token savings compared to a naive *list + read + grep* baseline.

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

## `doctor`

Runs integrity checks against the database — the kind of structural invariants a corrupted compile or an interrupted write could break. By default it only reports. Pass `--fix` and it attempts to repair the failed checks, **taking a timestamped backup first** so a botched repair is never destructive.

```bash
remindb doctor --db ./notes.db          # report only
remindb doctor --db ./notes.db --fix    # self-heal (backs up first)
remindb doctor --db ./notes.db --json   # machine-readable, for CI
```

| Flag | Purpose |
|------|---------|
| `--fix` | Attempt to repair failed checks. Takes a timestamped backup before touching anything. |
| `--json` | Emit machine-readable JSON instead of the human report. |

Reach for `doctor` when search results look wrong, a compile died mid-run, or you want a CI gate that a synced `.db` is structurally sound before it ships.

## `update`

Reinstalls remindb in place by re-running the official install script — `install.sh` piped to `bash` on Linux / macOS, `install.ps1` piped to PowerShell on Windows. It reads the installed version, compares against the latest GitHub release, and only re-runs when they differ. `dev`-builds (from `go build` / `go install`) always proceed; there's no published version to compare against.

```bash
remindb update
remindb update --force   # reinstall regardless of version
```
