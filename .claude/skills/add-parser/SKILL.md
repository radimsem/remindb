---
name: add-parser
description: Use when adding a new file format to remindb's parser package — symptoms include "support .toml/.csv/.xml files", "ingest a new file type", "register an extension in ParseBytes", "wire up a new ContextNode source", or any task that touches `pkg/parser/` to extend supported source formats.
---

# Add a parser for a new file format

remindb's `pkg/parser/` package turns raw source bytes into a slice of `*ContextNode` trees. Adding a format means routing one new extension through `ParseBytes` and emitting nodes that match the conventions every other format already follows.

## Where the format lands

Five files. Skipping any one of them ships a half-wired format.

| File | What changes |
|---|---|
| `pkg/parser/<format>.go` | New file — the parser itself |
| `pkg/parser/parser.go` | Add a `case` to the `ParseBytes` switch |
| `pkg/parser/<format>_test.go` | New file — table tests over fixture strings |
| `pkg/parser/fuzz_test.go` | Add 2–4 `f.Add(...)` seeds to `FuzzParseBytes` |
| `pkg/parser/testdata/` (optional) | Drop a sample file if tests are easier with a real fixture |

## The parser file shape

Mirror `pkg/parser/yaml.go` exactly. The pattern is from `.claude/rules/go-concise.md` §4 ("Namespace prefix-sharing helpers via a struct"): an empty struct provides the namespace, a free function is the entry point, methods drop the prefix.

```go
package parser

import "fmt"

type TomlParser struct{}

func parseToml(path string, data []byte) ([]*ContextNode, error) {
    return TomlParser{}.parse(path, data)
}

func (p TomlParser) parse(path string, data []byte) ([]*ContextNode, error) {
    // ... format-specific decode ...
    if err != nil {
        return nil, fmt.Errorf("failed to parse: toml %s: %w", path, err)
    }
    return []*ContextNode{buildNode(path, "", root, 1)}, nil
}
```

Initialism rule (memory + `go-concise.md`): `TomlParser` not `TOMLParser`, `JsonParser` not `JSONParser`. Pascal-case file-format initialisms.

Error message rule (`go-concise.md` §5): action errors take a `failed to <verb>:` prefix and wrap with `%w`. Validation errors carry no prefix.

## The switch entry

Open `pkg/parser/parser.go`, find `ParseBytes`, add the case in the alphabetic position you'd expect a reader to scan:

```go
switch ext {
case ".md", ".markdown":
    return parseMarkdown(path, data)
case ".toml":
    return parseToml(path, data)         // <-- new line
case ".yml", ".yaml":
    return parseYaml(path, data)
// ...
default:
    return nil, fmt.Errorf("%w: %q", ErrUnsupportedExt, ext)
}
```

Multi-extension formats list every variant in one `case` line (see `.md`/`.markdown`, `.yml`/`.yaml`, `.jsonl`/`.ndjson`).

## The unit test

`pkg/parser/yaml_test.go` is the cleanest template. Table-driven, exercises the happy path plus at least one each of: empty input, malformed input, deeply nested input, and the format's specific edge case (BOM, frontmatter delimiter, mixed-type collection, etc.).

## The fuzz seed corpus — do not skip this

`pkg/parser/fuzz_test.go` defines `FuzzParseBytes`, which `scripts/fuzz.sh` auto-discovers. Add seeds covering your format's shape variety:

```go
f.Add("data.toml", []byte("title = \"Hello\"\n[section]\nkey = 1"))
f.Add("data.toml", []byte(""))                    // empty
f.Add("data.toml", []byte("[unclosed"))           // malformed
f.Add("DATA.TOML", []byte("title = \"upper\""))   // extension case
```

The fuzz invariant is "must never panic regardless of input." If your parser deref's a nil from the decoder or indexes past a slice, fuzz will find it. Run `scripts/fuzz.sh 30s` locally before committing to confirm the new seeds don't regress.

## Quick reference

```
1. Create pkg/parser/<format>.go        (empty struct + parseXxx + (p) parse)
2. Wire pkg/parser/parser.go            (one case in ParseBytes)
3. Create pkg/parser/<format>_test.go   (table tests)
4. Add seeds to pkg/parser/fuzz_test.go (FuzzParseBytes f.Add lines)
5. go test ./pkg/parser/...             (must pass)
6. scripts/fuzz.sh 30s                  (must pass, no panics)
```

## Common mistakes

- **Forgetting fuzz seeds.** The fuzz test will still run and pass, but it'll never exercise the new format. The fuzz step is what catches "decoder returns nil and parser deref's it" before it ships.
- **Returning `(nil, nil)` for empty input but the test asserts `len(nodes) == 0`.** Both work since `len(nil) == 0`, but be explicit in the test about which one your parser actually returns. `yaml.go` short-circuits with `nil, nil` when the document is empty — mirror its style if your decoder gives you an obvious "no content" signal.
- **Setting `SourceFile` to something other than the `path` arg.** Downstream code (search ranking, `MemoryFetch` ancestor walks) keys on it. Pass `path` straight through to `buildNode`.
- **Shadowing the package error sentinels.** Use `ErrInvalidUTF8` and `ErrUnsupportedExt` from `parser.go`; don't redefine them per format.
- **Stutter naming.** `parser.NewTomlParser()` is wrong; the empty struct is exposed bare as `TomlParser` and constructed with `TomlParser{}`. See `go-concise.md` §7.

## Cross-references

- `.claude/rules/go-concise.md` — error message format, naming, struct-with-methods pattern
- `.claude/rules/git-versioning.md` — commit each of (parser, switch, tests, fuzz seeds) as one logical chunk; `feat(parser): add toml support` covers all five files in one commit since they share the idea
- `.claude/skills/add-fuzz-target/SKILL.md` — for the broader fuzz discipline if you're adding a brand-new fuzz target rather than seeding an existing one
