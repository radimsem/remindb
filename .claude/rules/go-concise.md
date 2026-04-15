# Go Concise Coding Patterns

Rules for writing idiomatic, token-efficient, memory-efficient Go in `remindb`.
The file models the discipline it prescribes: terse, direct, example-driven.

**Use when:** writing or reviewing Go in this project.

**Format:** each rule shows the intent, then side-by-side `// Bad` / `// Good`.
Examples are generic unless a `remindb`-specific gotcha is worth anchoring.

**Priority when rules conflict:** correctness > readability > memory efficiency > token efficiency.

---

## 1. Concise Syntax ★

### Infer with `:=`

The LHS type is implied by the RHS. Drop the `var T =` ceremony.

```go
// Bad
var count int = len(nodes)
var name  string = "root"

// Good
count := len(nodes)
name  := "root"
```

Use `var` only for zero-value declarations or when you must state an interface type explicitly.

### Let zero values work for you

Go guarantees zero-value init. Don't re-declare it.

```go
// Bad
var nodes []*Node = nil
var counts map[string]int = make(map[string]int, 0)
var mu sync.Mutex = sync.Mutex{}

// Good
var nodes []*Node
var counts map[string]int   // nil; readable but not writable — make when writing
var mu sync.Mutex
```

A nil slice is usable with `append`, `len`, and `range`. Only `make` a map when you intend to write to it.

### Composite literals over `new`

```go
// Bad
n := new(Node)
n.Kind = NodeHeading

// Good
n := &Node{Kind: NodeHeading}
```

### Name fields in composite literals

If the struct has more than two fields or the order is non-obvious, name them.

```go
// Bad
n := Node{"h1", 1, nil, "doc.md"}

// Good
n := Node{Kind: "h1", Depth: 1, Path: "doc.md"}
```

### Drop redundant element types in nested literals

```go
// Bad
nodes := []*Node{
    &Node{Kind: "h1"},
    &Node{Kind: "p"},
}

// Good
nodes := []*Node{
    {Kind: "h1"},
    {Kind: "p"},
}
```

### `for range` without unused indices or values

```go
// Bad
for i, _ := range nodes { visit(nodes[i]) }
for i := 0; i < n; i++  { _ = i; doWork() }

// Good
for _, n := range nodes { visit(n) }
for range n             { doWork() }   // Go 1.22+: range over int
```

### `any` over `interface{}`

Since Go 1.18, `any` is the canonical alias. Shorter, identical semantics.

```go
// Bad
cache := map[string]interface{}{}

// Good
cache := map[string]any{}
```

### Exported fields beat getters/setters

Go is not Java. Unless validation or synchronization is required, export the field.

```go
// Bad
type Node struct { kind string }
func (n *Node) Kind() string       { return n.kind }
func (n *Node) SetKind(k string)   { n.kind = k }

// Good
type Node struct { Kind string }
```

### Short names in short scopes

Loop vars, receivers, single-use locals: one or two letters is fine.

```go
// Bad
for indexOfNode, currentNode := range nodeList { ... }
func (receiver *Node) String() string          { ... }

// Good
for i, n := range nodes { ... }
func (n *Node) String() string { ... }
```

### Multi-return, blank out what you don't use

```go
// Bad
result, ok := cache[k]
if !ok { return nil }
return result

// Good
v, ok := cache[k]
if !ok { return nil }
return v
```

---

## 2. Memory Efficiency ★

### Preallocate when the size is known

`append` on a nil slice grows by doubling, copying each time. If you know the size (or a close upper bound), set capacity up front.

```go
// Bad
var deltas []Delta
for _, n := range nodes {
    deltas = append(deltas, diff(n))
}

// Good
deltas := make([]Delta, 0, len(nodes))
for _, n := range nodes {
    deltas = append(deltas, diff(n))
}
```

Same for maps — the hint avoids rehashing:

```go
index := make(map[string]*Node, len(nodes))
```

### Reuse slice backing arrays in hot loops

