# Automode playbook — infer the whole setup yourself

Reference for `remindb-setup` under `automode` (chosen in the interview or passed as `/remindb-setup automode`). The goal: a config that fits this specific workspace, picked for **best result + consistency**, written without per-option approval. Knob semantics → `config-model.md`; templates to anchor on → `config-examples.md`; per-host install + env + invocation → `host-wiring.md`.

## Order of operations — first-time, config-first (Pass 1)

automode infers every parameter, but the **ordering is fixed** by the config-first invariant: author `.remindb/` before the compile so `ignore`/`pinned`/`temperatures.json` apply at insert time. No reseed on a first run — reseeding is the Pass 2 / reconfigure path only.

1. **Binary** — `remindb --version`; if missing, install it (still confirm the remote installer, the one trust action automode keeps).
2. **Detect host** — probe `~/.claude` · `~/.codex` · `~/.gemini` · `~/.openclaw` · `~/.config/opencode`; if more than one matches, pick the most likely and **record the assumption** in the summary. Look up its `host-wiring.md` row.
3. **Resolve source + db** — confirm the source root (pre-attach, so infer it from the host's state dir, e.g. `~/.claude/projects`); derive a db path (e.g. `~/.cache/remindb/<host>.db`). Unresolvable → stop and report, don't guess.
4. **Classify the workspace** (below) → pick the closest `config-examples.md` template as a baseline; walk the tree once; adjust from what you actually see.
5. **Write `.remindb/` files** (before compiling). Print a summary: every file written + the one-line reason for each non-obvious choice.
6. **Compile** — `remindb compile <source> --db <db>` (no `--reseed`).
7. **Seed adjacent context** — `remindb compile <abs-file> --db <db>` per out-of-source file the host loads (`CLAUDE.md`/`AGENTS.md`/`GEMINI.md`/`README.md`).
8. **Wire the env** — apply the host's durable mechanism from `host-wiring.md` yourself (or snippet-fallback for the two flagged cases). **Confirm before writing host config** — the second trust action automode keeps.
9. **Report** — print what was written + the "install/enable the plugin and restart, then re-run to verify" close.

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
