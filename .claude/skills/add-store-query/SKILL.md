---
name: add-store-query
description: Use when adding or modifying a SQL query against the SQLite store — symptoms include "add a method to *Store", "new column on nodes", "expose X via the store", "add an index", "support a new lookup", "write a migration", or any change to `pkg/store/queries.go`, `pkg/store/*.go` methods, or `migrations/`. Covers both the read/write method workflow and the schema-migration workflow with FTS5 trigger sync.
---

# Add a query (and optionally a migration) to the store

`pkg/store/` is the SQLite layer. Queries are SQL string constants in `queries.go`; methods on `*Store` execute them through `s.Tx(...)` for writes or `s.db.QueryContext(...)` for reads. Schema lives in `migrations/000N_*.sql`, embedded via `embed.FS` and applied at startup.

There are two workflows here. Pick the right one before you start.

```
Need new SQL only?  ──> Workflow A (query)
Need a new column / table / index?  ──> Workflow A + Workflow B (migration)
```

## Workflow A — add a query and method

Three files. Same pattern every time.

| File | What changes |
|---|---|
| `pkg/store/queries.go` | Add a `q<Verb><Noun> = ...` const |
| `pkg/store/<domain>.go` | Add the method on `*Store` (`temperature.go`, `node.go`, `search.go`, `snapshot.go` — pick the matching domain) |
| `pkg/store/store_test.go` | Add a test that opens `testutil.OpenTestDB(t)` and exercises the method |

### Query const

Constants are grouped at the bottom of `queries.go` next to related queries. Name them `q<Verb><Noun>` — e.g., `qSelectColdNodes`, `qBoostTemperature`, `qIncrementAccess`. No package prefix; the file is the namespace.

```go
const qListSourceFiles = `SELECT DISTINCT source_file FROM nodes ORDER BY source_file`
```

For batch queries with a variable IN list, follow the `qBoostTemperatureBatchPrefix` pattern: define the prefix as a const, append placeholders + `)` in the calling method.

### Method shape

Two flavors based on read vs write.

**Reads** call `s.db.QueryContext` directly, defer `rows.Close()`, and return through `collectRows` (or a dedicated row-scanner if the columns differ from `nodeColumns`). Reads do **not** wrap in `s.Tx(...)`.

```go
func (s *Store) ListSourceFiles(ctx context.Context) ([]string, error) {
    rows, err := s.db.QueryContext(ctx, qListSourceFiles)
    if err != nil {
        return nil, err
    }
    defer func() { _ = rows.Close() }()

    var out []string
    for rows.Next() {
        var path string
        if err := rows.Scan(&path); err != nil {
            return nil, err
        }
        out = append(out, path)
    }
    return out, rows.Err()
}
```

**Writes** wrap in `s.Tx(ctx, func(tx *sql.Tx) error { ... })` so the FTS5 triggers fire atomically with the main update. See `BoostTemperature` / `DecayTemperatures` in `temperature.go`.

```go
func (s *Store) MarkVisited(ctx context.Context, id string) error {
    return s.Tx(ctx, func(tx *sql.Tx) error {
        _, err := tx.ExecContext(ctx, qMarkVisited, time.Now().Unix(), id)
        return err
    })
}
```

For writes that need `RowsAffected`, capture it inside the closure via a named-scope variable; see `DecayTemperatures` for the pattern.

### Test