Reset with `s = s[:0]` to keep the allocated array and clear the length.

```go
// Bad — new allocation per iteration
for _, f := range files {
    tokens := []byte{}
    tokens = tokenize(tokens, f)
    emit(tokens)
}

// Good — backing array reused
var tokens []byte
for _, f := range files {
    tokens = tokens[:0]
    tokens = tokenize(tokens, f)
    emit(tokens)
}
```

### Order struct fields by alignment

On 64-bit platforms fields align to their size. Large-first minimizes padding.

```go
// Bad — 24 bytes with padding
type Node struct {
    Flag bool    // 1B + 7B pad
    ID   uint64  // 8B
    Kind uint8   // 1B + 7B trailing pad
}

// Good — 16 bytes
type Node struct {
    ID   uint64  // 8
    Flag bool    // 1
    Kind uint8   // 1 (6B trailing pad)
}
```

Order of thumb: `uint64/pointer → uint32 → uint16 → uint8/bool`. Verify hot structs with `go vet -vettool=$(which fieldalignment)`.

### Pick one receiver style per type

Methods on the same type should use the same receiver. Use **pointer** receivers when:
- the method mutates the receiver,
- the struct is large (≳64 bytes),
- the struct contains sync primitives.

Otherwise prefer **value** receivers — no indirection, fewer heap escapes.

```go
// Bad — mixed
func (n Node)  Kind() string      { return n.kind }
func (n *Node) SetKind(k string)  { n.kind = k }

// Good — consistent
func (n *Node) Kind() string      { return n.kind }
func (n *Node) SetKind(k string)  { n.kind = k }
```

### Stream through `io.Reader` / `io.Writer`

Don't buffer whole files when a streaming interface exists. Parser and emitter paths in `remindb` should take readers/writers, not `[]byte`.

```go
// Bad — buffers the entire file
data, err := os.ReadFile(path)
if err != nil { return err }
return parser.Parse(data)

// Good — streams
f, err := os.Open(path)
if err != nil { return err }
defer f.Close()
return parser.Parse(f)   // Parse(io.Reader) (*AST, error)
```

For AST emission downstream of the transformer, prefer a push model (writer receives each node) or Go 1.23+ range-over-func iterators over collecting into a slice.

### `strings.Builder` for strings, `bytes.Buffer` for bytes

Both avoid the O(n²) cost of `+=`.

```go
// Bad
s := ""
for _, t := range tokens { s += t }

// Good
var b strings.Builder
b.Grow(estSize)                 // hint when size is predictable
for _, t := range tokens { b.WriteString(t) }
return b.String()
```

### Avoid needless conversions

`[]byte(s)` and `string(b)` both allocate. Minimize round trips.

```go
// Bad — three allocations
return []byte(strings.ToUpper(string(data)))

// Good — one
return bytes.ToUpper(data)
```

### Return small values by value; large or shared data by pointer

Escape analysis keeps small return values on the stack.

```go
// Bad — forces a heap allocation for a 16-byte value
func NewPoint(x, y int) *Point { return &Point{x, y} }

// Good
func NewPoint(x, y int) Point { return Point{x, y} }
```

For large structs, types with identity, or types carrying sync primitives, return a pointer.

### `sync.Pool` only on profiled hot paths

Premature pooling is worse than honest allocation. Reach for it when the profiler points to per-call buffer churn (e.g., per-node render buffers in the transformer).

```go
var bufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

func render(n *Node) string {
    b := bufPool.Get().(*bytes.Buffer)
    b.Reset()
    defer bufPool.Put(b)
    writeNode(b, n)
    return b.String()
}
```

### Accept narrow interface types

Narrow interfaces cost nothing; `any` forces boxing on every call.

```go
// Bad
func Hash(v any) uint64 { ... }

// Good
func Hash(r io.Reader) (uint64, error) { ... }
```

### Avoid keeping large backing arrays alive through subslices

Subslicing shares the backing array. If you keep a small slice from a large one for a long time, the whole array stays on the heap.

