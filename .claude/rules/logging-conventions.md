# Logging Conventions

Rules for writing and reviewing `log/slog` calls in `remindb`.

**Use when:** writing or reviewing any `*slog.Logger.{Debug,Info,Warn,Error}` call, any code that takes a `*slog.Logger` as a dependency, or any log-related plumbing in `cmd/`.

**Scope:** `slog` is the only logging package used in `remindb`. No `log` (stdlib pre-slog), no `fmt.Println` for diagnostics, no third-party loggers. Tests use `t.Logf`, not slog.

**Priority when rules conflict:** safety (no leaks) > signal (one event one line) > consistency > brevity.

---

## 1. Level Selection ★

The four `slog` levels carry distinct semantics. Picking the wrong level breaks log-grep workflows and the `--verbose` flag in `cmd/remindb/serve.go:108-114`.

| Level | When | Example |
|---|---|---|
| `Debug` | Per-call traces, per-tick state, anything you'd want only with `--verbose` | `d.Logger.Debug("mcp call", "tool", name, "elapsed_ms", ...)` |
| `Info` | Process milestones — start, stop, one-time setup, scheduled-event summaries | `logger.Info("serve: starting", "db", dbPath, ...)` |
| `Warn` | Recoverable failure — operation degraded but the program continues | `s.logger.Warn("failed to send: cold-node notification", "err", err)` |
| `Error` | Operation aborted; the failure is being returned or terminating a goroutine | `t.logger.Error("temperature tick failed", "err", err)` |

```go
// Bad — Info for a per-call trace; floods stderr without --verbose
logger.Info("search call", "query", q, "budget", b)

// Bad — Warn for a hard failure that's being returned
logger.Warn("compile failed", "err", err)
return err

// Bad — Error for a recoverable problem; conflates "we kept going" with "we stopped"
logger.Error("skipping unsupported file", "path", p)

// Good — Debug for per-call trace
logger.Debug("mcp call", "tool", name, "elapsed_ms", elapsed)

// Good — Warn for "happened but we recovered"
logger.Warn("compile: skipping unsupported file", "path", p, "err", err)

// Good — Error for terminal failure of a long-running goroutine
logger.Error("serve: stopped with error", "err", err)
```

A useful test: if you remove the line, does the program's behavior change at all? If yes, it's not a log — it's control flow. If no, the question is "would I want this in normal operation?" → Info, "only when debugging?" → Debug.

---

## 2. Structured Fields, Never `Sprintf` Into the Message ★

`slog`'s value is structured fields. Embedding values in the message defeats searchability and breaks the JSON handler used in tests.

```go
// Bad — values in the message
logger.Info(fmt.Sprintf("compiled %d files in %dms", n, elapsed))
logger.Warn("failed to load " + path + ": " + err.Error())

// Bad — interpolated message; values can't be filtered by handler
logger.Info(fmt.Sprintf("tick: decayed=%d cold=%d", decayed, cold))

// Good
logger.Info("compile: done", "files", n, "elapsed_ms", elapsed)
logger.Warn("failed to load: file", "path", path, "err", err)
logger.Debug("temperature tick", "decayed", decayed, "cold", cold)
```

The message is a **constant** noun-phrase or "verb-ing" describing the event. Variable data goes in the `key, value, key, value, ...` variadic pairs.

---

## 3. Field Names ★

Conventions in this codebase, observable across `pkg/mcp/`, `pkg/temperature/`, `pkg/compiler/`, and `cmd/`:

- **`snake_case` keys.** `node_id`, `payload_bytes`, `elapsed_ms`, `rescan_interval`, `tick_interval`. Not `camelCase`, not `kebab-case`.
- **`err` for errors.** Always. Not `error`, not `e`, not `cause`. The `slog` text handler prints it `err=<message>` which downstream parsers expect.
- **Suffix units when ambiguous.** `elapsed_ms`, `payload_bytes`, `tick_interval` (Duration is self-describing in `slog`). Bare `count` is fine when context makes the unit obvious.
- **`id` for opaque identifiers; `<thing>_id` when ambiguous.** `tool`, `node_id`, `snapshot_id`, `cursor_hash`. Not `nodeId`, not `tool_name`.