`store_test.go` defines a local unexported `openTestDB(t)` helper (the public `testutil.OpenTestDB` is for *cross-package* callers like integration tests; inside `pkg/store` itself, use the local helper to avoid an import cycle). Seed rows through other store methods (don't poke SQL directly inside the test body), exercise the new method, assert. Mirror the existing tests for tone — minimal, table-driven where it helps.

## Workflow B — add a migration (when schema changes)

Migrations are numbered SQL files in `migrations/`, loaded via `//go:embed *.sql` (`migrations/migrations.go`). They run in lex order at startup, so file naming carries the order.

### Naming

`000N_<verb>_<noun>.sql` — zero-padded to 4 digits, snake_case body. Existing files: `0001_init.sql`, `0002_compile_root.sql`. Next is `0003_<verb>_<noun>.sql`.

### Content rules

- **`IF NOT EXISTS` / `IF EXISTS` everywhere.** Migrations may be applied to a partially-initialized DB; idempotency is the only safe assumption.
- **One logical change per migration.** Adding a column and adding an index = two migrations, two files. Easier to roll forward, easier to read in `git log`.
- **Constraints belong on the column, not in a separate migration.** `NOT NULL`, `DEFAULT`, `REFERENCES` go on the `ALTER TABLE ADD COLUMN` line.

### FTS5 trigger sync — the easy-to-miss bit

`nodes_fts` is a virtual FTS5 table whose content is mirrored from `nodes` via three triggers (`nodes_fts_insert`, `nodes_fts_delete`, `nodes_fts_update` in `0001_init.sql:52-67`). If your migration adds a *searchable* column to `nodes`, you must:

1. Add the column to the `nodes_fts USING fts5(...)` declaration — but FTS5 virtual-table column lists can't be ALTERed. The migration has to **drop and recreate** `nodes_fts`, then **rebuild** by triggering the insert path for every existing row.
2. Update **all three** triggers to reference the new column.

This is invasive. Reach for it only when the column truly needs full-text matching. For metadata-only columns, leave `nodes_fts` alone.

If you only need indexable but non-FTS lookup, add a regular `CREATE INDEX IF NOT EXISTS idx_nodes_<col> ON nodes(<col>);` line — see the existing `idx_nodes_temperature`, `idx_nodes_parent` etc. at the bottom of `0001_init.sql`.

### Verify the migration applies cleanly

```
go test ./pkg/store/... -run TestMigrate    # must pass
go test ./...                                # full suite picks up schema drift
```

## Quick reference

```
Workflow A (query only):
1. pkg/store/queries.go        (q<Verb><Noun> const)
2. pkg/store/<domain>.go       (method on *Store)
3. pkg/store/store_test.go     (Test<Method>_<Case>)
4. go test ./pkg/store/...

Workflow B (schema):
1. migrations/000N_<verb>_<noun>.sql   (one logical change, IF NOT EXISTS)
2. If touching nodes searchable cols: rebuild nodes_fts + update 3 triggers
3. Then Workflow A for the read/write methods that consume the new shape
4. go test ./...
```

## Common mistakes

- **`s.db.ExecContext` for writes instead of `s.Tx(...)`.** FTS5 triggers run inside the implicit transaction; raw `Exec` may still work for simple updates but breaks atomicity guarantees and is inconsistent with the codebase. Use `s.Tx`.
- **Naming the const `qInsertSomething` when it's an UPDATE.** The verb has to match the SQL operation. Reviewer-grep relies on it.
- **Adding a column without an index, then querying it.** SQLite does table scans on un-indexed columns. If your new method's WHERE clause filters on the column, add `CREATE INDEX IF NOT EXISTS` in the same migration.
- **Touching `nodes_fts` without rebuilding.** A schema change to `nodes` searchable columns leaves `nodes_fts` stale until a full rebuild. Search results will silently drop the new column from ranking.
- **Forgetting `IF NOT EXISTS` on `CREATE TABLE` / `CREATE INDEX`.** Re-running migrations on a partially-applied DB will fail without it. Every existing line in `0001_init.sql` uses it.
- **Using `time.Now()` in the SQL string.** Use SQLite's `unixepoch()` function (see `qBoostTemperature`) or pass `time.Now().Unix()` as a bind arg. String concatenation is both wrong and the start of an injection bug.

## Cross-references

- `.claude/rules/go-concise.md` — error wrapping (action errors get `failed to <verb>:`), method receiver consistency, no premature interface
- `.claude/rules/git-versioning.md` — `feat(store): add MarkVisited query` is one commit; `feat(migration): 0003 add visited_at column` is a separate commit if it ships independently. Migration files are append-only — never edit a committed migration; write a new one
- `.claude/skills/add-mcp-tool/SKILL.md` — when a new tool needs a new query, do this skill first