```go
// Bad — 1 MB array pinned by a 10-byte slice
func prefix(buf []byte) []byte { return buf[:10] }

// Good — copy when storing long-term
func prefix(buf []byte) []byte {
    out := make([]byte, 10)
    copy(out, buf)
    return out
}
```

---

## 3. Control Flow

### Early returns; no `else` after `return`

Handle unhappy paths first; leave the happy path unindented.

```go
// Bad
func emit(n *Node) error {
    if n != nil {
        if n.Kind != "" {
            if err := write(n); err == nil {
                return nil
            } else {
                return err
            }
        }
        return ErrKindMissing
    }
    return ErrNilNode
}

// Good
func emit(n *Node) error {
    if n == nil      { return ErrNilNode }
    if n.Kind == ""  { return ErrKindMissing }
    return write(n)
}
```

### `switch` over chained `else if`

```go
// Bad
if k == "h1" { ... } else if k == "h2" { ... } else if k == "h3" { ... } else { ... }

// Good
switch k {
case "h1": ...
case "h2": ...
case "h3": ...
default:   ...
}
```

### Named returns only when they clarify

Useful for documenting multi-return APIs. Harmful when they invite naked returns in long functions.

```go
// Good — naming aids godoc
func Split(s string) (head, tail string) {
    i := strings.IndexByte(s, '/')
    if i < 0 { return s, "" }
    return s[:i], s[i+1:]
}

// Bad — naked return hides the value
func compile(src []byte) (ast *AST, err error) {
    ast, err = parse(src)
    if err != nil { return }   // what is ast here?
    return
}
```

---

## 4. Types & Interfaces

### Define interfaces at the consumer, keep them small

Producers return concrete types; consumers declare the interface they need.

```go
// Bad — the store package defines a fat interface everyone must satisfy
package store
type Store interface {
    Insert(*Node) error
    Update(*Node) error
    Delete(id string) error
    Get(id string) (*Node, error)
    Search(q string) ([]*Node, error)
    // ... more
}

// Good — the consumer declares only what it uses
package query
type nodeGetter interface { Get(id string) (*Node, error) }
func Assemble(ng nodeGetter, ids []string) ([]*Node, error) { ... }
```

### Accept interfaces, return concrete types

```go
// Bad
func NewParser() Parser { return &parser{} }   // Parser is an interface

// Good
func NewParser() *Parser { return &Parser{} }
```

### Compose; don't build class hierarchies

```go
// Bad — pseudo-inheritance via embedded type with overridable method
type BaseNode struct{ ... }
func (b *BaseNode) Emit() { ... }
type HeadingNode struct{ BaseNode }
func (h *HeadingNode) Emit() { ... /* "override" */ }

// Good — small interface, independent types
type Emitter interface{ Emit(io.Writer) error }

type Heading   struct{ ... }
type Paragraph struct{ ... }

func (h Heading)   Emit(w io.Writer) error { ... }
func (p Paragraph) Emit(w io.Writer) error { ... }
```

### No interface until a second implementation appears

Premature interfaces fragment the code. With one concrete type, export the type.

---

## 5. Error Handling

### Wrap with `%w`, match with `errors.Is` / `errors.As`

```go
// Bad — breaks the chain
return fmt.Errorf("parse failed: %s", err)

// Good
return fmt.Errorf("parse %s: %w", path, err)

// At call sites
if errors.Is(err, os.ErrNotExist) { ... }
```

### Sentinel errors for matchable conditions

```go
var ErrNotFound = errors.New("remindb: node not found")

func (s *Store) Get(id string) (*Node, error) {
    n, ok := s.nodes[id]
    if !ok { return nil, ErrNotFound }
    return n, nil
}
```

### Custom error types only when callers need fields

If callers just compare, use a sentinel. A struct is justified only when fields are inspected.

```go
// Good — carries actionable fields
type ParseError struct {
    Path string
    Line int
    Err  error
}
func (e *ParseError) Error() string { return fmt.Sprintf("%s:%d: %v", e.Path, e.Line, e.Err) }
func (e *ParseError) Unwrap() error { return e.Err }
```

