---
name: remindb-setup
description: 'Config-first setup wizard for a remindb MCP server — run it as `/remindb-setup` (interactive), `/remindb-setup automode` (hands-off), or `/remindb-setup only-config` (bridge hosts: author `.remindb/` only, no env wiring). Two passes. First-time (no server attached yet): detect the host, author the `.remindb/` config (ignore/pinned/temperatures/config.json) BEFORE compiling, compile the source, seed adjacent context, install the MCP plugin, then wire the MCP env. Verify (server attached): MemoryStats + `remindb://doctor`, reconfigure, and reseed onto existing nodes. Use on first-time workspace memory setup, when remindb tools are missing/misconfigured, on "no results"/wrong-workspace symptoms, to reconfigure an existing brain, or for bridge hosts that own env wiring externally (e.g. Hermes Agent).'
user-invocable: true
---

# remindb-setup — the setup wizard

You are the wizard. This skill is the script you follow to set up or reconfigure a remindb workspace from inside a live session — there is no separate UI. Drive it with Bash + the `Memory*` tools + file writes.

## Standing instruction — verify CLI flags via `--help` first

Before invoking any `remindb` subcommand, confirm its flags with `remindb <subcommand> --help` (`remindb compile --help`, `remindb inspect --help`, …). Don't trust remembered syntax — silently-wrong positional args are the wizard's most common footgun (e.g. `remindb inspect <path>` is ignored; the flag is `--db <path>`, and the default `memory.db` quietly opens an empty file in CWD).

## Two halves, two passes — detect which one you're in

remindb ships as two independently-installed halves: the **skill** (this, via `npx skills add`) and the **MCP plugin** (per host). The skill installs first and can run *before* the plugin is attached — that's what makes config-first ordering possible. Detect the pass by whether the server is attached this session:

- **`Memory*` tools absent** from your available tool set → **Pass 1: first-time config-first setup** (§Pass 1). The default first run.
- **`Memory*` tools present** → **Pass 2: verify / reconfigure** (§Pass 2).

## Mode

- **`/remindb-setup`** — interactive: propose each choice for approval before writing.
- **`/remindb-setup automode`** — infer every parameter, write directly, then report. The trust actions it still confirms: running the remote installer (step 1), invoking plugin install (step 7), and writing host MCP config (step 8). Inference rules → `references/automode-playbook.md`.
- **`/remindb-setup only-config`** — bridge-host short-circuit: run Pass 1 **steps 1–4 only** (binary → host → source → `.remindb/`), then stop with a one-line summary. Parses `Source root:` (mandatory; absent → fail loud, interactive or automode — don't guess) and `Also adjacent-seed:` (optional, newline-separated absolute paths queued for the calling host to compile later) from the prompt body. Composable with `automode`. Use when the calling host owns compile + plugin install + env wiring via its own pipeline — see the bridge-host note in `references/host-wiring.md`; parsing rules in `references/automode-playbook.md`.

Not every host surfaces this as a literal `/remindb-setup` command — invoke it per the host's row in `references/host-wiring.md`. The wizard runs the same once invoked.

## Pass 1 — first-time config-first setup (server not attached)

Order matters: author `.remindb/` **before** compiling, so `ignore`/`pinned`/`temperatures.json` apply at insert time — no `--reseed` retrofit. **`only-config`** truncates this list to steps 1–4 and stops with a summary (see Mode above).

1. **Binary.** `remindb --version` (Bash). Missing → install it; **confirm before the remote installer**, even in automode. One-liners + PATH detail → `references/config-model.md` §Bootstrap.
2. **Identify your host.** You already know which agent runtime you're in — check your system prompt and the tools/skills exposed this session (e.g. `/plugin install` = Claude Code; `/skills` picker = Codex; `openclaw mcp` = OpenClaw; `activate_skill` = Gemini CLI; OpenCode activates skills via plain language). For host-specific config paths, lean on your runtime's own knowledge first — system prompt, loaded host-specific skills, runtime docs — rather than probing the filesystem. If runtime context is still ambiguous, ask the user (automode: pick the most likely and record the assumption). Then look up the matching row in `references/host-wiring.md`.
3. **Locate the source root.** Confirm the dir to compile — it's also where `.remindb/` lives. You're pre-attach, so there's no `REMINDB_SOURCE` to read yet; settle it with the user (automode: infer from the host's state dir, e.g. `~/.claude/projects`).
4. **Author `.remindb/`.** Write `ignore` / `pinned` / `temperatures.json` / `config.json` at `<source>/.remindb/`. Semantics + how each change lands → `references/config-model.md`; copy-pasteable templates per workspace type → `references/config-examples.md`. Interactive: show the plan first. automode: write, then summarize each non-obvious choice.
5. **Compile.** `remindb compile <source> --db <db>` — insert-time apply; **no `--reseed`** (the config is already in place).
6. **Seed adjacent context (optional).** Files outside the source root — `CLAUDE.md` / `AGENTS.md` / `GEMINI.md` / `README.md` — won't be in the DB. Offer to fold them in via the **CLI** (you're pre-attach, so `MemoryCompile` the tool isn't available yet), one per file, absolute paths:
   ```bash
   remindb compile /abs/path/CLAUDE.md --db <db>
   ```
