# remindb for Gemini CLI

Drops [remindb](https://github.com/radimsem/remindb) into Gemini CLI as an MCP server. The agent picks up the full `remindb__Memory*` tool suite, backed by a compiled SQLite view of whatever workspace you point it at.

## How it works

The extension ships a `gemini-extension.json` with an inlined `mcpServers` entry. On activation, Gemini CLI spawns `remindb serve` over stdio and merges its tools into the session.

`GEMINI.md` ships alongside the manifest as context for the model when the extension is active.

## Installation

### 1. Install the remindb binary

It needs to be on `$PATH`:

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

### 2. Compile your workspace

remindb needs a SQLite file built from a source tree before the agent can read from it. The source is whatever workspace you want Gemini to remember — a code repo, a docs tree, a notes directory.

```bash
mkdir -p ~/.cache/remindb
remindb compile ~/code/my-project --db ~/.cache/remindb/my-project.db
```

Drop a `.remindb/ignore` at the workspace root if you need to exclude noise (build outputs, vendored deps, generated files). The same file is honored by `serve`'s background rescan and the `MemoryCompile` tool.

### 3. Point remindb at your workspace

The extension reads two env vars to find your workspace: `REMINDB_SOURCE` (the directory to compile and watch) and `REMINDB_DB` (where the compiled SQLite file lives). Export them in the shell **before launching Gemini with the extension installed** — otherwise the first activation falls back to a stray `memory.db` in cwd:

```bash
export REMINDB_DB=$HOME/.cache/remindb/my-project.db
export REMINDB_SOURCE=$HOME/code/my-project
```

Add them to your shell rc (`~/.bashrc`, `~/.zshrc`, fish config) to make it permanent, or set them per-session if you switch between workspaces.

### 4. Install the extension

`gemini extensions install` accepts a GitHub URL or a local path, but its URL form has no subdirectory selector. The plugin lives at `plugins/gemini-cli/` inside the remindb repo, so clone first and install from that subdirectory:

```bash
git clone https://github.com/radimsem/remindb.git ~/code/remindb
gemini extensions install ~/code/remindb/plugins/gemini-cli
```

Pin to a release tag:

```bash
git -C ~/code/remindb checkout v0.1.0
gemini extensions install ~/code/remindb/plugins/gemini-cli
```

Update later with `git pull` and a re-install (local-path installs aren't tracked by `gemini extensions update`):

```bash
git -C ~/code/remindb pull
gemini extensions uninstall remindb
gemini extensions install ~/code/remindb/plugins/gemini-cli
```

Confirm the server is connected:

```bash
gemini mcp list
```

You should see `remindb` with the full `Memory*` tool suite.

#### Seed remaining context

Step 2 only compiled `REMINDB_SOURCE`. Gemini loads `GEMINI.md` from two places outside that path: the global `~/.gemini/GEMINI.md` (where `/memory add` and `save_memory` write) and project-root or ancestor `GEMINI.md` files above your cwd. Anything else outside the workspace won't be in the DB either.

Ask Gemini in your first session to fold them in. Use absolute paths — `MemoryCompile` doesn't expand `~`:

```
remindb__MemoryCompile(path="/home/you/.gemini/GEMINI.md", message="seed: global memory")
remindb__MemoryCompile(path="/home/you/code/parent/GEMINI.md", message="seed: ancestor memory")
```

Re-run whenever a file changes — after `/memory add` or an external edit.

## Skills

The agent-side skills (`remind` for reads, `memoize` for writes) teach Gemini how to call the `Memory*` tools effectively. Install them through [`vercel-labs/skills`](https://github.com/vercel-labs/skills):

```bash
npx skills@latest add radimsem/remindb/skills -a gemini-cli
```

Refresh them later — independent of `remindb update` — with:

```bash
npx skills@latest update
```

## Tools exposed

The plugin surfaces the full `remindb` `Memory*` tool suite under the `remindb__` namespace. See the [main README](https://github.com/radimsem/remindb#mcp-tools) for the canonical tool list and per-tool token-savings benchmarks.

## License

MIT — same as remindb.
