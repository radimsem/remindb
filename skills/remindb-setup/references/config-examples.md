# Config examples — `.remindb/` starting points per workspace type

Reference for `remindb-setup`. Load when authoring `.remindb/` — in Pass 1 this happens **before** the first `remindb compile`, so these files shape the very first insert (no reseed). Copy a template, then tune to the actual tree. Knob semantics → `config-model.md`.

## Code repository

Source = a code repo whose docs/specs you want as memory; skip build output and vendored deps.

`.remindb/ignore`
```
# build + deps + generated noise (hardcoded skip dirs already excluded)
dist/
build/
*.lock
*.min.*
coverage/
testdata/**/*.golden
```

`.remindb/pinned`
```
README.md
CONTRIBUTING.md
**/CONTEXT.md
docs/adr/**
**/*.spec.md
SECURITY.md
```

`.remindb/temperatures.json`
```json
{ "README.md": 0.8, "docs/architecture.md": 0.7, "docs/**": 0.5, "CHANGELOG.md": 0.2 }
```

`.remindb/config.json`
```json
{
  "budgets": { "search": 1000, "fetch": 1500, "fetch_batch": 4000 },
  "temperature": { "tick_interval": "5m", "cold_threshold": 0.1, "hot_threshold": 0.5 },
  "rescan": { "enabled": true, "interval": "30s" }
}
```

## Docs / notes vault (e.g. Obsidian, a wiki)

Mostly Markdown, large, fairly uniform; everything is signal. Pin the maps-of-content, let leaves decay.

`.remindb/ignore`
```
.obsidian/
*.canvas
attachments/
```

`.remindb/pinned`
```
**/MOC*.md
**/index.md
**/_index.md
```

`.remindb/config.json`
```json
{
  "budgets": { "search": 1500, "fetch": 2000 },
  "temperature": { "tick_interval": "10m", "cold_threshold": 0.1 },
  "rescan": { "enabled": true, "interval": "60s" }
}
```

## Agent cross-project memory (`~/.claude/projects`)

Index per-project `memory/*.md`, skip the surrounding session telemetry.

`.remindb/ignore`
```
# compile only per-project memory/ markdown
*.jsonl
subagents/
tool-results/
```

`.remindb/pinned`
```
**/memory/**.md
```

`.remindb/config.json`
```json
{
  "budgets": { "search": 1000, "fetch": 1500 },
  "temperature": { "tick_interval": "5m" },
  "rescan": { "enabled": true, "interval": "30s" }
}
```

## Mixed workspace (code + docs + notes)

Conservative defaults; lean on `ignore` to cut noise, pin only the few canonical entry points.

`.remindb/ignore`
```
node_modules/
dist/
build/
*.lock
*.log
.cache/
```

`.remindb/pinned`
```
README.md
**/CONTEXT.md
**/index.md
```

`.remindb/config.json`
```json
{
  "budgets": { "search": 1000, "fetch": 1500, "fetch_batch": 4000, "related": 1000 },
  "temperature": { "tick_interval": "5m", "cold_threshold": 0.1, "hot_threshold": 0.5 },
  "rescan": { "enabled": true, "interval": "30s" }
}
```

## Memory-provider bridge — Hermes Agent (`only-config` mode)

Source = `~/.hermes/memories/` (Hermes' own curated memory directory). Used with `/remindb-setup only-config` when the host owns env wiring via its own setup wizard — see the hermes-agent plugin README. Narrow source root means no aggressive `ignore` is needed; pin and warm the two files Hermes itself maintains under `memories/`.

`.remindb/ignore`
```
.DS_Store
Thumbs.db
```

`.remindb/pinned`
```
MEMORY.md
USER.md
```

`.remindb/temperatures.json`
```json
{ "MEMORY.md": 0.9, "USER.md": 0.8 }
```

`.remindb/config.json`
```json
{
  "budgets": { "search": 1500, "fetch": 2000 },
  "temperature": { "tick_interval": "10m", "cold_threshold": 0.1 },
  "rescan": { "enabled": true, "interval": "60s" }
}
```

> Templates are starting points, not answers. Walk the real tree first — a `pinned` glob that matches 80% of files kills the cold-set signal; an `ignore` that's too broad starves memory. Validate against `config-model.md` and the full schema before writing.