```go
// Bad — camelCase, ambiguous units, generic key for an error
logger.Info("compile done", "fileCount", n, "elapsed", ms, "error", err)

// Good
logger.Info("compile: done", "files", n, "elapsed_ms", ms, "err", err)
```

---

## 4. What Never Logs ★

Three categories that must never reach a log handler. The text handler writes to stderr in `serve`; payloads end up in user terminals, log files, and CI artifacts.

**Never log:**

- **User content / payloads / node bodies.** The `payload` arg to `MemoryWrite`, the `summary` arg to `MemorySummarize`, the full content of any `*store.Node`. Use byte counts and IDs only.
- **Secrets.** API keys, tokens, signing keys. The codebase has none currently, but the rule applies preemptively.
- **Full SQL strings with bind values inlined.** Log the query name (`qBoostTemperatureBatch`) or the verb (`"insert: nodes"`) plus argument counts.

```go
// Bad — full payload reaches the log
defer d.logCall("MemoryWrite", &err, time.Now(), "payload", input.Payload)

// Bad — node content
logger.Debug("emit node", "content", node.Content)

// Bad — SQL with values
logger.Debug("running query", "sql", fmt.Sprintf("UPDATE nodes SET temperature=%f WHERE id='%s'", t, id))

// Good — sizes and IDs
defer d.logCall("MemoryWrite", &err, time.Now(), "anchor", input.Anchor, "payload_bytes", len(input.Payload))
logger.Debug("emit node", "node_id", node.ID, "content_bytes", len(node.Content))
logger.Debug("running query", "name", "qBoostTemperatureBatch", "node_count", len(ids))
```

The one documented exception is `MemorySearch`'s `query` field — it's small, user-supplied, and necessary to debug a misbehaving FTS5 ranking. Never extend the exception to other tools.

---

## 5. Nil-Logger Safety ★

Library code — anything in `pkg/` — must accept a `nil` logger and behave silently. Two project conventions for the fallback:

| Where | Fallback | Why |
|---|---|---|
| `pkg/` libraries (`mcp`, `temperature`) | `slog.New(slog.DiscardHandler)` | Tests and embedders may not want output |
| CLI-time code (`pkg/compiler` when invoked directly) | `slog.Default()` | Users running `remindb compile` want feedback |

```go
// Bad — assumes the caller passed a non-nil logger
func NewServer(st *store.Store, ..., logger *slog.Logger) *Server {
    s := &Server{logger: logger, ...}
    s.logger.Info("server created")   // panics if logger == nil
    return s
}

// Good — pkg/ library default
func NewServer(st *store.Store, ..., logger *slog.Logger) *Server {
    if logger == nil {
        logger = slog.New(slog.DiscardHandler)
    }
    return &Server{logger: logger, ...}
}

// Good — CLI-time default (used only when the call is the entry point)
func Run(ctx context.Context, st *store.Store, opts ...Option) error {
    o := applyOptions(opts...)
    logger := o.logger
    if logger == nil {
        logger = slog.Default()
    }
    ...
}
```

See `pkg/mcp/server.go:26-28` and `pkg/temperature/tracker.go:30-32` for the discard pattern; `pkg/compiler/compiler.go:63-66` for the default pattern.

---

## 6. Handle OR Return — Never Both ★

Per `.claude/rules/go-concise.md` §5, errors are either logged-and-handled or returned-and-wrapped, not both. Logging a returned error is double-reporting and bloats the trail.

```go
// Bad — caller will log it again at a higher level
if err := insert(n); err != nil {
    logger.Error("insert failed", "err", err)
    return err
}

// Good — caller decides; we wrap with context
if err := insert(n); err != nil {
    return fmt.Errorf("failed to insert: %s: %w", n.ID, err)
}

// Good — terminal handling: we are the caller of last resort
g.Go(func() error {
    if err := srv.Run(ctx); err != nil {
        logger.Error("serve: stopped with error", "err", err)   // logged here because it's the top of the goroutine
        return err
    }
    return nil
})
```

The exception: when the function is the last frame in its goroutine (a `g.Go` callback, a deferred handler, the `main` body), log-and-return is fine because no one else will see the error.

---

## 7. The MCP Tool `defer d.logCall(...)` Pattern

Tools in `pkg/mcp/tools/` use a deferred helper instead of two log calls (one before, one after):

