---
name: remindb-setup
description: Config-first setup wizard for a remindb MCP server — run it as `/remindb-setup` (interactive) or `/remindb-setup automode` (hands-off). Two passes. First-time (no server attached yet): detect the host, author the `.remindb/` config (ignore/pinned/temperatures/config.json) BEFORE compiling, compile the source, seed adjacent context, then wire the MCP env. Verify (server attached): MemoryStats + `remindb://doctor`, reconfigure, and reseed onto existing nodes. Use on first-time workspace memory setup, when remindb tools are missing/misconfigured, on "no results"/wrong-workspace symptoms, or to reconfigure an existing brain.
user-invocable: true
---

# remindb-setup — the setup wizard

You are the wizard. This skill is the script you follow to set up or reconfigure a remindb workspace from inside a live session — there is no separate UI. Drive it with Bash + the `Memory*` tools + file writes.

## Two halves, two passes — detect which one you're in

remindb ships as two independently-installed halves: the **skill** (this, via `npx skills add`) and the **MCP plugin** (per host). The skill installs first and can run *before* the plugin is attached — that's what makes config-first ordering possible. Detect the pass by whether the server is attached this session:

- **`Memory*` tools absent** from your available tool set → **Pass 1: first-time config-first setup** (§Pass 1). The default first run.
- **`Memory*` tools present** → **Pass 2: verify / reconfigure** (§Pass 2).

## Mode: interactive vs automode

- **`/remindb-setup`** — interactive: propose each choice for approval before writing.
- **`/remindb-setup automode`** — infer every parameter, write directly, then report. The two trust actions it still confirms: running the remote installer (step 1) and writing host MCP config (step 7). Inference rules → `references/automode-playbook.md`.

Not every host surfaces this as a literal `/remindb-setup` command — invoke it per the host's row in `references/host-wiring.md`. The wizard runs the same once invoked.

## Pass 1 — first-time config-first setup (server not attached)

Order matters: author `.remindb/` **before** compiling, so `ignore`/`pinned`/`temperatures.json` apply at insert time — no `--reseed` retrofit.

1. **Binary.** `remindb --version` (Bash). Missing → install it; **confirm before the remote installer**, even in automode. One-liners + PATH detail → `references/config-model.md` §Bootstrap.
2. **Detect host.** Probe `~/.claude` · `~/.codex` · `~/.gemini` · `~/.openclaw` · `~/.config/opencode`; confirm the match (automode: pick the most likely, record the assumption). Per-host install + durable env + invocation → `references/host-wiring.md`.
3. **Locate the source root.** Confirm the dir to compile — it's also where `.remindb/` lives. You're pre-attach, so there's no `REMINDB_SOURCE` to read yet; settle it with the user (automode: infer from the host's state dir, e.g. `~/.claude/projects`).
4. **Author `.remindb/`.** Write `ignore` / `pinned` / `temperatures.json` / `config.json` at `<source>/.remindb/`. Semantics + how each change lands → `references/config-model.md`; copy-pasteable templates per workspace type → `references/config-examples.md`. Interactive: show the plan first. automode: write, then summarize each non-obvious choice.
5. **Compile.** `remindb compile <source> --db <db>` — insert-time apply; **no `--reseed`** (the config is already in place).
6. **Seed adjacent context (optional).** Files outside the source root — `CLAUDE.md` / `AGENTS.md` / `GEMINI.md` / `README.md` — won't be in the DB. Offer to fold them in via the **CLI** (you're pre-attach, so `MemoryCompile` the tool isn't available yet), one per file, absolute paths:
   ```bash
   remindb compile /abs/path/CLAUDE.md --db <db>
   ```
7. **Wire the MCP env — collaboratively.** The server reads `REMINDB_DB` + `REMINDB_SOURCE`. **Prefer to apply the wiring yourself** using the host's durable mechanism from `references/host-wiring.md` — write the `~/.codex/config.toml` block, merge `opencode.json`, run `openclaw mcp set`, edit the local-clone manifest. Emit a copy-paste snippet *only* where you can't durably write: Claude Code's overwrite-on-update marketplace cache (→ shell export), or an unknown host. Confirm before writing host config, even in automode.

Close Pass 1 by telling the user to **install/enable the MCP plugin and restart** (host steps in `references/host-wiring.md`). The server attaches on the next launch — re-run the wizard to verify (Pass 2).

## Pass 2 — verify / reconfigure (server attached)

- **Verify.** `MemoryStats()` → confirm the reported database path is the one you compiled and node count is non-zero; then read `remindb://doctor` for FTS5 / stale-root checks (for a machine-checkable path, `remindb://overview` exposes a `db_path` field). Cold-node notifications need a one-shot `SetLoggingLevel` (`logging/setLevel`) from the client — most send it automatically; if yours doesn't, the server is fine but the client must opt in (detail → `references/config-model.md`).
- **Reconfigure an existing brain.** Re-author any `.remindb/` file, then apply per the 3-tier model in `references/config-model.md`: live-reload (`temperature`/`rescan`) → nothing; insert-time (`temperatures.json`/`pinned`) → reseed (below); frozen (`budgets`/`server`/`redaction`) → restart.
- **Reseed onto existing nodes (reconfigure-only).** Authoring `temperatures.json`/`pinned` after the fact does nothing to already-compiled nodes. To apply them to an existing DB:
  ```bash
  remindb compile <source> --db <db> --reseed-temperatures --reseed-pinned
  ```
  **Warn:** `--reseed-pinned` re-applies `.remindb/pinned` to every match, **overwriting manual `MemoryUnpin` choices**. WAL + `busy_timeout(5s)` makes it safe while `serve` holds the file. `MemoryCompile` (the tool) never reseeds pins — hence the CLI.

Done? You're ready: `remember` (plain-language front door), `remind` (read tools), `memoize` (write tools).