### Handle or return, never both

```go
// Bad — double reporting
if err := insert(n); err != nil {
    log.Printf("insert: %v", err)
    return err
}

// Good — caller decides
if err := insert(n); err != nil {
    return fmt.Errorf("insert %s: %w", n.ID, err)
}
```

### Don't panic across package boundaries

Panics belong to programmer errors inside a package. Public APIs return `error`.

---

## 6. Concurrency — Parallelize Independent Work

Go's concurrency model is a primary lever for speed in `remindb`: parse many files, hash many nodes, emit many rows. Use it. The rules below keep parallelism **bounded, cancellable, and leak-free**.

### `errgroup` for bounded parallel fan-out

`golang.org/x/sync/errgroup` combines goroutine orchestration, first-error propagation, and `ctx` cancellation in one primitive.

```go
import "golang.org/x/sync/errgroup"

// Parse N files in parallel, cap at GOMAXPROCS, fail fast on first error.
func ParseAll(ctx context.Context, paths []string) ([]*AST, error) {
    g, ctx := errgroup.WithContext(ctx)
    g.SetLimit(runtime.GOMAXPROCS(0))

    out := make([]*AST, len(paths))
    for i, p := range paths {
        g.Go(func() error {
            ast, err := parser.ParseFile(ctx, p)
            if err != nil { return fmt.Errorf("parse %s: %w", p, err) }
            out[i] = ast           // safe: each i writes a distinct index
            return nil
        })
    }
    if err := g.Wait(); err != nil { return nil, err }
    return out, nil
}
```

Go 1.22+ scopes loop variables per iteration, so `i` and `p` capture correctly without aliasing.

### `context.Context` is the first parameter; propagate it

Anything that does I/O, blocks, or launches goroutines takes `ctx context.Context` first. Check `ctx.Err()` at loop boundaries and pass it down.

```go
// Bad
func (e *Engine) Query(q string) ([]*Node, error) { ... }

// Good
func (e *Engine) Query(ctx context.Context, q string) ([]*Node, error) {
    if err := ctx.Err(); err != nil { return nil, err }
    ...
}
```

Never store a `context.Context` in a struct. Pass it through calls.

### Fan-out / fan-in for streaming pipelines

When stages process independently, wire them with buffered channels and run each in its own goroutine.

```go
// parse → transform → emit, stages run in parallel, ctx cancels the whole thing.
func Pipeline(ctx context.Context, paths []string) error {
    g, ctx := errgroup.WithContext(ctx)

    asts := make(chan *AST, 16)
    g.Go(func() error {
        defer close(asts)
        return parseStage(ctx, paths, asts)
    })

    deltas := make(chan Delta, 16)
    g.Go(func() error {
        defer close(deltas)
        return transformStage(ctx, asts, deltas)
    })

    g.Go(func() error {
        return emitStage(ctx, deltas)
    })

    return g.Wait()
}
```

Small buffers (8–64) absorb speed variance between stages without hiding backpressure.

### Close channels on the send side, exactly once

```go
// Bad — sender may panic on a later send; closing is the receiver's problem
ch := make(chan *Node)
go func() { for v := range produce() { ch <- v } }()
defer close(ch)

// Good
ch := make(chan *Node)
go func() {
    defer close(ch)
    for v := range produce() { ch <- v }
}()
```

### Every goroutine needs an exit path

```go
// Bad — blocks forever if the reader stops
go func() {
    for _, n := range nodes { out <- n }
}()

// Good — honors cancellation
go func() {
    defer close(out)
    for _, n := range nodes {
        select {
        case out <- n:
        case <-ctx.Done(): return
        }
    }
}()
```

### Channels for ownership transfer; `sync` for shared state

Use channels to hand data to a new owner. Use `sync.Mutex` / `sync.RWMutex` / `sync/atomic` to protect state that multiple goroutines touch in place.

