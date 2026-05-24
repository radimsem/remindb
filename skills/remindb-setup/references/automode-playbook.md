# Automode playbook — infer the whole setup yourself

Reference for `remindb-setup` under `automode` (chosen in the interview or passed as `/remindb-setup automode`). The goal: a config that fits this specific workspace, picked for **best result + consistency**, written without per-option approval. Knob semantics → `config-model.md`; templates to anchor on → `config-examples.md`; per-host install + env + invocation → `host-wiring.md`.

## Order of operations — first-time, config-first (Pass 1)

automode infers every parameter, but the **ordering is fixed** by the config-first invariant: author `.remindb/` before the compile so `ignore`/`pinned`/`temperatures.json` apply at insert time. No reseed on a first run — reseeding is the Pass 2 / reconfigure path only.

1. **Binary** — `remindb --version`; if missing, install it (still confirm the remote installer, the one trust action automode keeps).
2. **Identify your host** from the agent runtime you're in — your system prompt and the tools/skills exposed this session pin it down (e.g. `/plugin install` = Claude Code; `/skills` = Codex; `openclaw mcp` = OpenClaw; `activate_skill` = Gemini CLI; plain-language skill activation = OpenCode). Don't probe filesystems. If runtime context is genuinely ambiguous, pick the most likely and **record the assumption** in the summary. Look up its `host-wiring.md` row.
3. **Resolve source + db** — confirm the source root (pre-attach, so infer it from the host's state dir, e.g. `~/.claude/projects`); derive a db path (e.g. `~/.cache/remindb/<host>.db`). Unresolvable → stop and report, don't guess.
4. **Classify the workspace** (below) → pick the closest `config-examples.md` template as a baseline; walk the tree once; adjust from what you actually see.
5. **Write `.remindb/` files** (before compiling). Print a summary: every file written + the one-line reason for each non-obvious choice.
6. **Compile** — `remindb compile <source> --db <db>` (no `--reseed`).
7. **Seed adjacent context** — `remindb compile <abs-file> --db <db>` per out-of-source file the host loads (`CLAUDE.md`/`AGENTS.md`/`GEMINI.md`/`README.md`).
8. **Install/enable the MCP plugin** — per the host's `Plugin install` column in `host-wiring.md`. For Claude Code the wizard prompts the user (`/plugin install` isn't Bash-callable); for Codex / OpenClaw / Gemini the wizard can run the install command itself after confirming. **Confirm before invoking install.** Wherever the loaded config is post-install (Claude Code cache, OpenClaw MCP store), install must complete before step 9.
9. **Wire the env** — locate the loaded config the running server reads (`host-wiring.md` names it per host) and apply the **scoped edit** (multi-brain safe). Shell-export snippet only where scoped isn't viable (unknown host, or user opt-in for Claude Code marketplace per `host-wiring.md` §Env-wiring principle). **Confirm before writing host config** — the second trust action automode keeps.
10. **Report** — print what was written + "reload the host (or restart your shell if you used the export fallback), then re-run `/remindb-setup` to verify (Pass 2 confirms `MemoryStats` `db_path` matches the compiled DB)".

## `only-config` — bridge-host truncation

When invoked as `/remindb-setup automode only-config`, run **through step 5 (Write `.remindb/` files)** and stop. Steps 6–10 (compile, adjacent-seed, plugin install, env wiring, report) are explicitly skipped — they belong to the calling host's own pipeline (e.g. Hermes Agent's `hermes memory setup`). Same truncation under interactive `only-config`; the mode is what's truncated, not the trust level.

Parse the prompt body for override labels (case-insensitive, leading-whitespace-tolerant). Value sits on the same line after the colon (no multi-line values); `~` / `$HOME` are expanded:

- **`Source root:`** — mandatory. **Fail loud** if absent — interactive or automode, don't infer from host state dirs the way step 3 normally does. Under `only-config`, the calling host (a plugin README, an external wizard) dictates the source explicitly; silently guessing is the wrong failure mode for a bridge.
- **`Also adjacent-seed:`** — optional, newline-separated absolute paths (one per line). The list terminates at the first blank line, the next recognized label, or end of prompt. Record the paths in the closing summary as files the calling host should later compile via `remindb compile <abs-file> --db <db>`. **Do not** run compile yourself — under `only-config` the wizard never touches the db.

Free-form prose elsewhere in the prompt body is ignored by the parser; it may inform the closing summary's wording but carries no mechanical effect.

Close with a one-line report: `.remindb/` files written + queued adjacent-seed paths + the calling-host pipeline name (so the user knows what owns the remaining steps).

## Classify the workspace

Sample the top two-three directory levels and file extensions:

- Mostly code (`.go/.ts/.py/.rs/...`) + a `docs/` or `README` → **code repo**.
- Mostly `.md` in a flat-ish tree, an `.obsidian/`, MOC/index files → **docs/notes vault**.
- Path is under `~/.claude/projects` (or has per-project `memory/` dirs + `*.jsonl`) → **agent memory**.
- A bit of everything → **mixed**.

## Inference rules

**`.remindb/ignore`** — exclude what adds bytes without signal:
- build output (`dist/`, `build/`, `target/`, `out/`), caches (`.cache/`, `coverage/`), lockfiles (`*.lock`), minified (`*.min.*`), large generated/golden fixtures.
- `node_modules`, `vendor`, `venv`, dotfiles are already skipped — don't restate.
- agent-memory case: `*.jsonl`, `subagents/`, `tool-results/`.
- When unsure, **don't ignore** — a false ignore silently starves memory; a little noise is recoverable.

**`.remindb/pinned`** — keep stable, high-value reference warm. Pin sparingly (over-pinning kills the cold-set signal):
- canonical entry points: `README.md`, `**/CONTEXT.md`, `**/index.md` / `**/_index.md`, MOCs.
- durable specs/decisions: `docs/adr/**`, `**/*.spec.md`, architecture docs, `SECURITY.md`.
- **Never** pin a glob matching a large fraction of the tree, generated files, or volatile working notes.

**`.remindb/temperatures.json`** — initial warmth by expected read-frequency:
- `0.7–0.9` high-traffic reference (top READMEs, architecture).
- `0.4–0.6` regular docs.
- `0.1–0.3` archives, changelogs, rarely-touched config.
- Only set entries that deviate from the default; don't enumerate the whole tree.

**`.remindb/config.json`** — defaults that fit usage:
- `budgets`: `search 1000–1500`, `fetch 1500–2000`, `fetch_batch 4000`. Larger/uniform corpora → higher.
- `temperature`: keep defaults (`cold 0.1`, `hot 0.5`, `tick 5m`) unless the corpus is large + slow-moving → longer `tick` (`10m`) so decay isn't aggressive.
- `rescan`: `enabled true`; `interval 30s` active repos, `60s` calmer vaults.
- `server` / `redaction`: leave at defaults unless the user stated a need — these are restart-only, so changing them forces a relaunch.

## Consistency checks before writing

- Every `pinned` glob also survives `ignore` (don't pin a file you excluded).
- `hot_threshold > cold_threshold` (required).
- `temperatures.json` paths are relative to the source root and actually match files.
- Total pinned set is a small minority of nodes.

Report what you did and why; the user can re-run `/remindb-setup` interactively to adjust any single choice.
