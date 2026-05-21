---
name: remindb-setup
description: Setup wizard for a remindb MCP server — run it as `/remindb-setup` (interactive) or `/remindb-setup automode` (hands-off). Confirms the binary + attached server, locates the workspace source, authors the `.remindb/` config (ignore/pinned/temperatures/config.json), offers to reseed temperatures/pins onto existing nodes, and tells you when a restart is needed. Use on first-time workspace memory setup, when remindb tools are missing/misconfigured, on "no results"/wrong-workspace symptoms, or to reconfigure an existing brain.
---

# remindb-setup — the setup wizard

You are the wizard. This skill is the script you follow to set up or reconfigure a remindb workspace from inside a live session — there is no separate UI. Drive it with Bash + the `Memory*` tools + file writes.

**Assumed entry state:** the remindb binary and the agent's MCP plugin are installed, so the server is attached this session and a `.db` already exists. Then `/remindb-setup` is the *reconfigure-an-existing-brain* flow. If the binary is missing, step 1 installs it as a fallback.

## Mode: interactive vs automode

- **`/remindb-setup`** — interactive. Step 3 first asks the user: *walk me through each `.remindb/` choice* **or** *automode (figure it out for me)*. If they pick automode here, behave as below.
- **`/remindb-setup automode`** — skip that question. You infer **every** parameter for the best, consistent result, write directly, and report — no per-option approval. The one thing automode still confirms is the remote-installer step (1), because piping a remote script to a shell is a trust action.

Automode inference rules live in `references/automode-playbook.md`.

## 1. Sanity + fallback install

```
remindb --version          # Bash
remindb__MemoryStats()     # confirms the server is attached; read db_path + source
```

- Binary present + tools attached → continue.
- **Binary missing** (edge case — plugin installed but no binary) → install it, then note the server only attaches on the next launch. **Confirm before running the remote installer**, even in automode. Install one-liners + version/PATH detail → `references/config-model.md` §Bootstrap.

## 2. Locate the workspace source

The `.remindb/` config dir lives at the **source root** — the dir `serve` was pointed at via `REMINDB_SOURCE`. Find it:

```
echo "$REMINDB_SOURCE"                 # Bash; the canonical source root
remindb__MemoryStats()                 # db_path + compile root, if the env var is unset
```

No source root resolvable → you can't author workspace config. Say so, tell the user to point `serve` at a source (`REMINDB_SOURCE` / `--source`) and recompile, and stop.

## 3. The `.remindb/` config wizard

Author the workspace config at `<source>/.remindb/`. All files optional; missing = defaults. What each does + how each change lands → `references/config-model.md`. Copy-pasteable starting points per workspace type → `references/config-examples.md`.

- **Interactive** — walk the source tree, then propose, for approval: `.remindb/ignore` (build/deps/generated noise), `.remindb/pinned` (stable reference files to keep warm), `.remindb/temperatures.json` (per-path initial warmth), `.remindb/config.json` (budgets, temperature, rescan). Show the plan before writing anything.
- **automode** — infer all of the above from a workspace walk per `references/automode-playbook.md`, write the files directly, then print a summary of what you wrote and why.

## 4. Offer to reseed temperatures + pins onto existing nodes

`temperatures.json` and `pinned` seed nodes **only at insert time** — authoring them now does nothing to the already-compiled DB. To apply them to existing nodes, reseed via the CLI:

```bash
remindb compile "$REMINDB_SOURCE" --db "<db_path>" --reseed-temperatures --reseed-pinned
```

- Interactive → **ask** before running it. automode → run it, surfacing the same warning.
- **Always warn:** `--reseed-pinned` re-applies `.remindb/pinned` to every matching node, **overwriting any manual `MemoryUnpin` choices**. The DB is WAL + `busy_timeout(5s)`, so this is safe to run while `serve` holds the file; `serve` sees the reseeded rows immediately.

Why the CLI and not the tools: `MemoryCompile` deliberately does **not** reseed pins (the agent can't reseed its own pin signal). The reseed flags are directory-compile-only.

## 5. Restart suggestion (advise, don't act)

Only the `temperature` and `rescan` blocks of `config.json` live-reload (re-sourced each tick). `budgets`, `server`, and `redaction` are read **once at `serve` startup** — they're frozen for this session. If you changed any of those (or installed the binary in step 1):

> Restart the agent / start a new session to apply the `config.json` changes (`budgets`/`server`/`redaction`) and to attach the freshly-installed server.

Output this as a suggestion. Don't restart anything yourself — you can't relaunch your own server.

## After setup — verify

Re-running `/remindb-setup` once the server is freshly attached is the verify pass: `MemoryStats` (right `db_path`, non-zero nodes), then `remindb://doctor` for FTS5/stale-root checks. Cold-node notifications require a one-shot `SetLoggingLevel` (`logging/setLevel`) from the client — most MCP clients send it automatically; if yours doesn't, the server is fine but the client must opt in. Details → `references/config-model.md`.

Done? You're ready: `remember` (plain-language front door), `remind` (read tools), `memoize` (write tools).