```go
// Bad — a channel used as a mutex
updates := make(chan func(), 1)
go func() { for f := range updates { f() } }()
updates <- func() { counter++ }

// Good
var mu sync.Mutex
mu.Lock()
counter++
mu.Unlock()
```

### Bound concurrency to CPU or I/O capacity

- CPU-bound (parsing, hashing, transforming): `runtime.GOMAXPROCS(0)`.
- I/O-bound (DB writes, network): higher cap; profile to find the knee.
- Unbounded `go f()` inside a loop is almost always a bug.

---

## 7. Comments & Naming

### Document exported identifiers; begin with the name

```go
// Bad
// parses a markdown file
func ParseMarkdown(r io.Reader) (*AST, error) { ... }

// Good
// ParseMarkdown reads Markdown from r and returns its AST.
func ParseMarkdown(r io.Reader) (*AST, error) { ... }
```

### Document *why*, not *what*, for non-obvious code

```go
// Bad
// increment i
i++

// Good
// Skip the BOM if present; some editors emit it for UTF-8.
if bytes.HasPrefix(data, bom) { data = data[3:] }
```

### Package comments on one file per package, starting "Package X"

```go
// Package parser turns source files into a unified AST.
// Each file extension dispatches to a format-specific parser; the AST
// shape is identical across formats.
package parser
```

### Naming conventions

- `MixedCaps`, never `snake_case`.
- Short names in short scopes (`i`, `n`, `r`, `b`); full names at package level.
- Avoid stutter: `parser.New()` not `parser.NewParser()`; `store.Node` not `store.StoreNode`.
- Initialisms stay uppercase: `parseURL`, `HTMLTag`, `userID`.
- Single-method interfaces take the `-er` suffix: `Reader`, `Emitter`, `NodeGetter`.
- No Hungarian notation and no type prefixes: `name`, not `strName`.

---

## 8. Anti-Patterns — Do Not

A scannable "do not" list. Cross-references point to fuller rules above.

### Don't initialize with explicit zero values

```go
// Bad
var s string = ""
var n int    = 0
var m map[string]int = nil
```

### Don't use `new(T)` for structs

```go
// Bad
p := new(Point)
// Good
p := &Point{}
```

### Don't compare errors directly when wrapping is possible

```go
// Bad — misses wrapped errors
if err == io.EOF { ... }

// Good
if errors.Is(err, io.EOF) { ... }
```

### Don't create named types that carry no invariant

```go
// Bad
type NodeID  string
type UserID  string
type PathStr string   // no methods, no invariants

// Good
// Just use string.
```

Exception: when the type prevents a real mixup (`Dollars` vs `Cents`, duration-like units).

### Don't ignore errors with `_`

```go
// Bad
data, _ := os.ReadFile(path)

// Good
data, err := os.ReadFile(path)
if err != nil { return err }
```

`_ = w.Close()` after a `bytes.Buffer` write is acceptable; the error genuinely cannot matter.

### Don't use `init()` for anything non-trivial

It runs before `main`, can't return errors, complicates testing. Prefer explicit initialization in `main` or `New...` constructors.

### Don't `log.Fatal` or `os.Exit` outside `main`

Libraries return errors. Fatal-exit belongs to the entry point.

### Don't pre-allocate what the common path won't use

```go
// Bad — allocates a map that stays empty unless caching is on
cache := make(map[string]*Node, 1024)
if !useCache { return ... }
```

### Don't export types, funcs, or fields you don't need to

Unexported = free to refactor. Exported = part of the API contract.

### Don't pin large backing arrays through subslices

See §2 — copy when storing a slice long-term.

### Don't conflate "concise" with "clever"

If a reader has to puzzle out what a line does, it's too dense. Readability wins over terseness.

---

## Priority When Rules Conflict

1. **Correctness** — never compromised.
2. **Readability** — the next reader (human or Claude) must get it in one pass.
3. **Memory efficiency** — avoid needless allocation; preallocate when sized.
4. **Token efficiency** — prefer the shorter idiomatic form when it doesn't hurt 1–3.
