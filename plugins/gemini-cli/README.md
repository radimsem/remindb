# remindb for Gemini CLI

Drops [remindb](https://github.com/radimsem/remindb) into Gemini CLI as an MCP server. The agent picks up the full `remindb__Memory*` tool suite, backed by a compiled SQLite view of whatever workspace you point it at.

## How it works

The extension ships a `gemini-extension.json` with an inlined `mcpServers` entry. On activation, Gemini CLI spawns `remindb serve` over stdio and merges its tools into the session.

`GEMINI.md` ships alongside the manifest as context for the model when the extension is active.

## Installation

Setup is **config-first and wizard-driven**: the `remindb-setup` skill authors `.remindb/`, compiles, and wires the env for you. Two Gemini-specific facts shape the order below — the extension installs from a **local clone**, and that clone's manifest is also where the **durable env** lives, so the clone comes before the wizard.

### 1. Install the remindb binary

It needs to be on `$PATH`:

```bash
curl -fsSL https://raw.githubusercontent.com/radimsem/remindb/main/install.sh | bash
```

On Windows:

```powershell
iwr -useb https://raw.githubusercontent.com/radimsem/remindb/main/install.ps1 | iex
```

Verify: `remindb --version`.

### 2. Install the companion skills

Pulls the `remindb-setup` wizard plus the `remind` (read) and `memorize` (write) skills:

```bash
npx skills@latest add radimsem/remindb/skills -a gemini-cli
```

Refresh later — independent of `remindb update` — with `npx skills@latest update`.

### 3. Clone the plugin source

`gemini extensions install` has no subdirectory selector and the plugin lives at `plugins/gemini-cli/`, so install from a clone. The clone's `gemini-extension.json` is also where the durable env lands:

```bash
git clone https://github.com/radimsem/remindb.git ~/code/remindb   # or: git -C ~/code/remindb checkout v0.1.0
```

### 4. Run the setup wizard

Gemini CLI surfaces skills via **model activation, not a slash command** — ask in plain language and Gemini activates the skill:

```
run the remindb setup wizard
```

(Manual fallback: open the installed `remindb-setup` `SKILL.md` and follow it by hand.) The wizard detects the host, authors `.remindb/` **before** compiling, runs `remindb compile`, offers to seed adjacent context (the global `~/.gemini/GEMINI.md` and any ancestor `GEMINI.md` above your cwd), and wires the MCP env by writing the two paths into the clone's `gemini-extension.json` (see [Durable env](#durable-env)).

### 5. Install the extension and verify

```bash
gemini extensions install ~/code/remindb/plugins/gemini-cli
```

Update later (local-path installs aren't tracked by `gemini extensions update`): `git -C ~/code/remindb pull`, then `gemini extensions uninstall remindb && gemini extensions install ~/code/remindb/plugins/gemini-cli`. Confirm the server is connected:

```bash
gemini mcp list
```

You should see `remindb` with the full `Memory*` tool suite. Ask Gemini to run the setup wizard again — now that the server is attached, it runs its verify pass (`MemoryStats` + `remindb://doctor`).

## Durable env

`remindb serve` reads `REMINDB_DB` and `REMINDB_SOURCE` as fallbacks for its `--db`/`--source` flags. The clone's `gemini-extension.json` carries an `env` block — the durable home for the two paths, written by the wizard (re-run the step-5 `uninstall` + `install` after any change):

```jsonc
// ~/code/remindb/plugins/gemini-cli/gemini-extension.json
"env": {
    "REMINDB_DB": "/home/you/.cache/remindb/my-project.db",
    "REMINDB_SOURCE": "/home/you/code/my-project"
}
```

Use absolute paths — the manifest does not expand `~` or `$HOME`.

## Tools exposed

The plugin surfaces the full `remindb` `Memory*` tool suite under the `remindb__` namespace. See the [main README](https://github.com/radimsem/remindb#mcp-tools) for the canonical tool list and per-tool token-savings benchmarks.

## License

MIT — same as remindb.