```go
func (d *Deps) HandleX(ctx ..., input XInput) (_ *gomcp.CallToolResult, _ any, err error) {
    defer d.logCall(ctx, "MemoryX", &err, time.Now(), "anchor", input.Anchor, "budget", input.Budget)
    ...
}
```

This is the only sanctioned way to log MCP tool calls. `d.logCall` (in `pkg/mcp/tools/deps.go`) inspects the captured `err` and routes to `DebugContext` for success or `ErrorContext` for failure with the same structured fields. `ctx` is **mandatory** — it carries the session id the registry middleware injected (`sessionlog.NewContext`), which the outermost `sessionlog.Handler` reads to tee the record into `.remindb/logs/<session-id>.log`. For the same reason, in-handler `Warn`/`Error` (e.g. boost failures) use the `*Context` variants (`WarnContext(ctx, …)`), not the bare ones, so they reach the right session file. Don't add extra `Info` / `Debug` lines around tool bodies — they desync the trace and double the log volume.

The per-session sink inherits §4 unconditionally: it formats the *same* payload-free fields the shared handler does, so "never log the payload/body" already covers it — there is no separate session-log redaction step, and none is needed. The on-disk format is **JSONL** — one `sessionlog.Record` (`{time, level, msg, fields}`) per line, the single shared definition the `remindb://sessions/logs/{id}` resource deserializes back (render serializes it, the resource parses it; no second hand-rolled parser that could drift). §4 still binds the `fields` object exactly as it binds the shared handler's attrs. Any structured (non-`%v`) serialization of slog attrs must coerce `error` (and `fmt.Stringer`) to string before encoding — `json.Marshal` of an error yields `{}`, silently dropping the message the text handler keeps. `sessionlog.jsonable` is the chokepoint; route new structured sinks through it. The session file is opt-in (`server.logging.session_files.enabled`); disabled ⇒ the `sessionlog.Handler` is not in the chain at all and behavior is byte-identical to today.

See `.claude/rules/mcp-tool-conventions.md` §9 for the full attr-selection rule.

---

## 8. Long-Running Loops Log on Tick, Not Per-Iteration

Background loops (`Tracker.Run`, `RescanLoop.Run`, the rescan inner loop) emit one summary log line per tick at `Debug`, plus an `Info` only when something actionable happened.

```go
// Bad — log per node; floods even at Debug
for _, n := range cold {
    logger.Debug("found cold node", "id", n.ID)
}

// Bad — Info on every tick; cluttering normal-volume output
logger.Info("temperature tick", "decayed", decayed, "cold", len(cold))

// Good — one summary line per tick at Debug
logger.Debug("temperature tick", "decayed", decayed, "cold", len(cold))

// Good — Info only when there's something the operator should see
if len(cold) > 0 {
    logger.Info("cold nodes detected", "count", len(cold))
}
```

See `pkg/temperature/cold.go:27-29` and `cmd/remindb/serve.go:85-88` for the canonical "tick at Debug, milestone at Info" split.

---

## Anti-Patterns — Do Not

- `fmt.Println` / `fmt.Fprintln(os.Stderr, ...)` for diagnostics. Use `slog`.
- Standard-library `log` package — banned project-wide.
- `Sprintf` into the message; values belong in structured fields.
- Logging the payload, summary, node content, full SQL with values, or any user body.
- `Info` for per-call traces (use `Debug`); `Warn` for recoverable issues that aren't recoveries (use `Error`); `Error` for things that aren't actually failures (use `Warn` or `Info`).
- `Logger == nil` panics — always default to `DiscardHandler` (library) or `Default()` (CLI entry point).
- Logging an error and returning it. One or the other.
- `camelCase` / `kebab-case` field keys; `error` instead of `err`.
- Per-iteration log lines in hot loops.
- Multiple log lines around an MCP tool body — `defer d.logCall(...)` is the entire trace contract for that layer.

---

## Priority When Rules Conflict

1. **Safety** — never leak payloads, secrets, or full bodies. Single hardest rule.
2. **Signal** — one event = one line; the `--verbose` flag should toggle a useful amount of additional detail, not a flood.
3. **Consistency** — match the conventions in adjacent files; field names, level choices, and message style stay uniform across packages.
4. **Brevity** — prefer the shorter idiomatic form when it doesn't hurt 1–3.
