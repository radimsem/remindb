# remindb Extension for Gemini CLI

Mounts the [remindb](https://github.com/radimsem/remindb) MCP server as a workspace memory backend for Gemini CLI agents.

Agents get eight `remindb__*` tools — `MemoryFetch`, `MemorySearch`, `MemoryWrite`, `MemoryCompile`, `MemoryDelta`, `MemorySummarize`, `MemoryHistory`, `MemoryTree` — backed by a compiled SQLite view of the workspace.

## How it works

The extension ships a `gemini-extension.json` with an inlined `mcpServers` entry. On activation, Gemini CLI spawns `remindb serve` over stdio and merges its tools into the session.

`GEMINI.md` ships alongside the manifest as context for the model when the extension is active.

## Installation

### 1. Install the remindb binary

The binary must be on `$PATH`:

```bash
curl -fsSL https://raw.githubusercontent.com/radimsem/remindb/master/install.sh | sh
```

On Windows:

```powershell
iwr -useb https://raw.githubusercontent.com/radimsem/remindb/master/install.ps1 | iex
```

Verify:

```bash
remindb --help
```

### 2. Compile a source directory

remindb needs a SQLite file populated from a source tree before the agent can read from it. A natural source for Gemini CLI is its own state folder at `~/.gemini/` — `GEMINI.md` context files, per-project shadow-git snapshots under `~/.gemini/history/<project_hash>`, and conversation checkpoints under `~/.gemini/tmp/<project_hash>/checkpoints`. Indexing it lets Gemini query its own persistent context through remindb instead of grepping the dot folder:

```bash
mkdir -p ~/.cache/remindb
remindb compile ~/.gemini --db ~/.cache/remindb/gemini.db
```

Or point at any other workspace you want agents to see — a docs tree, a notes repo, a project directory.

### 3. Install the extension from GitHub

```bash
gemini extensions install https://github.com/radimsem/remindb --path gemini-cli
```

Or pin to a ref:

```bash
gemini extensions install https://github.com/radimsem/remindb --path gemini-cli --ref v0.1.0
```

The CLI clones the repository into `~/.gemini/extensions/remindb/`. Use `gemini extensions update remindb` to sync.

Confirm the server is connected:

```bash
gemini mcp list
```

You should see `remindb` with eight `MemoryXxx` tools.

### 4. Point remindb at your workspace via `~/.gemini/extensions/remindb/.env`

`remindb serve` reads `REMINDB_DB` and `REMINDB_SOURCE` as fallbacks for its `--db` and `--source` flags. Gemini CLI auto-sources a `.env` file from the extension's own folder when it spawns MCP subprocesses, so drop the workspace paths there:

```bash
cat <<EOF > ~/.gemini/extensions/remindb/.env
REMINDB_DB=$HOME/.cache/remindb/gemini.db
REMINDB_SOURCE=$HOME/.gemini
EOF
```

Swap the two paths for a different workspace (e.g., `~/notes` + `~/.cache/remindb/notes.db`) whenever you want Gemini to read a different tree.

This scopes the env vars to Gemini's spawned subprocess, survives `gemini extensions update remindb` (update preserves `.env`), and lets you switch sources by editing one file and restarting Gemini CLI.

Prefer a shell-inherited env instead? Export the same pair in `~/.bashrc` / `~/.zshrc` / fish equivalent and restart Gemini CLI from that shell.

## Tools exposed

| Tool | Purpose |
|------|---------|
| `remindb__MemoryTree` | Structural overview of the compiled workspace |
| `remindb__MemorySearch` | Full-text search within a token budget |
| `remindb__MemoryFetch` | Context around an anchor node within a token budget |
| `remindb__MemoryWrite` | Write or update content at an anchor, creating a snapshot |
| `remindb__MemoryDelta` | Changes since a given snapshot |
| `remindb__MemorySummarize` | Replace a node's content with a provided summary |
| `remindb__MemoryHistory` | Browse version history for a node |
| `remindb__MemoryCompile` | Re-compile source files or a directory |

See the [remindb README](https://github.com/radimsem/remindb#readme) for token-savings benchmarks per tool.

## License

MIT — same as remindb.
