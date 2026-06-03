# Write paths — file (compile plane) vs MemoryWrite (flat plane)

Reference for `memorize`. Load when deciding *how* to persist memory, or when authoring a source file for the compile plane.

## Why the plane matters

`MemoryWrite` **does not parse**. It stores your payload as exactly one `text` node — raw, `FormatPlain`, depth 1, `source = mcp:write` — with no heading/list/code splitting, no tree, no TOON/MathML compaction. Headings and lists in a `MemoryWrite` payload become literal text inside one unsearchable node (the "flat blob" anti-pattern).

Only the **compile plane** builds structure: a file under `$REMINDB_SOURCE` → the parser → a multi-node subtree (headings own children; lists/code/tables become leaves; uniform data compacts to TOON; `[[Label]]` becomes a relation). So **structural memory must be a file**, not a `MemoryWrite`.

## Compile plane — writing a source file

### 1. Resolve the source root

```
echo "$REMINDB_SOURCE"      # canonical source root
remindb__MemoryStats()      # compile root, if the env var is unset
```

No source root → **`MemoryCompile` is not registered** (absent from the tool list) and there is no compile plane in this session (serve has no `--source`). Either set one, or fall back to `MemoryWrite` for flat notes.

### 2. Place the file where it topically belongs

Walk the existing tree (`remindb__MemoryTree`) to see how memory is organized, then:

- **New topic** → a new file at the path that fits the subject (e.g. a `decisions/`, `notes/`, or domain-named dir). Use your filesystem tools (`Write`).
- **Extends an existing topic** → edit the existing source file the content belongs to (find it via the node's `source_file`).

Constraints: use a **supported extension** (`.md` is the natural choice; also `.html`/`.json`/`.yaml`/`.toon`); the path must **not** match `.remindb/ignore`; author the file body with the shape rules (headings → tree spine, lists/code/tables → leaves — see `parser-mapping.md`).

### 3. Trigger the compile

Check `$REMINDB_SOURCE/.remindb/config.json` → `rescan.enabled`:

- **Rescan enabled** (the default; key absent = enabled) → the background loop picks up the changed file on its next tick and recompiles it automatically. Nothing else to do.
- **Rescan explicitly disabled** (`"rescan": { "enabled": false }`), or you can't wait for the tick → run the compile yourself:

```
remindb__MemoryCompile(path="<file or dir>", message="<why>")
```

Either way the compile is **incremental** — the compiler diffs the new parse against stored state and emits **only changed nodes** in one snapshot. Recompiling a whole dir after a one-file edit is cheap; prefer the narrow path anyway.

## Flat plane — MemoryWrite

For a **single text update to an existing anchor** or a **single new text fact** (no structure):

```
remindb__MemoryWrite(anchor="<id>", payload="<new text>")   # update one node in place
remindb__MemoryWrite(payload="<one short fact>")            # new flat text node
```

Update preserves the node's `node_type`/`parent_id`/`source`; create makes a `text` node at depth 1. One logical update per call (each snapshots).

## Trap — MemoryWrite on a file-sourced node

If a node's `source_file` is a real file (not `mcp:write`), updating it with `MemoryWrite` swaps the DB content in place while the file on disk stays stale — the two diverge, and a later rescan/recompile of that file overwrites your edit. For content that lives in a file, **edit the file and recompile** (compile plane) rather than `MemoryWrite`-ing the node.
