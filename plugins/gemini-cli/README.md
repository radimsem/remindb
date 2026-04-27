# remindb Extension for Gemini CLI

Mounts the [remindb](https://github.com/radimsem/remindb) MCP server as a workspace memory backend for Gemini CLI agents.

Agents get the full `remindb__Memory*` tool suite — backed by a compiled SQLite view of the workspace.

## How it works

The extension ships a `gemini-extension.json` with an inlined `mcpServers` entry. On activation, Gemini CLI spawns `remindb serve` over stdio and merges its tools into the session.

`GEMINI.md` ships alongside the manifest as context for the model when the extension is active.

## Installation

### 1. Install the remindb binary

The binary must be on `$PATH`:

```bash
curl -fsSL https://raw.githubusercontent.com/radimsem/remindb/main/install.sh | bash
```

On Windows:

```powershell
iwr -useb https://raw.githubusercontent.com/radimsem/remindb/main/install.ps1 | iex
```

Verify:

```bash
remindb --version
```

### 2. Compile a source directory

remindb needs a SQLite file populated from a source tree before the agent can read from it. A natural source for Gemini CLI is its own state folder at `~/.gemini/` — `GEMINI.md` (the global `/memory add` target) and any custom command markdown under `commands/`. Indexing it lets Gemini query its own persistent context through remindb instead of grepping the dot folder.

`~/.gemini/` also holds per-project session chats and shadow-git checkpoints under `tmp/<project_hash>/` and `history/<project_hash>/` — heavyweight transient state that bloats the index. Drop a `.remindb.ignore` at `~/.gemini/` to filter them out.

```bash
mkdir -p ~/.cache/remindb
printf '%s\n' \
    '# Compile only curated context; skip session state and credentials.' \
    '' \
    '# tmp/<project_hash>/{chats,checkpoints}.' \
    'tmp/' \
    '# Shadow-git checkpoint repos (one per project).' \
    'history/' \
    '# Any session jsonl.' \
    '*.jsonl' \
    '# Credentials — never index secrets.' \
    'oauth_creds.json' \
    > ~/.gemini/.remindb.ignore
remindb compile ~/.gemini --db ~/.cache/remindb/gemini.db
```

The same `.remindb.ignore` is honored by `serve`'s background rescan and the `MemoryCompile` MCP tool — set it once, all paths agree. Or point at any other workspace you want agents to see — a docs tree, a notes repo, a project directory.

### 3. Install the extension from GitHub

```bash
gemini extensions install https://github.com/radimsem/remindb --path plugins/gemini-cli
```

Or pin to a ref:

```bash
gemini extensions install https://github.com/radimsem/remindb --path plugins/gemini-cli --ref v0.1.0
```

The CLI clones the repository into `~/.gemini/extensions/remindb/`. Use `gemini extensions update remindb` to sync.

Confirm the server is connected:

```bash
gemini mcp list
```

You should see `remindb` with the full `Memory*` tool suite.

### 4. Export the workspace env vars

`remindb serve` reads `REMINDB_DB` and `REMINDB_SOURCE` as fallbacks for its `--db` and `--source` flags. The extension's `gemini-extension.json` declares both as `${VAR}` passthroughs in its `mcpServers.remindb.env` block, and Gemini CLI expands `$VAR` / `${VAR}` (POSIX, all platforms) at launch — so export them in the shell that launches Gemini CLI:

```bash
export REMINDB_DB=$HOME/.cache/remindb/gemini.db
export REMINDB_SOURCE=$HOME/.gemini
```

Put them in `~/.bashrc` / `~/.zshrc` / fish equivalent to make the mapping permanent, or scope them to a single session to switch workspaces between runs. Undefined variables resolve to empty strings, and `remindb` then falls back to a relative `memory.db` in Gemini's cwd — so set both before launching, not after.

## Tools exposed

The plugin surfaces the full `remindb` `Memory*` tool suite under the `remindb__` namespace. See the [main README](https://github.com/radimsem/remindb#mcp-tools) for the canonical tool list and token-savings benchmarks per tool.

## License

MIT — same as remindb.