7. **Install/enable the MCP plugin** — per the host's `Plugin install` column in `references/host-wiring.md` (e.g. `/plugin install remindb@remindb` for Claude Code, `codex plugin marketplace add radimsem/remindb` for Codex). Where the install command is Bash-callable the wizard can run it; for Claude Code the wizard prompts the user (`/plugin install` isn't a shell command). Confirm before invoking install. Plugin must be installed before step 8 wherever the loaded config is post-install (Claude Code cache, OpenClaw MCP store).
8. **Wire the MCP env.** Plugin installed → now set `REMINDB_DB` / `REMINDB_SOURCE` in the loaded config file. **Locate the file first** — `references/host-wiring.md` names it per host; for unknown hosts probe `grep -rl REMINDB <host-config-dir>`. Don't trust memory. **Scoped edit preferred** (multi-brain safe); shell-export fallback only when scoped isn't viable (unknown host, or user opt-in for Claude Code marketplace) — see §Env-wiring principle in `host-wiring.md`. Confirm before writing host config, even in automode. Then reload the host.

Close Pass 1 by telling the user that once the host has reloaded, the server attaches with `REMINDB_DB`/`REMINDB_SOURCE` resolved — re-run the wizard (Pass 2) to confirm `MemoryStats` `db_path` matches the compiled DB.

## Pass 2 — verify / reconfigure (server attached)

- **Verify.** `MemoryStats()` → confirm the reported database path is the one you compiled and node count is non-zero; then read `remindb://doctor` for FTS5 / stale-root checks (for a machine-checkable path, `remindb://overview` exposes a `db_path` field). Cold-node notifications need a one-shot `SetLoggingLevel` (`logging/setLevel`) from the client — most send it automatically; if yours doesn't, the server is fine but the client must opt in (detail → `references/config-model.md`).
- **Diagnostic — `db_path` mismatch / 0 nodes.** `MemoryStats` reports a `db_path` that's **not** the compiled DB — especially a literal `$PWD/${REMINDB_DB}` with 0 nodes (the Claude Code shape: cache `${VAR}` passthroughs evaluated against a shell where the var is unset) — means the env didn't resolve where the server launched: either the env var is unset, **or you edited the wrong config file**. Fix: set `REMINDB_DB`/`REMINDB_SOURCE` in the actually-loaded config (re-check `references/host-wiring.md` for which file that is on your host) or in the launch shell, then reload.
- **Reconfigure an existing brain.** Re-author any `.remindb/` file, then apply per the 3-tier model in `references/config-model.md`: live-reload (`temperature`/`rescan`) → nothing; insert-time (`temperatures.json`/`pinned`) → reseed (below); frozen (`budgets`/`server`/`redaction`) → restart.
- **Reseed onto existing nodes (reconfigure-only).** Authoring `temperatures.json`/`pinned` after the fact does nothing to already-compiled nodes. To apply them to an existing DB:
  ```bash
  remindb compile <source> --db <db> --reseed-temperatures --reseed-pinned
  ```
  **Warn:** `--reseed-pinned` re-applies `.remindb/pinned` to every match, **overwriting manual `MemoryUnpin` choices**. WAL + `busy_timeout(5s)` makes it safe while `serve` holds the file. `MemoryCompile` (the tool) never reseeds pins — hence the CLI.

Done? You're ready: `remember` (plain-language front door), `remind` (read tools), `memoize` (write tools).
